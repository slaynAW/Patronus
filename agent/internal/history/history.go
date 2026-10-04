// Package history tient le journal du PC sur 30 jours : démarrages, arrêts (propres ou non
// enregistrés), mises en veille, sorties de veille et commandes reçues. Il est consulté par les
// applications (commande « history ») : il reste complet même quand elles sont fermées.
package history

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol"
)

const (
	// Retention est la durée de conservation des évènements.
	Retention = 30 * 24 * time.Hour
	// MaxEvents borne la taille du journal (la réponse doit tenir dans protocol.MaxHistoryBytes).
	MaxEvents = 2000
	// sameBootTolerance : sans identifiant de démarrage, deux heures de démarrage aussi proches
	// désignent le même démarrage (l'horloge a pu être corrigée entre-temps).
	sameBootTolerance = 10 * time.Minute
	// WakeMatch : deux démarrages demandés à moins de cet écart sont le même (signalé deux fois, ou
	// par deux appareils à la fois).
	WakeMatch = time.Minute
	// WakeLead : une demande de démarrage précède le démarrage qu'elle provoque ; elle est acceptée
	// jusqu'à cet écart avant le début du journal (journal commencé à ce démarrage).
	WakeLead    = 10 * time.Minute
	fileVersion = 1
)

// Boot décrit le démarrage en cours du système.
type Boot struct {
	// ID identifie le démarrage (vide s'il est inconnu sur ce système).
	ID string
	// At est l'heure du démarrage.
	At time.Time
}

type state struct {
	Version int    `json:"version"`
	Since   int64  `json:"since"`
	BootID  string `json:"bootId,omitempty"`
	Boot    int64  `json:"boot,omitempty"`
	// PendingShutdown : le dernier évènement est un arrêt enregistré à l'arrêt du service, à confirmer
	// au démarrage suivant (un simple redémarrage du service n'est pas un arrêt du PC).
	PendingShutdown bool                    `json:"pendingShutdown,omitempty"`
	Events          []protocol.HistoryEvent `json:"events"`
}

// Log est le journal, enregistré dans un fichier JSON.
type Log struct {
	mu   sync.Mutex
	path string
	now  func() time.Time
	st   state
	// Explain, s'il est renseigné, cherche la cause d'un arrêt non enregistré survenu après since
	// (SystemCause sous Windows) ; à renseigner avant Started.
	Explain func(since, now time.Time) Cause
	// lostAt / lostSince : arrêt non enregistré trouvé par Started et début de la recherche de sa
	// cause (pour Reexplain).
	lostAt, lostSince int64
}

// Open ouvre (ou crée) le journal. Un fichier illisible est remplacé par un journal vide.
func Open(path string, now func() time.Time) (*Log, error) {
	if now == nil {
		now = time.Now
	}
	l := &Log{path: path, now: now}
	raw, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, err
	default:
		if json.Unmarshal(raw, &l.st) != nil || l.st.Version != fileVersion {
			l.st = state{}
		}
	}
	if l.st.Since == 0 {
		l.st = state{Version: fileVersion, Since: now().Unix()}
	}
	l.prune()
	return l, nil
}

// Add ajoute un évènement et enregistre le journal.
func (l *Log) Add(e protocol.HistoryEvent) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.st.PendingShutdown = false
	l.insert(e)
	return l.save()
}

// AddWakes ajoute les démarrages demandés par une application (heures en secondes Unix), en ignorant
// ceux hors de la période couverte (à WakeLead près), dans le futur ou déjà connus. Renvoie le
// nombre d'ajouts.
func (l *Log) AddWakes(times []int64, client, by string) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.prune()
	now := l.now().Unix()
	added := 0
	for _, t := range times {
		if t < l.st.Since-int64(WakeLead.Seconds()) || t > now+60 || l.hasWake(t) {
			continue
		}
		l.insert(protocol.HistoryEvent{T: t, K: protocol.HistoryWake, C: client, B: by})
		added++
	}
	if added == 0 {
		return 0, nil
	}
	return added, l.save()
}

func (l *Log) hasWake(t int64) bool {
	window := int64(WakeMatch.Seconds())
	for _, e := range l.st.Events {
		if e.K == protocol.HistoryWake && e.T-t <= window && t-e.T <= window {
			return true
		}
	}
	return false
}

// Snapshot renvoie le début de la période couverte et les évènements (du plus ancien au plus récent).
func (l *Log) Snapshot() protocol.History {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.prune()
	events := slices.Clone(l.st.Events)
	if events == nil {
		events = []protocol.HistoryEvent{}
	}
	return protocol.History{From: l.st.Since, Events: events}
}

// Started enregistre le démarrage de l'agent. lastAlive est le dernier signe de vie connu (zéro si inconnu).
//
//   - Même démarrage du système (agent relancé ou mis à jour) : l'arrêt provisoire éventuellement
//     enregistré n'en était pas un, il est retiré.
//   - Nouveau démarrage : si l'arrêt précédent n'a pas été enregistré (coupure de courant, arrêt forcé),
//     un évènement « lost » est daté du dernier signe de vie ; puis le démarrage est ajouté à son heure réelle.
func (l *Log) Started(boot Boot, lastAlive time.Time) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	st := &l.st
	if l.sameBoot(boot) {
		if st.PendingShutdown && len(st.Events) > 0 && st.Events[len(st.Events)-1].K == protocol.HistoryShutdown {
			st.Events = st.Events[:len(st.Events)-1]
		}
		st.PendingShutdown = false
		st.BootID, st.Boot = boot.ID, boot.At.Unix()
		return l.save()
	}

	bootAt := boot.At.Unix()
	var last *protocol.HistoryEvent
	if n := len(st.Events); n > 0 {
		last = &st.Events[n-1]
	}
	// Cause d'un éventuel arrêt anormal : événements écrits par Windows depuis le dernier signe de vie.
	var cause Cause
	if st.Boot != 0 && l.Explain != nil {
		seen := time.Unix(st.Boot, 0).Add(10 * time.Minute)
		if !lastAlive.IsZero() && lastAlive.After(seen) {
			seen = lastAlive
		}
		if last != nil && last.T > seen.Unix() {
			seen = time.Unix(last.T, 0)
		}
		l.lostSince = explainWindow(seen).Unix()
		cause = l.Explain(time.Unix(l.lostSince, 0), l.now())
	}
	l.lostAt = 0
	lost := func(t int64) {
		l.insert(protocol.HistoryEvent{T: t, K: protocol.HistoryLost, R: cause.Reason, D: cause.Detail})
		l.lostAt = t
	}
	switch {
	case st.Boot != 0 && !lastAlive.IsZero() && lastAlive.Unix() < bootAt && !st.PendingShutdown &&
		(last == nil || (last.K != protocol.HistoryShutdown && last.K != protocol.HistorySleep && lastAlive.Unix() >= last.T)):
		// Extinction demandée mais arrêt du service non enregistré (arrêt très rapide) : c'est bien un
		// arrêt, sauf si Windows signale un arrêt anormal.
		if cause.Reason == "" && last != nil && last.K == protocol.HistoryCommand && (last.A == "shutdown" || last.A == "reboot") {
			l.insert(protocol.HistoryEvent{T: lastAlive.Unix(), K: protocol.HistoryShutdown})
		} else {
			lost(lastAlive.Unix())
		}
	case cause.Reason != "" && st.PendingShutdown && last != nil && last.K == protocol.HistoryShutdown:
		// Arrêt commencé proprement (service arrêté) mais terminé de force : blocage, bouton…
		last.K, last.R, last.D = protocol.HistoryLost, cause.Reason, cause.Detail
		l.lostAt = last.T
	case cause.Reason != "" && (last == nil || last.T < bootAt):
		// Arrêt anormal pendant la veille, ou dernier signe de vie inconnu.
		t := bootAt - 1
		if !lastAlive.IsZero() && lastAlive.Unix() < bootAt && (last == nil || lastAlive.Unix() > last.T) {
			t = lastAlive.Unix()
		} else if last != nil && last.T+1 < bootAt {
			t = last.T + 1
		}
		lost(t)
	}
	st.PendingShutdown = false
	l.insert(protocol.HistoryEvent{T: bootAt, K: protocol.HistoryBoot})
	st.BootID, st.Boot = boot.ID, bootAt
	if bootAt < st.Since {
		st.Since = bootAt
	}
	l.prune()
	return l.save()
}

// Reexplain cherche de nouveau la cause de l'arrêt non enregistré trouvé au démarrage : Windows écrit
// le code d'un écran bleu quelques minutes après le démarrage. Renvoie vrai si la cause a été précisée.
func (l *Log) Reexplain() (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.Explain == nil || l.lostAt == 0 {
		return false, nil
	}
	cause := l.Explain(time.Unix(l.lostSince, 0), l.now())
	for i := len(l.st.Events) - 1; i >= 0; i-- {
		e := &l.st.Events[i]
		if e.T != l.lostAt || e.K != protocol.HistoryLost {
			continue
		}
		if cause.rank() <= (Cause{Reason: e.R, Detail: e.D}).rank() {
			return false, nil
		}
		e.R, e.D = cause.Reason, cause.Detail
		return true, l.save()
	}
	return false, nil
}

// Stopping enregistre l'arrêt du service. Il est provisoire : s'il ne s'agissait que d'un
// redémarrage de l'agent, Started le retirera.
func (l *Log) Stopping() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.insert(protocol.HistoryEvent{T: l.now().Unix(), K: protocol.HistoryShutdown})
	l.st.PendingShutdown = true
	return l.save()
}

func (l *Log) sameBoot(boot Boot) bool {
	st := &l.st
	if st.Boot == 0 {
		return false
	}
	if st.BootID != "" && boot.ID != "" {
		return st.BootID == boot.ID
	}
	d := boot.At.Unix() - st.Boot
	return d > -int64(sameBootTolerance.Seconds()) && d < int64(sameBootTolerance.Seconds())
}

// insert ajoute un évènement en conservant l'ordre chronologique.
func (l *Log) insert(e protocol.HistoryEvent) {
	events := l.st.Events
	i := len(events)
	for i > 0 && events[i-1].T > e.T {
		i--
	}
	l.st.Events = slices.Insert(events, i, e)
	l.prune()
}

func (l *Log) prune() {
	limit := l.now().Add(-Retention).Unix()
	if l.st.Since < limit {
		l.st.Since = limit
	}
	events := l.st.Events
	start := 0
	for start < len(events) && events[start].T < limit {
		start++
	}
	if len(events)-start > MaxEvents {
		// Journal plein : la période couverte commence au plus ancien évènement conservé.
		start = len(events) - MaxEvents
		if events[start].T > l.st.Since {
			l.st.Since = events[start].T
		}
	}
	if start > 0 {
		l.st.Events = slices.Clone(events[start:])
	}
}

// save écrit le journal de façon atomique (fichier temporaire puis renommage).
func (l *Log) save() error {
	raw, err := json.Marshal(l.st)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0o700); err != nil {
		return err
	}
	tmp := l.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, l.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// ReadAlive lit le dernier signe de vie enregistré (zéro si inconnu).
func ReadAlive(path string) time.Time {
	raw, err := os.ReadFile(path)
	if err != nil {
		return time.Time{}
	}
	var v struct {
		T int64 `json:"t"`
	}
	if json.Unmarshal(raw, &v) != nil || v.T <= 0 {
		return time.Time{}
	}
	return time.Unix(v.T, 0)
}

// WriteAlive enregistre un signe de vie.
func WriteAlive(path string, t time.Time) error {
	raw, _ := json.Marshal(map[string]int64{"t": t.Unix()})
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

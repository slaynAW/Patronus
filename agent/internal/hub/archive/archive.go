// Package archive garde, pour chaque PC confié au hub, le journal de son agent et ses mesures minute par
// minute au-delà des 30 et 90 jours que garde l'agent (docs/HUB.md, §9) : un journal chiffré du stockage
// du hub par PC et par mois, en ajout seul. La durée de conservation est réglable ; un PC retiré du hub
// emporte son archive.
package archive

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/slaynaw/wakeonlan/agent/internal/hub/store"
	"github.com/slaynaw/wakeonlan/agent/protocol"
)

// Conservation (§9.1) : un an par défaut, de 3 mois à 3 ans.
const (
	DefaultRetentionDays = 365
	MinRetentionDays     = 90
	MaxRetentionDays     = 3 * 365
)

const (
	historyPrefix = "history-"
	metricsPrefix = "metrics-"
	suffix        = ".bin"
	// MaxEvents borne les évènements d'une réponse (hub.history), à reprendre ensuite.
	MaxEvents = 2000
)

var (
	idPattern  = regexp.MustCompile(`^[A-Za-z0-9_-]{22}$`)
	dayPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	// ErrBadPC : identifiant de PC hors format (jamais un chemin).
	ErrBadPC = errors.New("identifiant de PC invalide")
)

// Archive est l'archive de tous les PC, sûre pour un usage concurrent.
type Archive struct {
	st  *store.Store
	now func() time.Time

	mu     sync.Mutex
	events map[string]map[string][]protocol.HistoryEvent // évènements déjà gardés, par PC puis par mois
	days   map[string]map[string]bool                    // jours de mesures déjà gardés, par PC
}

// New ouvre l'archive dans le stockage st.
func New(st *store.Store, now func() time.Time) *Archive {
	return &Archive{st: st, now: now, events: map[string]map[string][]protocol.HistoryEvent{}, days: map[string]map[string]bool{}}
}

func dir(pc string) (string, error) {
	if !idPattern.MatchString(pc) {
		return "", ErrBadPC
	}
	return "pc/" + pc, nil
}

func month(t int64) string { return time.Unix(t, 0).UTC().Format("2006-01") }

// readMonth lit les évènements gardés d'un mois ; une précision ajoutée plus tard remplace l'évènement.
func (a *Archive) readMonth(pc, m string) ([]protocol.HistoryEvent, error) {
	d, err := dir(pc)
	if err != nil {
		return nil, err
	}
	records, err := a.st.Records(d + "/" + historyPrefix + m + suffix)
	if err != nil {
		return nil, err
	}
	var evs []protocol.HistoryEvent
	for _, r := range records {
		var e protocol.HistoryEvent
		if json.Unmarshal(r, &e) != nil {
			return nil, fmt.Errorf("archive du PC %s illisible (%s)", pc, m)
		}
		evs, _ = protocol.MergeEvent(evs, e)
	}
	return evs, nil
}

// load lit les évènements d'un mois où l'on ajoute (sous verrou), gardés en mémoire : seuls les mois que
// couvre le journal d'un agent (30 jours) y restent, les plus anciens sont oubliés.
func (a *Archive) load(pc, m string) ([]protocol.HistoryEvent, error) {
	if evs, ok := a.events[pc][m]; ok {
		return evs, nil
	}
	evs, err := a.readMonth(pc, m)
	if err != nil {
		return nil, err
	}
	if a.events[pc] == nil {
		a.events[pc] = map[string][]protocol.HistoryEvent{}
	}
	oldest := month(a.now().AddDate(0, 0, -62).Unix())
	for kept := range a.events[pc] {
		if kept < oldest {
			delete(a.events[pc], kept)
		}
	}
	a.events[pc][m] = evs
	return evs, nil
}

// AddEvents garde les évènements du journal d'un agent qui manquent à l'archive, ou qui précisent un
// évènement déjà gardé (cause d'un arrêt anormal) ; renvoie le nombre d'évènements ajoutés ou précisés.
func (a *Archive) AddEvents(pc string, events []protocol.HistoryEvent) (int, error) {
	d, err := dir(pc)
	if err != nil {
		return 0, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	byMonth := map[string][][]byte{}
	changed := 0
	for _, e := range events {
		m := month(e.T)
		evs, err := a.load(pc, m)
		if err != nil {
			return changed, err
		}
		next, added := protocol.MergeEvent(slices.Clone(evs), e)
		if !added {
			continue
		}
		raw, err := json.Marshal(e)
		if err != nil {
			return changed, err
		}
		byMonth[m] = append(byMonth[m], raw)
		a.events[pc][m] = next
		changed++
	}
	for m, records := range byMonth {
		if err := a.st.Append(d+"/"+historyPrefix+m+suffix, records...); err != nil {
			delete(a.events[pc], m) // relu au prochain accès
			return changed, err
		}
	}
	return changed, nil
}

// Events renvoie les évènements gardés d'un PC à partir de from (secondes Unix), dans l'ordre, MaxEvents
// au plus : more indique qu'il en reste après le dernier. first est le plus ancien évènement gardé (0 si
// aucun).
func (a *Archive) Events(pc string, from int64) (events []protocol.HistoryEvent, more bool, first int64, err error) {
	d, err := dir(pc)
	if err != nil {
		return nil, false, 0, err
	}
	names, err := a.st.List(d)
	if err != nil {
		return nil, false, 0, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, name := range names {
		m, ok := strings.CutPrefix(name, historyPrefix)
		if !ok {
			continue
		}
		m = strings.TrimSuffix(m, suffix)
		evs, err := a.readMonth(pc, m)
		if err != nil {
			return nil, false, 0, err
		}
		for _, e := range evs {
			if first == 0 || e.T < first {
				first = e.T
			}
			if e.T >= from {
				events = append(events, e)
			}
		}
	}
	slices.SortStableFunc(events, func(x, y protocol.HistoryEvent) int { return cmp.Compare(x.T, y.T) })
	if len(events) > MaxEvents {
		return events[:MaxEvents], true, first, nil
	}
	return events, false, first, nil
}

// metricsDay est un jour de mesures gardé.
type metricsDay struct {
	Day  string                `json:"day"`
	Rows []protocol.MetricsRow `json:"rows"`
}

// HasDay indique un jour de mesures déjà gardé.
func (a *Archive) HasDay(pc, day string) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	days, err := a.loadDays(pc)
	return days[day], err
}

func (a *Archive) loadDays(pc string) (map[string]bool, error) {
	if days, ok := a.days[pc]; ok {
		return days, nil
	}
	d, err := dir(pc)
	if err != nil {
		return nil, err
	}
	names, err := a.st.List(d)
	if err != nil {
		return nil, err
	}
	days := map[string]bool{}
	for _, name := range names {
		if !strings.HasPrefix(name, metricsPrefix) {
			continue
		}
		records, err := a.st.Records(d + "/" + name)
		if err != nil {
			return nil, err
		}
		for _, r := range records {
			var md metricsDay
			if json.Unmarshal(r, &md) != nil {
				return nil, fmt.Errorf("mesures du PC %s illisibles (%s)", pc, name)
			}
			days[md.Day] = true
		}
	}
	a.days[pc] = days
	return days, nil
}

// AddDay garde les mesures d'un jour (« 2026-10-03 », UTC), une seule fois.
func (a *Archive) AddDay(pc, day string, rows []protocol.MetricsRow) error {
	d, err := dir(pc)
	if err != nil {
		return err
	}
	if !dayPattern.MatchString(day) {
		return fmt.Errorf("jour invalide : %q", day)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	days, err := a.loadDays(pc)
	if err != nil || days[day] {
		return err
	}
	raw, err := json.Marshal(metricsDay{Day: day, Rows: rows})
	if err != nil {
		return err
	}
	if err := a.st.Append(d+"/"+metricsPrefix+day[:7]+suffix, raw); err != nil {
		delete(a.days, pc)
		return err
	}
	days[day] = true
	return nil
}

// Days renvoie les jours de mesures gardés d'un PC, du plus ancien au plus récent.
func (a *Archive) Days(pc string) ([]string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	days, err := a.loadDays(pc)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(days))
	for day := range days {
		out = append(out, day)
	}
	slices.Sort(out)
	return out, nil
}

// Day renvoie les mesures gardées d'un jour (nil s'il n'est pas gardé).
func (a *Archive) Day(pc, day string) ([]protocol.MetricsRow, error) {
	d, err := dir(pc)
	if err != nil {
		return nil, err
	}
	if !dayPattern.MatchString(day) {
		return nil, fmt.Errorf("jour invalide : %q", day)
	}
	records, err := a.st.Records(d + "/" + metricsPrefix + day[:7] + suffix)
	if err != nil {
		return nil, err
	}
	for _, r := range records {
		var md metricsDay
		if json.Unmarshal(r, &md) != nil {
			return nil, fmt.Errorf("mesures du PC %s illisibles (%s)", pc, day[:7])
		}
		if md.Day == day {
			return md.Rows, nil
		}
	}
	return nil, nil
}

// Prune retire les mois entièrement plus anciens que retentionDays, pour tous les PC ; renvoie le
// nombre de fichiers retirés.
func (a *Archive) Prune(retentionDays int) (int, error) {
	limit := a.now().AddDate(0, 0, -retentionDays).UTC().Format("2006-01")
	pcs, err := a.st.Dirs("pc")
	if err != nil {
		return 0, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	removed := 0
	for _, pc := range pcs {
		names, err := a.st.List("pc/" + pc)
		if err != nil {
			return removed, err
		}
		for _, name := range names {
			m := strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(name, historyPrefix), metricsPrefix), suffix)
			// Un mois est retiré quand il précède entièrement le mois de la limite.
			if len(m) != 7 || m >= limit {
				continue
			}
			if err := a.st.Remove("pc/" + pc + "/" + name); err != nil {
				return removed, err
			}
			removed++
		}
		delete(a.events, pc)
		delete(a.days, pc)
	}
	return removed, nil
}

// RemovePC retire toute l'archive d'un PC (PC retiré du hub).
func (a *Archive) RemovePC(pc string) error {
	d, err := dir(pc)
	if err != nil {
		return err
	}
	names, err := a.st.List(d)
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, name := range names {
		if err := a.st.Remove(d + "/" + name); err != nil {
			return err
		}
	}
	delete(a.events, pc)
	delete(a.days, pc)
	return nil
}

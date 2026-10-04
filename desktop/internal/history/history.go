// Package history tient l'historique des démarrages et extinctions des PC sur 30 jours (mêmes
// règles que core/history côté Android).
//
// Deux sources sont fusionnées :
//   - l'application : ses propres demandes (démarrage, extinction...) et les changements d'état
//     constatés pendant la surveillance (heure approximative) ;
//   - le journal de l'agent (commande « history ») : démarrages, arrêts, veille, à l'heure exacte,
//     même quand l'application était fermée. Il fait foi sur la période qu'il couvre.
package history

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol"
)

// Kind est le type d'un évènement.
type Kind string

const (
	On           Kind = "on"           // allumé
	Off          Kind = "off"          // éteint
	Lost         Kind = "lost"         // arrêt non enregistré (coupure de courant, arrêt forcé)
	Sleep        Kind = "sleep"        // mis en veille
	Resume       Kind = "resume"       // sorti de veille
	WakeSent     Kind = "wake"         // démarrage demandé (paquet magique)
	ShutdownSent Kind = "shutdown_req" // extinction demandée
	RebootSent   Kind = "reboot_req"   // redémarrage demandé
	SleepSent    Kind = "sleep_req"    // mise en veille demandée
	WakeTimeout  Kind = "wake_timeout" // pas de réponse après le démarrage demandé
)

// Source est l'origine d'un évènement.
type Source string

const (
	App   Source = "app"
	Agent Source = "agent"
)

// Event est un évènement de l'historique. Time est en millisecondes (heure Unix).
type Event struct {
	Device string `json:"d"`
	Time   int64  `json:"t"`
	Kind   Kind   `json:"k"`
	Source Source `json:"s"`
	// Approx : heure constatée par l'application (à quelques secondes près), pas mesurée par le PC.
	Approx bool `json:"x,omitempty"`
	// Client : pour une demande notée par l'agent, nom de l'appareil qui l'a faite (à défaut, son adresse).
	Client string `json:"c,omitempty"`
	// Cause et Detail : cause d'un arrêt anormal lue par l'agent dans le journal d'événements de
	// Windows (protocol.LostBSOD…) et code de l'écran bleu.
	Cause  string `json:"r,omitempty"`
	Detail string `json:"rd,omitempty"`
}

// Coverage est la période couverte par le journal de l'agent d'un PC (millisecondes).
type Coverage struct {
	From  int64 `json:"from"`
	Until int64 `json:"until"`
}

// Data est l'historique enregistré.
type Data struct {
	Version  int                 `json:"version"`
	Events   []Event             `json:"events"`
	Coverage map[string]Coverage `json:"coverage,omitempty"`
}

const (
	// Retention est la durée de conservation.
	Retention = 30 * 24 * time.Hour
	// MaxEvents borne la taille de l'historique enregistré.
	MaxEvents = 20_000
	// MatchWindow : une commande vue par l'agent à moins de cet écart d'une demande faite depuis cette
	// application est la même (elle n'est affichée qu'une fois).
	MatchWindow = time.Minute
	// WakeLead : un démarrage demandé est accepté par l'agent jusqu'à cet écart avant le début de son journal.
	WakeLead = 10 * time.Minute
	// MaxReportedWakes : démarrages signalés à l'agent en une fois (protocol.MaxWakes).
	MaxReportedWakes = protocol.MaxWakes
	// MaxBackupEvents : évènements joints à une sauvegarde complète (environ 300 Ko).
	MaxBackupEvents = 5_000
	version         = 1
)

// New renvoie un historique vide.
func New() Data { return Data{Version: version, Events: []Event{}, Coverage: map[string]Coverage{}} }

// Add ajoute un évènement.
func (d Data) Add(e Event, now int64) Data {
	d = d.clone()
	d.Events = append(d.Events, e)
	return d.prune(now)
}

// ReplaceAgent remplace le journal de l'agent d'un PC par sa dernière version lue.
func (d Data) ReplaceAgent(device string, h protocol.History, fetchedAt, now int64) Data {
	d = d.clone()
	kept := d.Events[:0]
	for _, e := range d.Events {
		if e.Device != device || e.Source != Agent {
			kept = append(kept, e)
		}
	}
	d.Events = kept
	for _, raw := range h.Events {
		if e, ok := FromAgent(device, raw); ok {
			d.Events = append(d.Events, e)
		}
	}
	d.Coverage[device] = Coverage{From: h.From * 1000, Until: fetchedAt}
	return d.prune(now)
}

// ForBackup renvoie l'historique joint à une sauvegarde complète : les MaxBackupEvents évènements les
// plus récents, pour que le fichier reste sous la taille acceptée à l'import (1 Mo), même par les
// anciennes versions.
func (d Data) ForBackup(now int64) Data {
	d = d.clone().prune(now)
	if len(d.Events) > MaxBackupEvents {
		slices.SortStableFunc(d.Events, func(a, b Event) int { return cmp.Compare(a.Time, b.Time) })
		d.Events = slices.Clone(d.Events[len(d.Events)-MaxBackupEvents:])
	}
	return d
}

// Merge ajoute un historique importé (sauvegarde d'un autre appareil) : les évènements déjà présents
// ne sont pas dupliqués ; la période couverte par le journal de chaque agent est étendue.
func (d Data) Merge(other Data, now int64) Data {
	d = d.clone()
	known := make(map[Event]bool, len(d.Events))
	for _, e := range d.Events {
		known[e] = true
	}
	for _, e := range other.Events {
		if !e.valid() {
			continue
		}
		if !known[e] {
			known[e] = true
			d.Events = append(d.Events, e)
		}
	}
	for device, c := range other.Coverage {
		if mine, ok := d.Coverage[device]; ok {
			c = Coverage{From: min(mine.From, c.From), Until: max(mine.Until, c.Until)}
		}
		d.Coverage[device] = c
	}
	return d.prune(now)
}

// UnreportedWakes renvoie les démarrages demandés depuis cette application que le journal de l'agent
// d'un PC ne contient pas encore (heures en secondes), à lui signaler (agent 1.4.0 ou plus). Ceux qui
// précèdent le début du journal (à WakeLead près) sont ignorés : l'agent les refuserait.
func (d Data) UnreportedWakes(device string, h protocol.History) []int64 {
	window := MatchWindow.Milliseconds()
	from := h.From*1000 - WakeLead.Milliseconds()
	var out []int64
	for _, e := range d.Events {
		if e.Device != device || e.Source != App || e.Kind != WakeSent || e.Time < from {
			continue
		}
		known := false
		for _, raw := range h.Events {
			if raw.K == protocol.HistoryWake && raw.T*1000-e.Time <= window && e.Time-raw.T*1000 <= window {
				known = true
				break
			}
		}
		if t := e.Time / 1000; !known && !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	if len(out) > MaxReportedWakes {
		out = out[len(out)-MaxReportedWakes:]
	}
	return out
}

// Keep ne conserve que les PC encore configurés.
func (d Data) Keep(ids map[string]bool) Data {
	d = d.clone()
	kept := d.Events[:0]
	for _, e := range d.Events {
		if ids[e.Device] {
			kept = append(kept, e)
		}
	}
	d.Events = kept
	for id := range d.Coverage {
		if !ids[id] {
			delete(d.Coverage, id)
		}
	}
	return d
}

// View renvoie les évènements à afficher (tous les PC si device est vide), du plus récent au plus ancien :
//   - le journal de l'agent fait foi sur la période qu'il couvre : les changements d'état constatés
//     par l'application pendant cette période sont masqués ;
//   - une demande notée par l'agent qui correspond à une demande faite depuis cette application n'est
//     affichée qu'une fois (celle de l'application) ; celles des autres appareils restent.
func (d Data) View(device string, now int64) []Event {
	limit := now - Retention.Milliseconds()
	var out []Event
	// Parcours du plus récent ajouté au plus ancien : à heure égale, le dernier enregistré passe devant.
	for i := len(d.Events) - 1; i >= 0; i-- {
		e := d.Events[i]
		if (device != "" && e.Device != device) || e.Time < limit {
			continue
		}
		if e.Source == App && (e.Kind == On || e.Kind == Off) {
			if c, ok := d.Coverage[e.Device]; ok && e.Time >= c.From && e.Time <= c.Until {
				continue
			}
		}
		if e.Source == Agent && isRequest(e.Kind) && d.hasOwnRequest(e) {
			continue
		}
		out = append(out, e)
	}
	slices.SortStableFunc(out, func(a, b Event) int { return cmp.Compare(b.Time, a.Time) })
	if out == nil {
		out = []Event{}
	}
	return out
}

func (d Data) hasOwnRequest(agentEvent Event) bool {
	window := MatchWindow.Milliseconds()
	for _, e := range d.Events {
		if e.Source == App && e.Device == agentEvent.Device && e.Kind == agentEvent.Kind &&
			e.Time-agentEvent.Time <= window && agentEvent.Time-e.Time <= window {
			return true
		}
	}
	return false
}

// isRequest : demande faite depuis une application (notée aussi par l'agent).
func isRequest(k Kind) bool {
	return k == ShutdownSent || k == RebootSent || k == SleepSent || k == WakeSent
}

// FromAgent convertit un évènement du journal de l'agent.
func FromAgent(device string, raw protocol.HistoryEvent) (Event, bool) {
	e := Event{Device: device, Time: raw.T * 1000, Source: Agent}
	switch raw.K {
	case protocol.HistoryBoot:
		e.Kind = On
	case protocol.HistoryShutdown:
		e.Kind = Off
	case protocol.HistoryLost:
		e.Kind = Lost
		switch raw.R {
		case protocol.LostBSOD, protocol.LostButton, protocol.LostPower, protocol.LostHardware:
			e.Cause = raw.R
			if len(raw.D) <= maxDetail {
				e.Detail = raw.D
			}
		}
	case protocol.HistorySleep:
		e.Kind = Sleep
	case protocol.HistoryResume:
		e.Kind = Resume
	case protocol.HistoryCommand:
		switch raw.A {
		case "shutdown":
			e.Kind = ShutdownSent
		case "reboot":
			e.Kind = RebootSent
		case "sleep":
			e.Kind = SleepSent
		default:
			return Event{}, false
		}
	case protocol.HistoryWake:
		e.Kind = WakeSent
	default:
		return Event{}, false
	}
	if isRequest(e.Kind) {
		e.Client = raw.C
		if strings.TrimSpace(raw.B) != "" {
			e.Client = raw.B
		}
	}
	return e, true
}

// valid écarte les évènements d'une sauvegarde qu'une version plus récente aurait pu ajouter.
func (e Event) valid() bool {
	switch e.Kind {
	case On, Off, Lost, Sleep, Resume, WakeSent, ShutdownSent, RebootSent, SleepSent, WakeTimeout:
	default:
		return false
	}
	return (e.Source == App || e.Source == Agent) && e.Device != "" && len(e.Client) <= 256 && len(e.Detail) <= maxDetail
}

// maxDetail borne le détail de la cause d'un arrêt anormal.
const maxDetail = 128

func (d Data) clone() Data {
	out := Data{Version: version, Events: slices.Clone(d.Events), Coverage: make(map[string]Coverage, len(d.Coverage))}
	for k, v := range d.Coverage {
		out.Coverage[k] = v
	}
	if out.Events == nil {
		out.Events = []Event{}
	}
	return out
}

// prune retire les évènements de plus de 30 jours et borne la taille (les plus anciens partent).
func (d Data) prune(now int64) Data {
	limit := now - Retention.Milliseconds()
	kept := d.Events[:0]
	for _, e := range d.Events {
		if e.Time >= limit {
			kept = append(kept, e)
		}
	}
	d.Events = kept
	if len(d.Events) > MaxEvents {
		slices.SortStableFunc(d.Events, func(a, b Event) int { return cmp.Compare(a.Time, b.Time) })
		d.Events = slices.Clone(d.Events[len(d.Events)-MaxEvents:])
	}
	return d
}

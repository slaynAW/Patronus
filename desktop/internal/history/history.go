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
	// Client : pour une commande reçue par l'agent, adresse de l'appareil qui l'a envoyée.
	Client string `json:"c,omitempty"`
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
	version     = 1
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
//   - une commande reçue par l'agent qui correspond à une demande faite depuis cette application n'est
//     affichée qu'une fois (la demande).
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

func isRequest(k Kind) bool { return k == ShutdownSent || k == RebootSent || k == SleepSent }

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
		e.Client = raw.C
	default:
		return Event{}, false
	}
	return e, true
}

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

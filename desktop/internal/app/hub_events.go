package app

import (
	"slices"

	"github.com/slaynaw/wakeonlan/agent/hubprotocol"
	"github.com/slaynaw/wakeonlan/desktop/internal/diag"
	"github.com/slaynaw/wakeonlan/desktop/internal/hubstore"
)

// « Pendant votre absence » (docs/HUB.md, §12) : les évènements détectés par le hub depuis que
// l'utilisateur les a vus pour la dernière fois. Ce sont des codes (type, PC, heure, précision) :
// l'interface rédige le texte avec les noms de PC de cet ordinateur et n'affiche jamais de texte libre
// ni de lien venus du hub (T38). Un évènement fait d'autre chose que de codes connus est écarté
// (hubprotocol.ValidEvent, mêmes vecteurs que l'application Android) ; seul le nom d'un appareil sans
// agent, que cet ordinateur ne connaît pas, vient du hub.

// hubEventsState : évènements non vus du hub d'empreinte pin (verrou hubMu).
type hubEventsState struct {
	pin    string
	events []hubprotocol.Event
	more   bool
}

// eventsSince renvoie le début des évènements à demander au hub h : après le dernier vu, ou depuis
// l'appairage.
func eventsSince(h hubstore.Hub) int64 {
	if h.EventsSeen > 0 {
		return h.EventsSeen + 1
	}
	return h.Paired
}

// keepEvents garde les évènements non vus lus sur le hub h (réponse à hub.events), sans ceux qu'il ne
// devrait pas donner (type inconnu, précision inattendue).
func (s *Service) keepEvents(h hubstore.Hub, resp hubprotocol.Response) {
	since := eventsSince(h)
	kept := make([]hubprotocol.Event, 0, len(resp.Events))
	for _, e := range resp.Events {
		if e.T >= since && hubprotocol.ValidEvent(e) {
			kept = append(kept, e)
		}
	}
	if dropped := len(resp.Events) - len(kept); dropped > 0 {
		diag.Warn(areaHub, "hub « %s » : %d évènement(s) inattendu(s) écarté(s)", h.Name, dropped)
	}
	s.hubMu.Lock()
	s.hubEvents = hubEventsState{pin: h.Pin, events: kept, more: resp.More}
	s.hubMu.Unlock()
}

// unseenEvents renvoie les évènements non vus du hub appairé.
func (s *Service) unseenEvents() ([]hubprotocol.Event, bool) {
	s.hubMu.Lock()
	defer s.hubMu.Unlock()
	if s.hub == nil || s.hubEvents.pin != s.hub.Pin {
		return nil, false
	}
	return slices.Clone(s.hubEvents.events), s.hubEvents.more
}

// hubEventsView décrit les évènements non vus pour l'interface, du plus récent au plus ancien, avec le
// nom du PC : celui de cet ordinateur pour un PC qu'il connaît (local : son identifiant), sinon celui
// gardé par le hub (hubName).
func (s *Service) hubEventsView() map[string]any {
	h, ok := s.currentHub()
	if !ok {
		return hubFailure(errNoHub)
	}
	events, more := s.unseenEvents()
	pcs, _ := s.hubPCs()
	byID := map[string]hubprotocol.PCInfo{}
	for _, p := range pcs {
		byID[p.ID] = p
	}
	local := map[string][2]string{} // empreinte de l'agent → identifiant et nom du PC
	for _, d := range s.ownDevices() {
		if d.Agent != nil && d.Agent.HasPin() {
			local[d.Agent.Pin] = [2]string{d.ID, d.Name}
		}
	}
	self := ""
	if id := s.identity.Existing(); id != nil {
		self = id.Pin.Short()
	}
	out := make([]map[string]any, 0, len(events))
	for i := len(events) - 1; i >= 0; i-- {
		e := events[i]
		v := map[string]any{"t": e.T, "k": e.K, "x": e.X, "i": e.I, "self": e.I != "" && e.I == self}
		if p, ok := byID[e.PC]; ok {
			if d, mine := local[p.Agent]; mine && p.Agent != "" {
				v["local"], v["name"] = d[0], d[1]
			} else {
				v["name"], v["hubName"] = p.Name, true
			}
		}
		out = append(out, v)
	}
	return map[string]any{"ok": true, "hub": h.Name, "events": out, "more": more}
}

// hubEventsSeen marque comme vus les évènements jusqu'à until (horloge du hub, secondes Unix).
func (s *Service) hubEventsSeen(until int64) map[string]any {
	h, ok := s.currentHub()
	if !ok {
		return hubFailure(errNoHub)
	}
	if until <= h.EventsSeen {
		return map[string]any{"ok": true}
	}
	s.hubMu.Lock()
	if s.hubEvents.pin == h.Pin {
		s.hubEvents.events = slices.DeleteFunc(s.hubEvents.events, func(e hubprotocol.Event) bool { return e.T <= until })
		if len(s.hubEvents.events) == 0 {
			s.hubEvents.more = false
		}
	}
	s.hubMu.Unlock()
	s.updateHub(h.Pin, func(n *hubstore.Hub) { n.EventsSeen = until })
	s.refreshHubSoon()
	return map[string]any{"ok": true}
}

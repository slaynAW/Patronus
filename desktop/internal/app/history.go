package app

import (
	"context"
	"time"

	"github.com/slaynaw/wakeonlan/desktop/internal/agentclient"
	"github.com/slaynaw/wakeonlan/desktop/internal/history"
	"github.com/slaynaw/wakeonlan/desktop/internal/status"
)

// État de la lecture du journal d'un agent, pour l'interface.
const (
	AgentHistoryOK          = "ok"          // journal lu
	AgentHistoryOutdated    = "outdated"    // agent trop ancien : à mettre à jour pour l'historique complet
	AgentHistoryUnreachable = "unreachable" // agent injoignable pour l'instant
)

const (
	// agentFetchInterval espace les lectures automatiques du journal d'un même agent.
	agentFetchInterval = 5 * time.Minute
	// agentFetchForced : délai minimal entre deux lectures demandées par l'interface.
	agentFetchForced  = 20 * time.Second
	agentFetchTimeout = 8 * time.Second
)

type agentFetchState struct {
	last    time.Time
	running bool
	status  string
}

// HistoryItem est un évènement affiché par l'interface.
type HistoryItem struct {
	Device string         `json:"device"`
	Name   string         `json:"name"`
	Time   int64          `json:"time"`
	Kind   history.Kind   `json:"kind"`
	Source history.Source `json:"source"`
	Approx bool           `json:"approx,omitempty"`
	Client string         `json:"client,omitempty"`
}

// onStatusChange est appelé à chaque relevé : il déduit les allumages / extinctions constatés et
// relit le journal d'un agent dès que son PC répond (ce qui s'est passé application fermée).
func (s *Service) onStatusChange() {
	s.statusMu.Lock()
	statuses := s.monitor.Statuses()
	prev := s.lastStatuses
	s.lastStatuses = statuses
	if prev != nil {
		if events := history.Transitions(prev, statuses, s.now().UnixMilli()); len(events) > 0 {
			s.recordAll(events)
		}
		for id, st := range statuses {
			if st.State == status.Online && prev[id].State != status.Online {
				s.refreshAgentHistory(id, false)
			}
		}
	}
	s.statusMu.Unlock()
	s.notify()
}

func requestKind(action agentclient.Action) history.Kind {
	switch action {
	case agentclient.Reboot:
		return history.RebootSent
	case agentclient.Sleep:
		return history.SleepSent
	}
	return history.ShutdownSent
}

// record ajoute un évènement daté de maintenant.
func (s *Service) record(e history.Event) {
	e.Time = s.now().UnixMilli()
	s.recordAll([]history.Event{e})
	s.notify()
}

func (s *Service) recordAll(events []history.Event) {
	s.histMu.Lock()
	defer s.histMu.Unlock()
	now := s.now().UnixMilli()
	for _, e := range events {
		s.hist = s.hist.Add(e, now)
	}
	s.histVersion++
	s.saveHistoryLocked()
}

func (s *Service) saveHistoryLocked() {
	if s.histStore != nil {
		_ = s.histStore.Save(s.hist) // simple journal : une écriture manquée n'est pas bloquante
	}
}

func (s *Service) keepHistory(ids map[string]bool) {
	s.histMu.Lock()
	defer s.histMu.Unlock()
	s.hist = s.hist.Keep(ids)
	for id := range s.agentFetch {
		if !ids[id] {
			delete(s.agentFetch, id)
		}
	}
	s.histVersion++
	s.saveHistoryLocked()
}

func (s *Service) clearHistory() {
	s.histMu.Lock()
	s.hist = history.New()
	s.agentFetch = map[string]*agentFetchState{}
	s.histVersion++
	s.saveHistoryLocked()
	s.histMu.Unlock()
	s.notify()
}

// refreshAgentHistories relit le journal d'un agent (id) ou de tous (id vide).
func (s *Service) refreshAgentHistories(id string, force bool) {
	s.mu.Lock()
	var ids []string
	for _, d := range s.cfg.Devices {
		if id == "" || d.ID == id {
			ids = append(ids, d.ID)
		}
	}
	s.mu.Unlock()
	for _, target := range ids {
		s.refreshAgentHistory(target, force)
	}
}

// refreshAgentHistory lit le journal de l'agent d'un PC en arrière-plan (sans effet si le PC n'a pas
// d'agent, ou si une lecture récente existe).
func (s *Service) refreshAgentHistory(id string, force bool) {
	d, ok := s.device(id)
	if !ok || d.Agent == nil || !d.Agent.HasKey() || !d.HasHost() {
		return
	}
	s.histMu.Lock()
	st := s.agentFetch[id]
	if st == nil {
		st = &agentFetchState{}
		s.agentFetch[id] = st
	}
	gap := agentFetchInterval
	if force {
		gap = agentFetchForced
	}
	now := s.now()
	if st.running || (!st.last.IsZero() && now.Sub(st.last) < gap) {
		s.histMu.Unlock()
		return
	}
	st.running, st.last = true, now
	s.histMu.Unlock()

	host, settings := d.Host, *d.Agent
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), agentFetchTimeout)
		defer cancel()
		h, err := s.agent.History(ctx, host, settings)
		_, stillThere := s.device(id)
		s.histMu.Lock()
		st.running = false
		switch {
		case err == nil && stillThere && s.agentFetch[id] == st:
			st.status = AgentHistoryOK
			now := s.now().UnixMilli()
			s.hist = s.hist.ReplaceAgent(id, h, now, now)
			s.saveHistoryLocked()
		case agentclient.CodeOf(err) == agentclient.Rejected:
			st.status = AgentHistoryOutdated
		case err != nil:
			st.status = AgentHistoryUnreachable
		}
		s.histVersion++
		s.histMu.Unlock()
		s.notify()
	}()
}

// historyView renvoie l'historique affiché (un PC, ou tous si id est vide).
func (s *Service) historyView(id string) map[string]any {
	s.mu.Lock()
	names := map[string]string{}
	hasAgent := false
	for _, d := range s.cfg.Devices {
		names[d.ID] = d.Name
		if d.ID == id && d.Agent != nil && d.Agent.HasKey() {
			hasAgent = true
		}
	}
	s.mu.Unlock()

	s.histMu.Lock()
	events := s.hist.View(id, s.now().UnixMilli())
	agentStatus := ""
	if st := s.agentFetch[id]; st != nil {
		agentStatus = st.status
	}
	version := s.histVersion
	s.histMu.Unlock()

	items := make([]HistoryItem, 0, len(events))
	for _, e := range events {
		name, ok := names[e.Device]
		if !ok {
			continue
		}
		items = append(items, HistoryItem{Device: e.Device, Name: name, Time: e.Time, Kind: e.Kind, Source: e.Source, Approx: e.Approx, Client: e.Client})
	}
	return map[string]any{"events": items, "hasAgent": hasAgent, "agent": agentStatus, "version": version}
}

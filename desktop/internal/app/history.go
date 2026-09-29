package app

import (
	"context"
	"fmt"
	"time"

	"github.com/slaynaw/wakeonlan/desktop/internal/agentclient"
	"github.com/slaynaw/wakeonlan/desktop/internal/diag"
	"github.com/slaynaw/wakeonlan/desktop/internal/history"
	"github.com/slaynaw/wakeonlan/desktop/internal/model"
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
	// wakesUnsupported : agent antérieur à 1.4.0, les démarrages ne lui sont plus signalés.
	wakesUnsupported bool
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
		s.logTransitions(prev, statuses)
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

// logTransitions note dans le journal les changements d'état des PC (et de l'erreur de leur agent).
func (s *Service) logTransitions(prev, next map[string]status.DeviceStatus) {
	for id, st := range next {
		old, known := prev[id]
		if known && old.State == st.State && old.AgentError == st.AgentError && old.Notice == st.Notice {
			continue
		}
		name := "[" + shortID(id) + "]"
		if d, ok := s.device(id); ok {
			name = "« " + d.Name + " »"
		}
		from := "-"
		if known {
			from = string(old.State)
		}
		detail := ""
		if st.Method != "" {
			detail += ", via " + string(st.Method)
		}
		if st.LatencyMs != nil {
			detail += fmt.Sprintf(" (%d ms)", *st.LatencyMs)
		}
		if st.AgentError != "" {
			detail += ", agent : " + string(st.AgentError)
		}
		if st.UnknownReason != "" {
			detail += ", raison : " + string(st.UnknownReason)
		}
		if st.Notice != "" {
			detail += ", avis : " + string(st.Notice)
		}
		diag.Info(areaStatus, "%s : %s → %s%s", name, from, st.State, detail)
	}
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
		// Simple journal : une écriture manquée n'est pas bloquante.
		if err := s.histStore.Save(s.hist); err != nil {
			diag.Warn(areaHistory, "historique non enregistré : %v", err)
		}
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

// mergeHistory ajoute l'historique d'une sauvegarde importée (sans doublon), pour les PC configurés.
func (s *Service) mergeHistory(h history.Data) {
	ids := map[string]bool{}
	for _, d := range s.allDevices() {
		ids[d.ID] = true
	}
	s.histMu.Lock()
	s.hist = s.hist.Merge(h, s.now().UnixMilli()).Keep(ids)
	s.histVersion++
	s.saveHistoryLocked()
	s.histMu.Unlock()
	s.notify()
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
	var ids []string
	for _, d := range s.allDevices() {
		if id == "" || d.ID == id {
			ids = append(ids, d.ID)
		}
	}
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
		var wakes []int64
		s.histMu.Lock()
		switch {
		case err == nil && stillThere && s.agentFetch[id] == st:
			st.status = AgentHistoryOK
			now := s.now().UnixMilli()
			s.hist = s.hist.ReplaceAgent(id, h, now, now)
			s.saveHistoryLocked()
			if !st.wakesUnsupported {
				wakes = s.hist.UnreportedWakes(id, h)
			}
		case agentclient.CodeOf(err) == agentclient.Rejected:
			st.status = AgentHistoryOutdated
			diag.Info(areaHistory, "journal de l'agent de « %s » : agent trop ancien", d.Name)
		case err != nil:
			st.status = AgentHistoryUnreachable
			diag.Info(areaHistory, "journal de l'agent de « %s » illisible : %s (%v)", d.Name, agentclient.CodeOf(err), err)
		}
		s.histVersion++
		s.histMu.Unlock()
		s.notify()
		if len(wakes) > 0 {
			s.reportWakes(id, st, host, settings, wakes)
		}
		s.histMu.Lock()
		st.running = false
		s.histMu.Unlock()
	}()
}

// reportWakes signale à l'agent les démarrages demandés depuis cette application, qu'il ne peut pas
// voir (paquet magique) : tous les appareils affichent ainsi le même historique (agent 1.4.0 ou plus).
func (s *Service) reportWakes(id string, st *agentFetchState, host string, settings model.AgentSettings, wakes []int64) {
	ctx, cancel := context.WithTimeout(context.Background(), agentFetchTimeout)
	defer cancel()
	h, err := s.agent.ReportWakes(ctx, host, settings, wakes)
	_, stillThere := s.device(id)
	s.histMu.Lock()
	switch {
	case err == nil && stillThere && s.agentFetch[id] == st:
		now := s.now().UnixMilli()
		s.hist = s.hist.ReplaceAgent(id, h, now, now)
		s.saveHistoryLocked()
		s.histVersion++
	case agentclient.CodeOf(err) == agentclient.Rejected:
		st.wakesUnsupported = true
	case err != nil:
		diag.Info(areaHistory, "démarrages non signalés à l'agent de [%s] : %s (%v)", shortID(id), agentclient.CodeOf(err), err)
	}
	s.histMu.Unlock()
	s.notify()
}

// historyView renvoie l'historique affiché (un PC, ou tous si id est vide).
func (s *Service) historyView(id string) map[string]any {
	names := map[string]string{}
	hasAgent := false
	for _, d := range s.allDevices() {
		names[d.ID] = d.Name
		if d.ID == id && d.Agent != nil && d.Agent.HasKey() {
			hasAgent = true
		}
	}

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

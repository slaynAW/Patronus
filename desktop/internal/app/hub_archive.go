package app

import (
	"context"
	"slices"

	"github.com/slaynaw/wakeonlan/agent/hubprotocol"
	"github.com/slaynaw/wakeonlan/agent/protocol"
	"github.com/slaynaw/wakeonlan/desktop/internal/agentclient"
	"github.com/slaynaw/wakeonlan/desktop/internal/diag"
	"github.com/slaynaw/wakeonlan/desktop/internal/hubstore"
	"github.com/slaynaw/wakeonlan/desktop/internal/model"
)

// Archive du hub (docs/PROTOCOLE-HUB.md, §6.3) : journal et mesures d'un PC confié, gardés par le hub
// au-delà des 30 et 90 jours de l'agent (un an par défaut). L'écran « Mesures » les lit comme une source
// de plus, après l'agent et avant les archives GitHub ; le journal de l'agent fait foi sur la période
// qu'il couvre (même fusion, évènement par évènement).

// hubJournalPages borne les pages de journal lues pour une période (2 000 évènements chacune).
const hubJournalPages = 20

// hubSource renvoie le hub appairé et l'identifiant du PC d sur ce hub, si le PC lui est confié.
func (s *Service) hubSource(d model.Device) (hubstore.Hub, string, bool) {
	h, ok := s.currentHub()
	if !ok {
		return hubstore.Hub{}, "", false
	}
	pc, ok := s.hubPCOf(d)
	if !ok {
		return hubstore.Hub{}, "", false
	}
	return h, pc.ID, true
}

// hubMetrics lit sur le hub les mesures des jours wanted (« 2006-01-02 ») qu'il garde pour le PC d, sur
// une seule session ; les jours passés restent en cache comme ceux de l'agent.
func (s *Service) hubMetrics(ctx context.Context, d model.Device, wanted []string) (map[string][]protocol.MetricsRow, error) {
	h, pc, ok := s.hubSource(d)
	if !ok || len(wanted) == 0 {
		return nil, nil
	}
	out := map[string][]protocol.MetricsRow{}
	var missing []string
	for _, day := range wanted {
		if rows, ok := s.cachedRows("hub|"+pc+"|"+day, day); ok {
			out[day] = rows
		} else {
			missing = append(missing, day)
		}
	}
	if len(missing) == 0 {
		return out, nil
	}
	err := s.agent.HubSession(ctx, hubTarget(h), func(ctx context.Context, ops agentclient.HubOps) error {
		list, err := ops.Do(ctx, hubprotocol.Request{Cmd: hubprotocol.CmdMetrics, PC: pc})
		if err != nil {
			return err
		}
		var kept []string
		if list.Metrics != nil {
			kept = list.Metrics.Days
		}
		for _, day := range missing {
			if !slices.Contains(kept, day) {
				continue
			}
			resp, err := ops.Do(ctx, hubprotocol.Request{Cmd: hubprotocol.CmdMetrics, PC: pc, Day: day})
			if err != nil {
				return err
			}
			var rows []protocol.MetricsRow
			if resp.Metrics != nil {
				rows = resp.Metrics.Rows
			}
			out[day] = rows
			s.keepRows("hub|"+pc+"|"+day, rows)
		}
		return nil
	})
	if err != nil {
		diag.Warn(areaHub, "mesures de « %s » sur le hub : %s %s (%v)", d.Name, agentclient.CodeOf(err), agentclient.ReasonOf(err), err)
	}
	return out, err
}

// hubJournal lit sur le hub le journal du PC d entre from et to (secondes), page par page.
func (s *Service) hubJournal(ctx context.Context, d model.Device, from, to int64) ([]protocol.HistoryEvent, error) {
	h, pc, ok := s.hubSource(d)
	if !ok {
		return nil, nil
	}
	var out []protocol.HistoryEvent
	err := s.agent.HubSession(ctx, hubTarget(h), func(ctx context.Context, ops agentclient.HubOps) error {
		next := from
		for range hubJournalPages {
			resp, err := ops.Do(ctx, hubprotocol.Request{Cmd: hubprotocol.CmdHistory, PC: pc, From: next})
			if err != nil {
				return err
			}
			if resp.History == nil || len(resp.History.Events) == 0 {
				return nil
			}
			for _, e := range resp.History.Events {
				if e.T >= to {
					return nil
				}
				if e.T >= from {
					out = append(out, e)
				}
			}
			if !resp.More {
				return nil
			}
			next = resp.History.Events[len(resp.History.Events)-1].T + 1
		}
		return nil
	})
	if err != nil {
		diag.Warn(areaHub, "journal de « %s » sur le hub : %s %s (%v)", d.Name, agentclient.CodeOf(err), agentclient.ReasonOf(err), err)
	}
	return out, err
}

// hubMonths renvoie les mois (« 2006-01 ») dont le hub garde des mesures du PC d.
func (s *Service) hubMonths(ctx context.Context, d model.Device) []string {
	h, pc, ok := s.hubSource(d)
	if !ok {
		return nil
	}
	var days []string
	err := s.agent.HubSession(ctx, hubTarget(h), func(ctx context.Context, ops agentclient.HubOps) error {
		resp, err := ops.Do(ctx, hubprotocol.Request{Cmd: hubprotocol.CmdMetrics, PC: pc})
		if err == nil && resp.Metrics != nil {
			days = resp.Metrics.Days
		}
		return err
	})
	if err != nil {
		diag.Warn(areaHub, "mois gardés par le hub pour « %s » : %s (%v)", d.Name, agentclient.CodeOf(err), err)
		return nil
	}
	var months []string
	for _, day := range days {
		if len(day) == len("2006-01-02") && !slices.Contains(months, day[:7]) {
			months = append(months, day[:7])
		}
	}
	return months
}

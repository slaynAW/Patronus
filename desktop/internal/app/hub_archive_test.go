package app

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol"
	"github.com/slaynaw/wakeonlan/agent/protocol2"
	"github.com/slaynaw/wakeonlan/agent/testhub"
)

// TestHubArchive : le journal et les mesures gardés par le vrai hub alimentent l'écran « Mesures »
// quand l'agent du PC ne répond plus (docs/PROTOCOLE-HUB.md, §6.3).
func TestHubArchive(t *testing.T) {
	h := testhub.Start(t)
	s := pairedWithHub(t, h, protocol2.RoleAdmin)
	agent := testhub.StartAgent(t)
	now := time.Now().UTC()
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
	boot := day.Add(9 * time.Hour).Unix()
	if err := agent.History.Add(protocol.HistoryEvent{T: boot, K: protocol.HistoryBoot}); err != nil {
		t.Fatal(err)
	}
	agent.Metrics.Set(yesterday, []protocol.MetricsRow{
		{T: boot, N: 6, CPUTemp: fp(55), CPUTempMax: fp(60)},
		{T: boot + 60, N: 6, CPUTemp: fp(57), CPUTempMax: fp(61)},
	})
	id := addAdminPC(t, s, agent, "PC Bureau")
	if r := call(t, s, "hubEntrust", map[string]any{"id": id}); r["ok"] != true {
		t.Fatalf("PC confié : %v", r)
	}
	s.refreshHub(context.Background())
	d, _ := s.device(id)

	// Le hub relit le journal et les jours passés de l'agent dès son premier relevé.
	deadline := time.Now().Add(10 * time.Second)
	for !slices.Contains(s.hubMonths(context.Background(), d), yesterday[:7]) {
		if time.Now().After(deadline) {
			t.Fatalf("mesures jamais gardées par le hub :\n%s", h.Log())
		}
		time.Sleep(100 * time.Millisecond)
	}
	months, err := s.metricsMonths(id, false)
	if err != nil || !slices.Contains(months.(map[string]any)["months"].([]string), yesterday[:7]) {
		t.Fatalf("mois proposés : %v %v", months, err)
	}

	// L'agent s'arrête : les mesures et le journal viennent du hub.
	agent.Stop()
	from, to := day.UnixMilli(), day.AddDate(0, 0, 1).UnixMilli()
	view, err := s.metricsRange(id, from, to, 100)
	if err != nil || !view.Hub || view.Agent || view.Minutes != 2 {
		t.Fatalf("mesures lues sur le hub : %+v %v", view, err)
	}
	if view.Summary.CPUTempMax == nil || *view.Summary.CPUTempMax != 61 {
		t.Errorf("résumé : %+v", view.Summary)
	}
	journal, err := s.metricsJournal(id, from, to)
	if err != nil {
		t.Fatal(err)
	}
	items := journal.(map[string]any)["events"].([]HistoryItem)
	if len(items) != 1 || items[0].Time != boot*1000 {
		t.Fatalf("journal lu sur le hub : %+v", items)
	}

	// Sans hub, rien de plus.
	if r := call(t, s, "hubForget", nil); r["ok"] != true {
		t.Fatalf("oubli : %v", r)
	}
	if view, err := s.metricsRange(id, from, to, 100); err != nil || view.Hub || view.Minutes != 0 {
		t.Errorf("sans hub : %+v %v", view, err)
	}
}

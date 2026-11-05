package app

import (
	"context"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol2"
	"github.com/slaynaw/wakeonlan/agent/testhub"
)

// TestHubSchedules : programmation d'un PC confié au vrai hub ; l'extinction donne au hub le rôle
// « control » sur le vrai agent, et la désactiver le lui retire (docs/PROTOCOLE-HUB.md, §6.7).
func TestHubSchedules(t *testing.T) {
	h := testhub.Start(t)
	s := pairedWithHub(t, h, protocol2.RoleAdmin)
	agent := testhub.StartAgent(t)
	id := addAdminPC(t, s, agent, "PC Bureau")
	if r := call(t, s, "hubEntrust", map[string]any{"id": id}); r["ok"] != true {
		t.Fatalf("PC confié : %v", r)
	}
	s.refreshHub(context.Background())
	pcs, _ := s.hubPCs()
	if len(pcs) != 1 {
		t.Fatalf("PC du hub : %+v", pcs)
	}
	pc := pcs[0].ID
	hubRole := func() protocol2.Role {
		d, ok := agent.Registry.Lookup(h.Pin)
		if !ok {
			t.Fatal("hub inconnu de l'agent")
		}
		return d.Role
	}
	weekdays := map[string]any{"on": true, "days": []int{1, 2, 3, 4, 5}, "at": "07:30"}
	night := map[string]any{"on": true, "days": []int{1, 2, 3, 4, 5, 6, 7}, "at": "23:00"}

	// Réveil seul : le hub garde le rôle wake.
	r := call(t, s, "hubSchedule", map[string]any{"pc": pc, "wake": weekdays})
	if r["ok"] != true || r["agent"] != "none" || hubRole() != protocol2.RoleWake {
		t.Fatalf("réveil programmé : %v (rôle %s)", r, hubRole())
	}
	// Extinction : rôle control donné au hub sur l'agent, puis programmation enregistrée.
	r = call(t, s, "hubSchedule", map[string]any{"pc": pc, "wake": weekdays, "shutdown": night})
	if r["ok"] != true || r["agent"] != "granted" || hubRole() != protocol2.RoleControl {
		t.Fatalf("extinction programmée : %v (rôle %s)", r, hubRole())
	}
	sched, _ := r["schedule"].(map[string]any)
	if sd, _ := sched["shutdown"].(map[string]any); sd == nil || sd["at"] != "23:00" || sd["next"].(float64) <= float64(time.Now().Unix()) {
		t.Fatalf("programmation : %v", sched)
	}
	// L'état du hub montre la programmation et le fuseau horaire ; le rôle effectif est relevé par le hub.
	info := call(t, s, "hubInfo", nil)
	listed, _ := info["pcs"].([]any)
	if info["timezone"] != "Europe/Paris" || len(listed) != 1 || listed[0].(map[string]any)["schedule"] == nil {
		t.Fatalf("état du hub : %v", info)
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		s.refreshHub(context.Background())
		if p, _ := s.hubPCs(); len(p) == 1 && p[0].Role == protocol2.RoleControl.String() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("rôle control jamais relevé par le hub")
		}
		time.Sleep(100 * time.Millisecond)
	}
	// Extinction désactivée : le rôle control est retiré au hub.
	off := map[string]any{"on": false, "days": []int{1, 2, 3, 4, 5, 6, 7}, "at": "23:00"}
	r = call(t, s, "hubSchedule", map[string]any{"pc": pc, "wake": weekdays, "shutdown": off})
	if r["ok"] != true || r["agent"] != "withdrawn" || hubRole() != protocol2.RoleWake {
		t.Fatalf("extinction désactivée : %v (rôle %s)", r, hubRole())
	}
	// Refus : jours invalides, PC inconnu ; aucune programmation : tout est retiré.
	for _, bad := range []map[string]any{
		{"pc": pc, "wake": map[string]any{"on": true, "days": []int{0}, "at": "07:30"}},
		{"pc": pc, "wake": map[string]any{"on": true, "days": []int{1}, "at": "7h30"}},
		{"pc": "inconnu", "wake": weekdays},
	} {
		if r := call(t, s, "hubSchedule", bad); r["ok"] != false || r["reason"] != "bad_request" {
			t.Errorf("%v : %v", bad, r)
		}
	}
	if r := call(t, s, "hubSchedule", map[string]any{"pc": pc}); r["ok"] != true {
		t.Fatalf("programmation retirée : %v", r)
	}
	if info := call(t, s, "hubInfo", nil); info["pcs"].([]any)[0].(map[string]any)["schedule"] != nil {
		t.Fatalf("après le retrait : %v", info)
	}
	// Fuseau horaire du hub.
	if r := call(t, s, "hubTimezone", map[string]any{"timezone": "America/Montreal"}); r["ok"] != true || r["timezone"] != "America/Montreal" {
		t.Fatalf("fuseau horaire : %v", r)
	}
	if r := call(t, s, "hubTimezone", map[string]any{"timezone": "Lune/Tranquillité"}); r["ok"] != false || r["reason"] != "bad_request" {
		t.Errorf("fuseau inconnu : %v", r)
	}
}

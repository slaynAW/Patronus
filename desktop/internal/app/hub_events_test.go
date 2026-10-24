package app

import (
	"context"
	"testing"

	"github.com/slaynaw/wakeonlan/agent/hubprotocol"
	"github.com/slaynaw/wakeonlan/agent/protocol2"
	"github.com/slaynaw/wakeonlan/agent/testhub"
)

// TestHubAbsence : un réveil envoyé par le vrai hub apparaît dans « Pendant votre absence », avec le
// nom du PC sur cet ordinateur ; une fois vu, il ne revient plus.
func TestHubAbsence(t *testing.T) {
	h := testhub.Start(t)
	s := pairedWithHub(t, h, protocol2.RoleAdmin)
	agent := testhub.StartAgent(t)
	id := addAdminPC(t, s, agent, "PC Bureau")
	if r := call(t, s, "hubEntrust", map[string]any{"id": id}); r["ok"] != true {
		t.Fatalf("PC confié : %v", r)
	}
	s.refreshHub(context.Background())
	if v := s.State().Hub; v == nil || v.Unseen != 0 {
		t.Fatalf("aucun évènement attendu : %+v", v)
	}
	if w := call(t, s, "wake", map[string]any{"id": id}); w["viaHub"] != true {
		t.Fatalf("réveil relayé : %v", w)
	}
	s.refreshHub(context.Background())
	if v := s.State().Hub; v.Unseen != 1 {
		t.Fatalf("évènements non vus : %+v", v)
	}
	r := call(t, s, "hubEvents", nil)
	events, _ := r["events"].([]any)
	if r["ok"] != true || len(events) != 1 {
		t.Fatalf("évènements : %v", r)
	}
	e := events[0].(map[string]any)
	if e["k"] != "wake" || e["self"] != true || e["i"] != s.identity.Existing().Pin.Short() || e["local"] != id ||
		e["name"] != "PC Bureau" || e["hubName"] != nil {
		t.Fatalf("réveil vu par le hub : %v", e)
	}
	if r := call(t, s, "hubEventsSeen", map[string]any{"until": e["t"]}); r["ok"] != true {
		t.Fatalf("vus : %v", r)
	}
	if v := s.State().Hub; v.Unseen != 0 {
		t.Fatalf("après lecture : %+v", v)
	}
	s.refreshHub(context.Background())
	if v := s.State().Hub; v.Unseen != 0 {
		t.Fatalf("évènement revenu après lecture : %+v", v)
	}
	// L'état vu par le hub accompagne le PC confié.
	if hv := hubOf(s.State(), id); hv == nil || hv.State != hubprotocol.StateOnline || hv.Checked == 0 {
		t.Errorf("état vu par le hub : %+v", hv)
	}
}

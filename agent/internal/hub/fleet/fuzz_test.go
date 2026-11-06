package fleet

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/hubprotocol"
	"github.com/slaynaw/wakeonlan/agent/protocol2"
)

// Fuzzing : go test ./internal/hub/fleet -run '^$' -fuzz FuzzOpen. Sans -fuzz, seuls les exemples de
// départ sont rejoués.

// FuzzOpen : une liste de PC quelconque ne fait jamais paniquer ; une liste acceptée n'a que des PC
// valides, aux identifiants distincts, et se réécrit puis se relit à l'identique.
func FuzzOpen(f *testing.F) {
	now := func() time.Time { return time.Date(2026, 11, 6, 3, 0, 0, 0, time.UTC) }
	st := &memStorage{}
	list, err := Open(st, now)
	if err != nil {
		f.Fatal(err)
	}
	agent := protocol2.PinOf([]byte("agent"))
	if _, err := list.Add(PC{Name: "PC Bureau", Host: "192.168.1.20", Port: 9770, MAC: "30:9C:23:01:02:03", Agent: &agent, Role: protocol2.RoleWake}); err != nil {
		f.Fatal(err)
	}
	if _, err := list.Add(PC{Name: "Box", Host: "box.lan", Probe: &hubprotocol.Probe{Kind: hubprotocol.ProbeHTTPS}}); err != nil {
		f.Fatal(err)
	}
	for _, seed := range [][]byte{st.data, []byte(`{"pcs":[]}`), []byte(`{"pcs":[{"id":"a","name":"x","host":"1.2.3.4"}]}`)} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		list, err := Open(&memStorage{data: raw}, now)
		if err != nil {
			return
		}
		ids := map[string]bool{}
		for _, p := range list.List() {
			if err := validate(p); err != nil || p.ID == "" || ids[p.ID] {
				t.Fatalf("PC accepté : %+v (%v)", p, err)
			}
			ids[p.ID] = true
			if p.Agent != nil && p.Role == protocol2.RoleAdmin {
				t.Fatalf("hub administrateur d'un PC : %+v", p)
			}
		}
		out := &memStorage{}
		saved, _ := json.Marshal(state{PCs: list.List()})
		out.data = saved
		again, err := Open(out, now)
		if err != nil || len(again.List()) != len(list.List()) {
			t.Fatalf("relecture : %v", err)
		}
	})
}

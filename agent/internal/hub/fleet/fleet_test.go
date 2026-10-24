package fleet

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/hubprotocol"
	"github.com/slaynaw/wakeonlan/agent/protocol2"
)

type memStorage struct{ data []byte }

func (m *memStorage) Load() ([]byte, error) {
	if m.data == nil {
		return nil, os.ErrNotExist
	}
	return m.data, nil
}

func (m *memStorage) Save(data []byte) error { m.data = append([]byte(nil), data...); return nil }

func TestFleet(t *testing.T) {
	st := &memStorage{}
	f, err := Open(st, time.Now)
	if err != nil || len(f.List()) != 0 {
		t.Fatalf("liste vide : %v", err)
	}
	pin := protocol2.PinOf([]byte("agent"))
	pc, err := f.Add(PC{Name: "PC Bureau", Host: "192.168.1.10", Port: 9770, Agent: &pin, Role: protocol2.RoleWake})
	if err != nil || len(pc.ID) != 22 || pc.Added == 0 {
		t.Fatalf("ajout : %+v, %v", pc, err)
	}
	if _, err := f.Add(PC{Name: "Doublon", Host: "192.168.1.11", Port: 9770, Agent: &pin, Role: protocol2.RoleWake}); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("doublon : %v", err)
	}
	for name, bad := range map[string]PC{
		"rôle admin":         {Name: "PC", Host: "192.168.1.12", Port: 9770, Agent: &pin, Role: protocol2.RoleAdmin},
		"agent sans port":    {Name: "PC", Host: "192.168.1.12", Agent: &pin, Role: protocol2.RoleWake},
		"sonde et agent":     {Name: "PC", Host: "192.168.1.12", Port: 9770, Agent: &pin, Role: protocol2.RoleWake, Probe: &hubprotocol.Probe{Kind: "tcp", Port: 1}},
		"sans sonde":         {Name: "Box", Host: "192.168.1.1"},
		"rôle sans agent":    {Name: "Box", Host: "192.168.1.1", Role: protocol2.RoleView, Probe: &hubprotocol.Probe{Kind: "http"}},
		"nom non nettoyé":    {Name: " Box", Host: "192.168.1.1", Probe: &hubprotocol.Probe{Kind: "http"}},
		"MAC non normalisée": {Name: "Box", Host: "192.168.1.1", MAC: "aa:bb:cc:dd:ee:ff", Probe: &hubprotocol.Probe{Kind: "http"}},
	} {
		if _, err := f.Add(bad); err == nil {
			t.Errorf("%s : accepté", name)
		}
	}
	// Update ne touche ni à l'identifiant, ni à l'agent, ni à la date d'ajout.
	other := protocol2.PinOf([]byte("autre"))
	up, err := f.Update(pc.ID, func(p *PC) { p.Name, p.ID, p.Agent, p.Added = "Bureau", "x", &other, 1 })
	if err != nil || up.Name != "Bureau" || up.ID != pc.ID || *up.Agent != pin || up.Added != pc.Added {
		t.Fatalf("modification : %+v, %v", up, err)
	}
	again, err := Open(st, time.Now)
	if err != nil || len(again.List()) != 1 || again.List()[0].Name != "Bureau" {
		t.Fatalf("relecture : %+v, %v", again.List(), err)
	}
	// Une liste illisible ou incohérente n'est jamais remplacée par une liste vide.
	for _, raw := range []string{"{", `{"pcs":[{"id":"","name":"PC","host":"192.168.1.1","probe":{"kind":"tcp","port":1}}]}`} {
		if _, err := Open(&memStorage{data: []byte(raw)}, time.Now); err == nil {
			t.Errorf("%s : accepté", raw)
		}
	}
	if _, err := f.Remove(pc.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Remove(pc.ID); !errors.Is(err, ErrUnknownPC) {
		t.Fatalf("second retrait : %v", err)
	}
}

package app

import (
	"net/netip"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol2"
	"github.com/slaynaw/wakeonlan/agent/testhub"
)

// pairedWithHub renvoie un service appairé au hub h avec le rôle role.
func pairedWithHub(t *testing.T, h *testhub.Hub, role protocol2.Role) *Service {
	t.Helper()
	s, _ := newService(t)
	if r := call(t, s, "hubPair", map[string]any{"text": h.Link(t, role)}); r["ok"] != true {
		t.Fatalf("appairage au hub : %v", r)
	}
	return s
}

// addAdminPC enregistre dans s le vrai agent a, cet ordinateur en étant administrateur ; renvoie
// l'identifiant du PC.
func addAdminPC(t *testing.T, s *Service, a *testhub.Agent, name string) string {
	t.Helper()
	code := a.Invite(t, protocol2.RoleAdmin, s.identity.Existing().Pin)
	r := call(t, s, "saveDevice", map[string]any{"form": pairingForm(name, "aa:bb:cc:dd:ee:01", a.Port),
		"pairing": map[string]any{"agent": a.ID.Pin.String(), "code": code}})
	id, _ := r["id"].(string)
	if r["ok"] != true || id == "" {
		t.Fatalf("PC %s : %v", name, r)
	}
	return id
}

// waitHub attend que l'état de l'interface remplisse cond (relevé du hub en arrière-plan).
func waitHub(t *testing.T, s *Service, what string, cond func(UIState) bool) UIState {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		st := s.State()
		if cond(st) {
			return st
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s : %+v", what, st.Devices)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func hubOf(st UIState, id string) *DeviceHubView {
	for _, d := range st.Devices {
		if d.ID == id {
			return d.Hub
		}
	}
	return nil
}

// TestHubEntrustAndRemove : un PC confié au vrai hub par cet ordinateur, administrateur du PC et du
// hub (docs/HUB.md, §5.3), un appareil sans agent, le réveil relayé par le hub, puis les retraits.
func TestHubEntrustAndRemove(t *testing.T) {
	h := testhub.Start(t)
	s := pairedWithHub(t, h, protocol2.RoleAdmin)
	agent := testhub.StartAgent(t)
	id := addAdminPC(t, s, agent, "PC Bureau")

	r := call(t, s, "hubEntrust", map[string]any{"id": id})
	if r["ok"] != true || r["role"] != "wake" {
		t.Fatalf("PC confié : %v", r)
	}
	// Le hub est enregistré par l'agent au rôle wake, sous son nom.
	var hubOnAgent bool
	for _, dev := range agent.Registry.Devices() {
		if dev.Pin == h.Pin {
			hubOnAgent = dev.Role == protocol2.RoleWake && dev.Name == testhub.Name
		}
	}
	if !hubOnAgent || len(agent.Registry.Invites()) != 0 {
		t.Fatalf("hub sur l'agent : %+v, invitations %+v", agent.Registry.Devices(), agent.Registry.Invites())
	}
	st := waitHub(t, s, "PC confié dans l'état", func(st UIState) bool { return hubOf(st, id) != nil })
	pc := hubOf(st, id).PC
	if r := call(t, s, "hubEntrust", map[string]any{"id": id}); r["code"] != "ALREADY_ENTRUSTED" {
		t.Errorf("PC confié deux fois : %v", r)
	}

	// Appareil sans agent : champs vérifiés avant d'aller au hub.
	for _, bad := range []map[string]any{
		{"name": "", "host": "192.168.1.1", "probe": "http"},
		{"name": "Box", "host": "pas une adresse", "probe": "http"},
		{"name": "Box", "host": "192.168.1.1", "probe": "ping"},
		{"name": "Box", "host": "192.168.1.1", "probe": "tcp"},
		{"name": "Box", "host": "192.168.1.1", "probe": "https", "mac": "zz"},
	} {
		if r := call(t, s, "hubAddDevice", bad); r["code"] != "INVALID" {
			t.Errorf("appareil %v : %v", bad, r)
		}
	}
	if r := call(t, s, "hubAddDevice", map[string]any{"name": "Box", "host": "127.0.0.1", "probe": "tcp", "port": "1"}); r["ok"] != true {
		t.Fatalf("appareil sans agent : %v", r)
	}
	info := call(t, s, "hubInfo", nil)
	pcs, _ := info["pcs"].([]any)
	if len(pcs) != 2 {
		t.Fatalf("PC du hub : %v", info)
	}
	var boxID string
	for _, p := range pcs {
		p := p.(map[string]any)
		switch p["name"] {
		case "PC Bureau":
			if p["local"] != id || p["agent"] != agent.ID.Pin.Short() || p["role"] != "wake" || p["mac"] != "AA:BB:CC:DD:EE:01" {
				t.Errorf("PC confié vu du hub : %v", p)
			}
		case "Box":
			boxID, _ = p["id"].(string)
			if p["probe"] != "tcp" || p["port"] != float64(1) || p["local"] != nil {
				t.Errorf("appareil vu du hub : %v", p)
			}
		}
	}

	// Cet ordinateur n'a aucun réseau (tests) : le PC n'est pas sur son réseau, le hub relaie le réveil.
	if w := call(t, s, "wake", map[string]any{"id": id}); w["ok"] != true || w["viaHub"] != true {
		t.Fatalf("réveil relayé : %v", w)
	}

	if r := call(t, s, "hubRemovePC", map[string]any{"pc": boxID}); r["ok"] != true || r["agent"] != "none" {
		t.Fatalf("retrait de l'appareil : %v", r)
	}
	if r := call(t, s, "hubRemovePC", map[string]any{"pc": pc}); r["ok"] != true || r["agent"] != "revoked" || r["hub"] != h.Pin.Short() {
		t.Fatalf("retrait du PC : %v", r)
	}
	for _, dev := range agent.Registry.Devices() {
		if dev.Pin == h.Pin {
			t.Fatal("hub encore autorisé par l'agent")
		}
	}
	waitHub(t, s, "PC retiré de l'état", func(st UIState) bool { return hubOf(st, id) == nil })
	if w := call(t, s, "wake", map[string]any{"id": id}); w["viaHub"] != false {
		t.Errorf("réveil d'un PC retiré : %v", w)
	}
	if r := call(t, s, "hubRemovePC", map[string]any{"pc": pc}); r["ok"] != false || r["reason"] != "bad_request" {
		t.Errorf("PC déjà retiré : %v", r)
	}
}

// TestHubEntrustRefused : seul un administrateur du hub confie un PC ; l'invitation de l'agent, créée
// d'abord, est alors annulée.
func TestHubEntrustRefused(t *testing.T) {
	h := testhub.Start(t)
	s := pairedWithHub(t, h, protocol2.RoleView)
	agent := testhub.StartAgent(t)
	id := addAdminPC(t, s, agent, "PC Bureau")
	r := call(t, s, "hubEntrust", map[string]any{"id": id})
	if r["ok"] != false || r["step"] != "hub" || r["reason"] != "insufficient_role" {
		t.Fatalf("PC confié par un appareil view : %v", r)
	}
	if invites := agent.Registry.Invites(); len(invites) != 0 {
		t.Errorf("invitation restée sur l'agent : %+v", invites)
	}
	for _, dev := range agent.Registry.Devices() {
		if dev.Pin == h.Pin {
			t.Error("hub enregistré par l'agent")
		}
	}
	// Un PC sans empreinte wolagent/2 ne peut pas être confié.
	if r := call(t, s, "hubEntrust", map[string]any{"id": "inconnu"}); r["code"] != "NO_KEY" {
		t.Errorf("PC inconnu : %v", r)
	}
}

func TestOnLocalNetwork(t *testing.T) {
	local := []netip.Prefix{netip.MustParsePrefix("192.168.1.20/24"), netip.MustParsePrefix("fd00::5/64")}
	for host, want := range map[string]bool{
		"192.168.1.30": true, "192.168.2.30": false, "fd00::9": true, "::ffff:192.168.1.7": true, "pc-bureau": false,
	} {
		if got := onLocalNetwork(host, local); got != want {
			t.Errorf("%s : %v", host, got)
		}
	}
}

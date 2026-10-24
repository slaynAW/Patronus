package app

import (
	"strings"
	"testing"

	"github.com/slaynaw/wakeonlan/agent/hubprotocol"
	"github.com/slaynaw/wakeonlan/agent/protocol2"
	"github.com/slaynaw/wakeonlan/agent/testhub"
)

// TestHubPairingAndAdmin : appairage au vrai hub par son lien, état, appareils autorisés, invitations,
// rôles, durée de conservation, puis oubli du hub (docs/HUB.md, §5.2 et §12).
func TestHubPairingAndAdmin(t *testing.T) {
	h := testhub.Start(t)
	s, _ := newService(t)
	if r := call(t, s, "hubInfo", nil); r["ok"] != false || r["code"] != "NO_HUB" {
		t.Fatalf("sans hub : %v", r)
	}
	for text, reason := range map[string]string{
		"n'importe quoi": "invalid",
		protocol2.Link{Host: "192.168.1.10", Agent: protocol2.PinOf([]byte("agent")), Code: "ABCDEFGH"}.String(): "agent_link",
		protocol2.Link{Host: "127.0.0.1", Agent: h.Pin, Code: "ABCDEFGH", Expires: 1}.Format(hubprotocol.LinkFormat): "expired",
	} {
		if r := call(t, s, "hubPair", map[string]any{"text": text}); r["ok"] != false || r["code"] != "LINK" || r["reason"] != reason {
			t.Errorf("lien %q : %v", text, r)
		}
	}

	r := call(t, s, "hubPair", map[string]any{"text": "  " + h.Link(t, protocol2.RoleAdmin) + "\n"})
	if r["ok"] != true || r["role"] != "admin" || r["name"] != testhub.Name || r["already"] != false {
		t.Fatalf("appairage : %v", r)
	}
	view := s.State().Hub
	if view == nil || view.Name != testhub.Name || view.Short != h.Pin.Short() || view.Role != "admin" || view.Port != h.Port {
		t.Fatalf("état de l'interface : %+v", view)
	}

	info := call(t, s, "hubInfo", nil)
	devices, _ := info["devices"].([]any)
	if info["ok"] != true || info["role"] != "admin" || info["version"] != testhub.Version || len(devices) != 1 ||
		info["retention"] != float64(365) || info["pcs"] != float64(0) {
		t.Fatalf("état du hub : %v", info)
	}
	if self, _ := devices[0].(map[string]any); self["self"] != true || self["short"] != s.identity.Existing().Pin.Short() {
		t.Errorf("cet ordinateur : %v", self)
	}

	// Invitation d'un second appareil, au rôle view ; une invitation annulée disparaît.
	cancelled := call(t, s, "hubInvite", map[string]any{"role": "wake", "ttl": 600, "name": "Brouillon"})
	inv := call(t, s, "hubInvite", map[string]any{"role": "view", "ttl": 3600, "name": "Tablette"})
	if inv["ok"] != true || cancelled["ok"] != true || !strings.HasPrefix(inv["qr"].(string), "<svg") {
		t.Fatalf("invitation : %v", inv)
	}
	link, err := hubprotocol.ParseLink(inv["link"].(string))
	if err != nil || link.Agent != h.Pin || link.Role != protocol2.RoleView || link.Host != "127.0.0.1" || link.Port != h.Port {
		t.Fatalf("lien de l'invitation : %+v %v", link, err)
	}
	for _, role := range []string{"control", "patron"} {
		if r := call(t, s, "hubInvite", map[string]any{"role": role, "ttl": 600}); r["ok"] != false || r["reason"] != "bad_request" {
			t.Errorf("invitation %s : %v", role, r)
		}
	}
	pending, _ := call(t, s, "hubInfo", nil)["invites"].([]any)
	if len(pending) != 2 {
		t.Fatalf("invitations : %v", pending)
	}
	for _, p := range pending {
		if p := p.(map[string]any); p["name"] == "Brouillon" {
			if r := call(t, s, "hubAdminChange", map[string]any{"action": "cancel", "invite": p["id"]}); r["ok"] != true {
				t.Fatalf("annulation : %v", r)
			}
		}
	}
	if pending, _ := call(t, s, "hubInfo", nil)["invites"].([]any); len(pending) != 1 {
		t.Fatalf("après annulation : %v", pending)
	}

	tablet, _ := newService(t)
	if r := call(t, tablet, "hubPair", map[string]any{"text": inv["link"]}); r["ok"] != true || r["role"] != "view" {
		t.Fatalf("appairage de la tablette : %v", r)
	}
	if r := call(t, tablet, "hubInfo", nil); r["ok"] != true || r["role"] != "view" || r["devices"] != nil {
		t.Fatalf("hub vu par la tablette : %v", r)
	}
	if r := call(t, tablet, "hubInvite", map[string]any{"role": "view", "ttl": 600}); r["ok"] != false || r["reason"] != "insufficient_role" {
		t.Fatalf("invitation par la tablette : %v", r)
	}

	// Rôle et nom changés par l'administrateur : la tablette relit son rôle.
	tabletPin := tablet.identity.Existing().Pin.String()
	for _, change := range []map[string]any{
		{"action": "role", "device": tabletPin, "role": "wake"},
		{"action": "rename", "device": tabletPin, "name": "Tablette du salon"},
	} {
		if r := call(t, s, "hubAdminChange", change); r["ok"] != true {
			t.Fatalf("%v : %v", change, r)
		}
	}
	if r := call(t, s, "hubAdminChange", map[string]any{"action": "role", "device": tabletPin, "role": "control"}); r["reason"] != "bad_request" {
		t.Errorf("rôle control : %v", r)
	}
	if r := call(t, tablet, "hubInfo", nil); r["role"] != "wake" || tablet.State().Hub.Role != "wake" {
		t.Fatalf("rôle relu : %v %+v", r, tablet.State().Hub)
	}

	// Conservation de l'archive.
	if r := call(t, s, "hubRetention", map[string]any{"days": 730}); r["ok"] != true || r["retention"] != float64(730) {
		t.Fatalf("conservation : %v", r)
	}
	if r := call(t, s, "hubRetention", map[string]any{"days": 30}); r["reason"] != "bad_request" {
		t.Errorf("conservation trop courte : %v", r)
	}
	if r := call(t, s, "hubInfo", nil); r["retention"] != float64(730) {
		t.Errorf("conservation relue : %v", r)
	}

	// Le dernier administrateur ne peut pas se retirer ; un autre hub n'est pas accepté en même temps.
	selfPin := s.identity.Existing().Pin.String()
	if r := call(t, s, "hubAdminChange", map[string]any{"action": "revoke", "device": selfPin}); r["reason"] != "last_admin" {
		t.Errorf("retrait du dernier administrateur : %v", r)
	}
	other := testhub.Start(t)
	if r := call(t, s, "hubPair", map[string]any{"text": other.Link(t, protocol2.RoleAdmin)}); r["code"] != "HUB_PAIRED" {
		t.Errorf("second hub : %v", r)
	}

	// La tablette oublie le hub ; n'étant pas administrateur, elle ne peut pas s'en retirer :
	// l'administrateur la retire.
	if r := call(t, tablet, "hubForget", nil); r["ok"] != true || r["revoked"] != false || tablet.State().Hub != nil {
		t.Fatalf("oubli par la tablette : %v", r)
	}
	if r := call(t, s, "hubAdminChange", map[string]any{"action": "revoke", "device": tabletPin}); r["ok"] != true {
		t.Fatalf("retrait de la tablette : %v", r)
	}
	if devs := h.Devices(); len(devs) != 1 || devs[0].Pin.String() != selfPin {
		t.Fatalf("appareils du hub : %+v", devs)
	}
	// Un second administrateur peut, lui, se retirer en oubliant le hub.
	second, _ := newService(t)
	if r := call(t, second, "hubPair", map[string]any{"text": h.Link(t, protocol2.RoleAdmin)}); r["ok"] != true {
		t.Fatalf("second administrateur : %v", r)
	}
	if r := call(t, second, "hubForget", nil); r["ok"] != true || r["revoked"] != true {
		t.Fatalf("oubli par le second administrateur : %v", r)
	}
	if devs := h.Devices(); len(devs) != 1 {
		t.Fatalf("appareils du hub après son départ : %+v", devs)
	}
	// Le dernier administrateur oublie le hub sans pouvoir s'en retirer ; un nouveau lien le
	// retrouve tel quel, sans user le code.
	if r := call(t, s, "hubForget", nil); r["ok"] != true || r["revoked"] != false || s.State().Hub != nil {
		t.Fatalf("oubli par l'administrateur : %v", r)
	}
	if r := call(t, s, "hubPair", map[string]any{"text": h.Link(t, protocol2.RoleView)}); r["ok"] != true || r["already"] != true || r["role"] != "admin" {
		t.Fatalf("appairage de nouveau : %v", r)
	}
}

// TestHubOtherService : un lien de hub qui mène au port d'un agent est refusé avant tout envoi du code.
func TestHubOtherService(t *testing.T) {
	agent := testhub.StartAgent(t)
	s, _ := newService(t)
	link := protocol2.Link{Host: "127.0.0.1", Port: agent.Port, Agent: agent.ID.Pin, Code: "ABCDEFGH"}.Format(hubprotocol.LinkFormat)
	if r := call(t, s, "hubPair", map[string]any{"text": link}); r["ok"] != false || r["code"] != "OTHER_SERVICE" {
		t.Fatalf("agent pris pour un hub : %v", r)
	}
	if s.State().Hub != nil {
		t.Fatal("hub enregistré malgré l'échec")
	}
}

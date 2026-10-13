package app

import (
	"strings"
	"testing"

	"github.com/slaynaw/wakeonlan/agent/protocol"
	"github.com/slaynaw/wakeonlan/agent/protocol2"
	"github.com/slaynaw/wakeonlan/desktop/internal/pairing"
)

// TestAgentAdmin : appareils autorisés et invitations gérés depuis cet ordinateur (rôle admin) ;
// l'invitation donne un lien v=2 complet (adresse, MAC, empreinte, code) et son QR code.
func TestAgentAdmin(t *testing.T) {
	s, _ := newService(t)
	agent, _, pin := agentV2(t)
	r := call(t, s, "saveDevice", map[string]any{"form": pairingForm("PC Bureau", "aa:bb:cc:dd:ee:01", agent.Port()),
		"pairing": map[string]any{"agent": pin.String(), "code": agent.Invite("admin")}})
	id, _ := r["id"].(string)
	if r["ok"] != true {
		t.Fatalf("appairage : %v", r)
	}
	phone := protocol2.PinOf([]byte("téléphone"))
	agent.Register(phone, "control")

	a := call(t, s, "agentAdmin", map[string]any{"id": id})
	devices, _ := a["devices"].([]any)
	if a["ok"] != true || len(devices) != 2 {
		t.Fatalf("état : %v", a)
	}
	first, _ := devices[0].(map[string]any)
	if first["self"] != true || first["role"] != "admin" || first["short"] != s.identity.Existing().Pin.Short() {
		t.Errorf("cet ordinateur : %v", first)
	}

	inv := call(t, s, "agentInvite", map[string]any{"id": id, "role": "wake", "ttl": 3600, "name": "Tablette"})
	if inv["ok"] != true || inv["role"] != "wake" || !strings.HasPrefix(inv["qr"].(string), "<svg") {
		t.Fatalf("invitation : %v", inv)
	}
	info, err := pairing.Parse(inv["link"].(string))
	if err != nil || info.Version != 2 || info.Agent != pin.String() || info.Host != "127.0.0.1" || info.MAC != "AA:BB:CC:DD:EE:01" ||
		info.Role != "wake" || protocol2.FormatCode(info.Code) != inv["code"] || info.Name != "PC Bureau" {
		t.Fatalf("lien de l'invitation : %+v %v", info, err)
	}
	if bad := call(t, s, "agentInvite", map[string]any{"id": id, "role": "owner", "ttl": 3600}); bad["ok"] != false {
		t.Errorf("rôle inconnu accepté : %v", bad)
	}
	listed := call(t, s, "agentAdmin", map[string]any{"id": id})["invites"].([]any)
	if len(listed) != 1 || listed[0].(map[string]any)["role"] != "wake" {
		t.Fatalf("invitations : %v", listed)
	}
	inviteID := listed[0].(map[string]any)["id"]
	if c := call(t, s, "agentAdminChange", map[string]any{"id": id, "action": "cancel", "invite": inviteID}); c["ok"] != true || agent.Invites() != 0 {
		t.Errorf("annulation : %v", c)
	}

	for _, c := range []map[string]any{
		{"id": id, "action": "role", "device": phone.String(), "role": "view"},
		{"id": id, "action": "rename", "device": phone.String(), "name": " Pixel de Léa "},
	} {
		if r := call(t, s, "agentAdminChange", c); r["ok"] != true {
			t.Fatalf("%v : %v", c, r)
		}
	}
	if role, name := agent.Device(phone); role != "view" || name != "Pixel de Léa" {
		t.Errorf("appareil modifié : %q %q", role, name)
	}
	if r := call(t, s, "agentAdminChange", map[string]any{"id": id, "action": "revoke", "device": phone.String()}); r["ok"] != true {
		t.Fatalf("retrait : %v", r)
	}
	// Dernier administrateur : refusé, la cause est donnée à l'interface.
	me := s.identity.Existing().Pin.String()
	if r := call(t, s, "agentAdminChange", map[string]any{"id": id, "action": "revoke", "device": me}); r["ok"] != false ||
		r["reason"] != protocol2.CodeLastAdmin {
		t.Errorf("dernier administrateur : %v", r)
	}
	if r := call(t, s, "agentAdminChange", map[string]any{"id": id, "action": "explode", "device": me}); r["ok"] != false {
		t.Errorf("action inconnue : %v", r)
	}
	// PC encore en wolagent/1 : rien à administrer.
	key, _ := protocol.NewKey()
	v1 := addAgentDevice(t, s, agent.Port(), key)
	if r := call(t, s, "agentAdmin", map[string]any{"id": v1}); r["ok"] != false || r["code"] != "NO_KEY" {
		t.Errorf("PC en wolagent/1 : %v", r)
	}
}

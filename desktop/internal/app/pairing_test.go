package app

import (
	"strconv"
	"testing"

	"github.com/slaynaw/wakeonlan/agent/protocol2"
	"github.com/slaynaw/wakeonlan/desktop/internal/model"
	"github.com/slaynaw/wakeonlan/desktop/internal/share"
)

// pairingForm : fiche d'un PC remplie depuis un lien v=2 (pas de clé).
func pairingForm(name, mac string, port int) model.DeviceForm {
	form := model.NewForm()
	form.Name, form.MAC, form.Host = name, mac, "127.0.0.1"
	form.AgentEnabled, form.AgentPort = true, strconv.Itoa(port)
	return form
}

// TestSaveDeviceWithPairing : lien v=2 collé, cet ordinateur est appairé avec le code à
// l'enregistrement ; la fiche n'est enregistrée qu'en cas de succès, avec l'empreinte et sans clé.
func TestSaveDeviceWithPairing(t *testing.T) {
	s, _ := newService(t)
	agent, _, pin := agentV2(t)
	code := agent.Invite("admin")
	r := call(t, s, "saveDevice", map[string]any{"form": pairingForm("PC Bureau", "aa:bb:cc:dd:ee:01", agent.Port()),
		"pairing": map[string]any{"agent": pin.String(), "code": protocol2.FormatCode(code)}})
	id, _ := r["id"].(string)
	if r["ok"] != true || id == "" || r["role"] != "admin" {
		t.Fatalf("appairage : %v", r)
	}
	if a := agentOf(t, s, id); a.Pin != pin.String() || a.HasKey() {
		t.Fatalf("réglages enregistrés : %v", a)
	}
	me := s.identity.Existing()
	if role, name := agent.Device(me.Pin); role != "admin" || name != s.agent.By {
		t.Fatalf("enregistrement sur l'agent : %q %q", role, name)
	}
	if v := s.State().Devices[0]; v.AgentPin != pin.Short() || !v.HasAgent || v.ShareControl {
		t.Errorf("vue : %+v", v)
	}

	// Échecs : rien n'est enregistré, la cause est renvoyée à l'interface.
	other := protocol2.PinOf([]byte("autre agent"))
	fresh := agent.Invite("control")
	for _, c := range []struct {
		agent, code, want string
	}{
		{pin.String(), code, "PAIRING_FAILED"},  // code déjà utilisé
		{other.String(), fresh, "PIN_MISMATCH"}, // autre agent à cette adresse : le code n'est pas envoyé
		{"pas une empreinte", fresh, "PAIRING_FAILED"},
	} {
		// Une autre installation (identité inconnue de l'agent) : cet ordinateur-ci est déjà enregistré.
		s2, _ := newService(t)
		r := call(t, s2, "saveDevice", map[string]any{"form": pairingForm("PC Salon", "aa:bb:cc:dd:ee:02", agent.Port()),
			"pairing": map[string]any{"agent": c.agent, "code": c.code}})
		if r["ok"] != false || r["pairing"] != c.want || len(s2.State().Devices) != 0 {
			t.Errorf("%s : %v", c.want, r)
		}
	}
	// Le code resté valable sert encore.
	s3, _ := newService(t)
	if r := call(t, s3, "saveDevice", map[string]any{"form": pairingForm("PC Salon", "aa:bb:cc:dd:ee:02", agent.Port()),
		"pairing": map[string]any{"agent": pin.String(), "code": fresh}}); r["ok"] != true || r["role"] != "control" {
		t.Errorf("code resté valable : %v", r)
	}
}

// TestRepairKeepsKeySharedWithControl : réappairer une fiche existante remplace sa clé par
// l'empreinte, sauf si le PC est partagé avec le droit « démarrer et éteindre » (la clé y circule).
func TestRepairKeepsKeySharedWithControl(t *testing.T) {
	_, srv := newGistServer(t)
	agent, key, pin := agentV2(t)
	d := testDevice(t, "dev-1", "PC streaming", "127.0.0.1", key)
	d.Agent.Port = agent.Port()
	s := newShareService(t, srv, d)
	s.sharing.mu.Lock()
	s.sharing.state.Owner = &share.Owner{Name: "Hugo", People: []share.Person{
		{Name: "Léa", Device: "x", Rights: map[string]share.Right{"dev-1": share.RightFull}},
	}}
	s.sharing.mu.Unlock()

	form := model.FormFrom(d)
	form.AgentKey = "" // le lien v=2 ne contient pas de clé
	r := call(t, s, "saveDevice", map[string]any{"id": "dev-1", "form": form,
		"pairing": map[string]any{"agent": pin.String(), "code": agent.Invite("admin")}})
	if r["ok"] != true || r["id"] != "dev-1" {
		t.Fatalf("réappairage : %v", r)
	}
	if a := agentOf(t, s, "dev-1"); a.Pin != pin.String() || a.Key != key {
		t.Fatalf("clé non gardée pour le partage : %v", a)
	}

	// Sans partage « démarrer et éteindre » : la clé est effacée.
	s.sharing.mu.Lock()
	s.sharing.state.Owner.People = nil
	s.sharing.mu.Unlock()
	agent.Register(s.identity.Existing().Pin, "")
	r = call(t, s, "saveDevice", map[string]any{"id": "dev-1", "form": form,
		"pairing": map[string]any{"agent": pin.String(), "code": agent.Invite("control")}})
	if r["ok"] != true || r["role"] != "control" {
		t.Fatalf("réappairage : %v", r)
	}
	if a := agentOf(t, s, "dev-1"); a.Pin != pin.String() || a.HasKey() {
		t.Fatalf("clé non effacée : %v", a)
	}
}

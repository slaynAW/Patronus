package server

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/hubprotocol"
	"github.com/slaynaw/wakeonlan/agent/internal/devices"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/agents"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/fleet"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/hubtest"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/settings"
	"github.com/slaynaw/wakeonlan/agent/internal/identity"
	"github.com/slaynaw/wakeonlan/agent/protocol"
	"github.com/slaynaw/wakeonlan/agent/protocol2"
)

// testAgent est un vrai agent wolagent/2, sur l'adresse de bouclage.
type testAgent struct {
	port int
	id   *identity.Identity
	reg  *devices.Registry
	a    *hubtest.Agent
}

func startAgent(t *testing.T) *testAgent {
	a := hubtest.StartAgent(t)
	return &testAgent{port: a.Port, id: a.ID, reg: a.Registry, a: a}
}

func (a *testAgent) invite(t *testing.T, role protocol2.Role, only protocol2.Pin) string {
	return a.a.Invite(t, role, only)
}

func (e *env) addPC(t *testing.T, a *testAgent, code string) hubprotocol.Response {
	t.Helper()
	return call(t, e.local(t), hubprotocol.Request{
		Cmd: hubprotocol.CmdPCsAdd, Name: "PC Bureau", Host: "127.0.0.1", Port: a.port, MAC: "aa-bb-cc-dd-ee-01",
		Agent: a.id.Pin.String(), Code: code,
	})
}

func TestPCWithAgent(t *testing.T) {
	e, a := start(t), startAgent(t)
	resp := e.addPC(t, a, a.invite(t, protocol2.RoleWake, e.hub.Pin))
	if !resp.OK || resp.PC == nil || resp.PC.Role != "wake" || resp.PC.MAC != "AA:BB:CC:DD:EE:01" || resp.PC.ID == "" ||
		resp.PC.Agent != a.id.Pin.String() || resp.PC.Port != a.port || resp.PC.Probe != nil {
		t.Fatalf("PC confié : %+v %+v", resp, resp.PC)
	}
	// L'agent a enregistré le hub, sous son nom, avec le rôle de l'invitation.
	d, ok := a.reg.Lookup(e.hub.Pin)
	if !ok || d.Role != protocol2.RoleWake || d.Name != "Hub NAS" {
		t.Fatalf("hub vu par l'agent : %+v, %v", d, ok)
	}
	list := call(t, e.local(t), hubprotocol.Request{Cmd: hubprotocol.CmdPCs})
	if len(list.PCs) != 1 || list.PCs[0].ID != resp.PC.ID {
		t.Fatalf("liste : %+v", list.PCs)
	}
	// Une seconde fois le même agent : refusé avant tout appairage.
	if r := e.addPC(t, a, "7KQ29MXP"); r.Code != protocol2.CodeBadRequest || !strings.Contains(r.Message, "déjà") {
		t.Fatalf("doublon : %+v", r)
	}
	// Modification : nom et MAC ; l'agent ne change pas, une sonde est refusée.
	up := call(t, e.local(t), hubprotocol.Request{Cmd: hubprotocol.CmdPCsUpdate, PC: resp.PC.ID, Name: "Bureau", MAC: "AA:BB:CC:DD:EE:02"})
	if !up.OK || up.PC.Name != "Bureau" || up.PC.MAC != "AA:BB:CC:DD:EE:02" || up.PC.Agent != a.id.Pin.String() || up.PC.Host != "127.0.0.1" {
		t.Fatalf("modification : %+v", up.PC)
	}
	if r := call(t, e.local(t), hubprotocol.Request{Cmd: hubprotocol.CmdPCsUpdate, PC: resp.PC.ID, Probe: &hubprotocol.Probe{Kind: "tcp", Port: 22}}); r.Code != protocol2.CodeBadRequest {
		t.Fatalf("sonde sur un PC avec agent : %+v", r)
	}
	// La liste est gardée chiffrée, et relue.
	again, err := fleet.Open(e.st.File(fleet.FileName), time.Now)
	if err != nil || len(again.List()) != 1 || again.List()[0].Name != "Bureau" || again.List()[0].Role != protocol2.RoleWake {
		t.Fatalf("relecture : %+v, %v", again.List(), err)
	}
	if r := call(t, e.local(t), hubprotocol.Request{Cmd: hubprotocol.CmdPCsRemove, PC: resp.PC.ID}); !r.OK {
		t.Fatalf("retrait : %+v", r)
	}
	if r := call(t, e.local(t), hubprotocol.Request{Cmd: hubprotocol.CmdPCsRemove, PC: resp.PC.ID}); r.Code != protocol2.CodeBadRequest {
		t.Fatalf("second retrait : %+v", r)
	}
	// Confié de nouveau : l'agent connaît déjà le hub, son rôle est repris.
	if r := e.addPC(t, a, "7KQ29MXP"); !r.OK || r.PC.Role != "wake" {
		t.Fatalf("confié de nouveau : %+v", r)
	}
}

func TestPCAdminInviteDowngraded(t *testing.T) {
	e, a := start(t), startAgent(t)
	// Le hub, invité au rôle admin, en est le seul administrateur : il ne peut s'en défaire, et le dit.
	resp := e.addPC(t, a, a.invite(t, protocol2.RoleAdmin, e.hub.Pin))
	if resp.Code != protocol2.CodeBadRequest || !strings.Contains(resp.Message, "wol-agent devices revoke "+e.hub.Pin.Short()) {
		t.Fatalf("seul administrateur : %+v", resp)
	}
	if len(e.srv.cfg.Fleet.List()) != 0 {
		t.Fatal("PC confié malgré le refus")
	}
	if _, err := a.reg.Remove(e.hub.Pin, true); err != nil {
		t.Fatal(err)
	}
	// Cas réel : l'application qui invite est administrateur du PC. Le hub se ramène à wake.
	if _, err := a.reg.Add(devices.Device{Pin: pinOf(t, newDevice(t)), Name: "Pixel 8", Role: protocol2.RoleAdmin, Via: devices.ViaPair}); err != nil {
		t.Fatal(err)
	}
	resp = e.addPC(t, a, a.invite(t, protocol2.RoleAdmin, e.hub.Pin))
	if !resp.OK || resp.PC.Role != "wake" {
		t.Fatalf("invitation admin : %+v", resp)
	}
	if d, _ := a.reg.Lookup(e.hub.Pin); d.Role != protocol2.RoleWake {
		t.Fatalf("rôle du hub sur l'agent : %s", d.Role)
	}
}

func TestPCRefusals(t *testing.T) {
	e, a := start(t), startAgent(t)
	local := e.local(t)
	// Mauvaise empreinte : rien n'est envoyé à l'agent, rien n'est enregistré.
	other := startAgent(t)
	r := call(t, local, hubprotocol.Request{Cmd: hubprotocol.CmdPCsAdd, Name: "PC", Host: "127.0.0.1", Port: a.port, Agent: other.id.Pin.String(), Code: a.invite(t, protocol2.RoleWake, e.hub.Pin)})
	if r.Code != protocol2.CodeBadRequest || len(a.reg.Devices()) != 0 {
		t.Fatalf("mauvaise empreinte : %+v, %v", r, a.reg.Devices())
	}
	// Code faux, invitation réservée à un autre appareil.
	if r := e.addPC(t, a, "7KQ29MXP"); r.Code != protocol2.CodeBadRequest || !strings.Contains(r.Message, "agent") {
		t.Fatalf("code faux : %+v", r)
	}
	if r := e.addPC(t, a, a.invite(t, protocol2.RoleWake, other.id.Pin)); r.Code != protocol2.CodeBadRequest {
		t.Fatalf("invitation d'un autre : %+v", r)
	}
	// Champs invalides.
	for name, req := range map[string]hubprotocol.Request{
		"sans invitation": {Name: "PC", Host: "127.0.0.1", Port: a.port, Agent: a.id.Pin.String()},
		"empreinte":       {Name: "PC", Host: "127.0.0.1", Agent: "abc", Code: "7KQ29MXP"},
		"nom vide":        {Name: " ", Host: "192.168.1.9", Probe: &hubprotocol.Probe{Kind: "tcp", Port: 80}},
		"adresse":         {Name: "Box", Host: "999.1.1.1", Probe: &hubprotocol.Probe{Kind: "tcp", Port: 80}},
		"MAC":             {Name: "Box", Host: "192.168.1.1", MAC: "FF:FF:FF:FF:FF:FF", Probe: &hubprotocol.Probe{Kind: "tcp", Port: 80}},
		"sans sonde":      {Name: "Box", Host: "192.168.1.1"},
		"sonde inconnue":  {Name: "Box", Host: "192.168.1.1", Probe: &hubprotocol.Probe{Kind: "icmp"}},
		"port de sonde":   {Name: "Box", Host: "192.168.1.1", Probe: &hubprotocol.Probe{Kind: "tcp"}},
		"port sans agent": {Name: "Box", Host: "192.168.1.1", Port: 9770, Probe: &hubprotocol.Probe{Kind: "tcp", Port: 80}},
	} {
		req.Cmd = hubprotocol.CmdPCsAdd
		if r := call(t, local, req); r.Code != protocol2.CodeBadRequest {
			t.Errorf("%s : %+v", name, r)
		}
	}
	if n := len(e.srv.cfg.Fleet.List()); n != 0 {
		t.Fatalf("%d PC enregistrés malgré les refus", n)
	}
	// Rôle insuffisant.
	viewer := newDevice(t)
	e.enroll(t, viewer, protocol2.RoleWake, "Tablette")
	if r := call(t, e.dial(t, viewer), hubprotocol.Request{Cmd: hubprotocol.CmdPCs}); r.Code != protocol2.CodeInsufficientRole {
		t.Fatalf("hub.pcs (wake) : %+v", r)
	}
}

func TestDeviceWithoutAgent(t *testing.T) {
	e := start(t)
	local := e.local(t)
	r := call(t, local, hubprotocol.Request{Cmd: hubprotocol.CmdPCsAdd, Name: "Box", Host: "192.168.1.1", Probe: &hubprotocol.Probe{Kind: "https"}})
	if !r.OK || r.PC.Probe == nil || r.PC.Probe.Kind != "https" || r.PC.Agent != "" || r.PC.Role != "" {
		t.Fatalf("appareil sans agent : %+v", r.PC)
	}
	up := call(t, local, hubprotocol.Request{Cmd: hubprotocol.CmdPCsUpdate, PC: r.PC.ID, Probe: &hubprotocol.Probe{Kind: "tcp", Port: 8443}})
	if !up.OK || up.PC.Probe.Kind != "tcp" || up.PC.Probe.Port != 8443 || up.PC.Name != "Box" {
		t.Fatalf("sonde modifiée : %+v", up.PC)
	}
	if bad := call(t, local, hubprotocol.Request{Cmd: hubprotocol.CmdPCsUpdate, PC: r.PC.ID, Port: 9770}); bad.Code != protocol2.CodeBadRequest {
		t.Fatalf("port d'agent sur un appareil sans agent : %+v", bad)
	}
	for range hubprotocol.MaxPCs - 1 {
		call(t, local, hubprotocol.Request{Cmd: hubprotocol.CmdPCsAdd, Name: "Caméra", Host: "192.168.1.50", Probe: &hubprotocol.Probe{Kind: "http"}})
	}
	if over := call(t, local, hubprotocol.Request{Cmd: hubprotocol.CmdPCsAdd, Name: "Une de trop", Host: "192.168.1.51", Probe: &hubprotocol.Probe{Kind: "http"}}); over.Code != protocol2.CodeBadRequest {
		t.Fatalf("au-delà de %d : %+v", hubprotocol.MaxPCs, over)
	}
}

// TestPrivateTargets : le hub ne joint que des adresses privées, jamais l'adresse de bouclage en service.
func TestPrivateTargets(t *testing.T) {
	e, a := start(t), startAgent(t)
	strict := &agents.Dialer{Identity: e.hub, Networks: agents.TargetNetworks()}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, host := range []string{"127.0.0.1", "localhost", "8.8.8.8"} {
		if _, err := strict.Pair(ctx, host, a.port, a.id.Pin, "7KQ29MXP", "Hub NAS"); err == nil || !strings.Contains(err.Error(), "réseau local") {
			t.Errorf("%s : %v", host, err)
		}
	}
	for _, p := range agents.TargetNetworks() {
		if p.Addr().IsLoopback() {
			t.Errorf("bouclage permis : %s", p)
		}
	}
}

func TestArchiveCommands(t *testing.T) {
	e, a := start(t), startAgent(t)
	pc := e.addPC(t, a, a.invite(t, protocol2.RoleWake, e.hub.Pin)).PC
	arch := e.srv.cfg.Archive
	boot := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC).Unix()
	if _, err := arch.AddEvents(pc.ID, []protocol.HistoryEvent{{T: boot, K: protocol.HistoryBoot}, {T: boot + 3600, K: protocol.HistoryShutdown}}); err != nil {
		t.Fatal(err)
	}
	if err := arch.AddDay(pc.ID, "2026-09-01", []protocol.MetricsRow{{T: boot, N: 6}}); err != nil {
		t.Fatal(err)
	}
	viewer := newDevice(t)
	e.enroll(t, viewer, protocol2.RoleView, "Tablette")
	v := e.dial(t, viewer)

	h := call(t, v, hubprotocol.Request{Cmd: hubprotocol.CmdHistory, PC: pc.ID})
	if !h.OK || h.History == nil || h.History.From != boot || len(h.History.Events) != 2 || h.More {
		t.Fatalf("hub.history : %+v %+v", h, h.History)
	}
	if h := call(t, v, hubprotocol.Request{Cmd: hubprotocol.CmdHistory, PC: pc.ID, From: boot + 1}); len(h.History.Events) != 1 {
		t.Fatalf("à partir d'une date : %+v", h.History)
	}
	days := call(t, v, hubprotocol.Request{Cmd: hubprotocol.CmdMetrics, PC: pc.ID})
	if !days.OK || days.Metrics == nil || len(days.Metrics.Days) != 1 || days.Metrics.Days[0] != "2026-09-01" {
		t.Fatalf("jours : %+v", days.Metrics)
	}
	day := call(t, v, hubprotocol.Request{Cmd: hubprotocol.CmdMetrics, PC: pc.ID, Day: "2026-09-01"})
	if !day.OK || len(day.Metrics.Rows) != 1 || day.Metrics.Rows[0].N != 6 {
		t.Fatalf("mesures d'un jour : %+v", day.Metrics)
	}
	if none := call(t, v, hubprotocol.Request{Cmd: hubprotocol.CmdMetrics, PC: pc.ID, Day: "2026-09-02"}); !none.OK || len(none.Metrics.Rows) != 0 {
		t.Fatalf("jour non gardé : %+v", none.Metrics)
	}
	for _, req := range []hubprotocol.Request{
		{Cmd: hubprotocol.CmdHistory, PC: "inconnu"},
		{Cmd: hubprotocol.CmdMetrics, PC: pc.ID, Day: "../../x"},
	} {
		if r := call(t, v, req); r.Code != protocol2.CodeBadRequest {
			t.Errorf("%+v : %+v", req, r)
		}
	}
	// Réglages : lecture (admin), durée de conservation bornée.
	if r := call(t, v, hubprotocol.Request{Cmd: hubprotocol.CmdSettings}); r.Code != protocol2.CodeInsufficientRole {
		t.Fatalf("réglages (view) : %+v", r)
	}
	local := e.local(t)
	if r := call(t, local, hubprotocol.Request{Cmd: hubprotocol.CmdSettings}); !r.OK || r.Settings.Retention != 365 {
		t.Fatalf("réglages par défaut : %+v", r.Settings)
	}
	for _, days := range []int{89, 1096, -5} {
		if r := call(t, local, hubprotocol.Request{Cmd: hubprotocol.CmdSettings, Retention: days}); r.Code != protocol2.CodeBadRequest {
			t.Errorf("conservation de %d jours : %+v", days, r)
		}
	}
	if r := call(t, local, hubprotocol.Request{Cmd: hubprotocol.CmdSettings, Retention: 730}); !r.OK || r.Settings.Retention != 730 {
		t.Fatalf("deux ans : %+v", r.Settings)
	}
	if again, err := settings.Open(e.st.File(settings.FileName)); err != nil || again.Get().RetentionDays != 730 {
		t.Fatalf("réglages relus : %+v, %v", again, err)
	}
	// Un PC retiré emporte son archive.
	call(t, local, hubprotocol.Request{Cmd: hubprotocol.CmdPCsRemove, PC: pc.ID})
	if events, _, _, _ := arch.Events(pc.ID, 0); len(events) != 0 {
		t.Fatalf("archive du PC retiré : %d évènements", len(events))
	}
}

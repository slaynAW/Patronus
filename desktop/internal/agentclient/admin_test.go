package agentclient

import (
	"context"
	"os"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol2"
	"github.com/slaynaw/wakeonlan/desktop/internal/agentclient/agenttest"
	"github.com/slaynaw/wakeonlan/desktop/internal/model"
)

// TestAdmin : administration des appareils par un appareil de rôle admin (§6.2) ; le dernier
// administrateur reste protégé, et un rôle plus faible est refusé.
func TestAdmin(t *testing.T) {
	s, settings, agent := startV2(t, agenttest.Normal)
	ctx := context.Background()
	c, me := newClient(t)
	settings.Key, settings.Pin = "", agent.String()
	s.Register(me, "admin")
	phone := protocol2.PinOf([]byte("téléphone"))
	s.Register(phone, "control")

	a, err := c.Admin(ctx, "127.0.0.1", settings)
	if err != nil || len(a.Devices) != 2 || len(a.Invites) != 0 {
		t.Fatalf("état : %+v %v", a, err)
	}
	self := slices.IndexFunc(a.Devices, func(d protocol2.DeviceInfo) bool { return d.Self })
	if self < 0 || a.Devices[self].Device != me.String() || a.Devices[self].Role != "admin" {
		t.Errorf("appareil qui demande : %+v", a.Devices)
	}

	// Invitation : le code n'est que dans la réponse, la liste ne le donne pas.
	inv, err := c.Invite(ctx, "127.0.0.1", settings, "wake", 2*time.Hour, "Tablette")
	if err != nil || len(inv.Code) != 8 || inv.Role != "wake" || inv.Name != "Tablette" || inv.Agent != agent.String() {
		t.Fatalf("invitation : %+v %v", inv, err)
	}
	if a, _ := c.Admin(ctx, "127.0.0.1", settings); len(a.Invites) != 1 || a.Invites[0].Code != "" || a.Invites[0].ID != inv.ID {
		t.Errorf("invitations : %+v", a.Invites)
	}
	if _, err := c.Invite(ctx, "127.0.0.1", settings, "wake", 8*24*time.Hour, ""); ReasonOf(err) != protocol2.CodeBadRequest {
		t.Errorf("validité trop longue : %v", err)
	}
	if err := c.CancelInvite(ctx, "127.0.0.1", settings, inv.ID); err != nil || s.Invites() != 0 {
		t.Errorf("annulation : %v (%d)", err, s.Invites())
	}

	// Rôle, nom, retrait.
	if err := c.SetRole(ctx, "127.0.0.1", settings, phone.String(), "view"); err != nil {
		t.Fatal(err)
	}
	if err := c.Rename(ctx, "127.0.0.1", settings, phone.String(), "Pixel de Léa"); err != nil {
		t.Fatal(err)
	}
	if role, name := s.Device(phone); role != "view" || name != "Pixel de Léa" {
		t.Errorf("appareil modifié : %q %q", role, name)
	}
	if err := c.Revoke(ctx, "127.0.0.1", settings, phone.String()); err != nil {
		t.Fatal(err)
	}
	if role, _ := s.Device(phone); role != "" {
		t.Error("appareil non retiré")
	}

	// Dernier administrateur : impossible depuis une application.
	if err := c.SetRole(ctx, "127.0.0.1", settings, me.String(), "control"); ReasonOf(err) != protocol2.CodeLastAdmin {
		t.Errorf("rétrogradation du dernier administrateur : %v", err)
	}
	if err := c.Revoke(ctx, "127.0.0.1", settings, me.String()); ReasonOf(err) != protocol2.CodeLastAdmin {
		t.Errorf("retrait du dernier administrateur : %v", err)
	}

	// Rôle insuffisant ; PC encore en wolagent/1 (pas d'empreinte) : rien n'est envoyé.
	s.Register(me, "control")
	if _, err := c.Admin(ctx, "127.0.0.1", settings); ReasonOf(err) != protocol2.CodeInsufficientRole {
		t.Errorf("rôle control : %v", err)
	}
	attempts := s.Attempts()
	if _, err := c.Admin(ctx, "127.0.0.1", model.AgentSettings{Port: settings.Port, Key: "x"}); CodeOf(err) != NoKey || s.Attempts() != attempts {
		t.Errorf("sans empreinte : %v", err)
	}
}

// Administration de bout en bout contre le VRAI agent : un appareil invité avec le rôle admin gère
// les appareils et les invitations, sans pouvoir retirer le dernier administrateur.
func TestEndToEndAdminWithRealAgent(t *testing.T) {
	port, err := strconv.Atoi(os.Getenv("WOL_E2E_PORT"))
	path := os.Getenv("WOL_E2E_IDENTITY")
	if err != nil || path == "" {
		t.Skip("WOL_E2E_PORT ou WOL_E2E_IDENTITY non défini (test lancé par la CI)")
	}
	ctx := context.Background()
	local, agent := agentAdmin(t, path, port)
	settings := model.AgentSettings{Port: port, Pin: agent.String()}
	invite := func(role string) string {
		resp, err := local.Do(ctx, protocol2.Request{Cmd: protocol2.CmdInvite, Role: role, TTL: 120})
		if err != nil || !resp.OK {
			t.Fatalf("invitation : %+v %v", resp, err)
		}
		return resp.Invite.Code
	}
	c, me := newClient(t)
	if _, err := c.Pair(ctx, "127.0.0.1", settings, invite("admin")); err != nil {
		t.Fatal(err)
	}
	other, otherPin := newClient(t)
	if _, err := other.Pair(ctx, "127.0.0.1", settings, invite("view")); err != nil {
		t.Fatal(err)
	}
	a, err := c.Admin(ctx, "127.0.0.1", settings)
	if err != nil || !slices.ContainsFunc(a.Devices, func(d protocol2.DeviceInfo) bool { return d.Self && d.Device == me.String() }) {
		t.Fatalf("état : %+v %v", a, err)
	}
	inv, err := c.Invite(ctx, "127.0.0.1", settings, "control", 10*time.Minute, "Invité CI")
	if err != nil || inv.Agent != agent.String() {
		t.Fatalf("invitation par l'application : %+v %v", inv, err)
	}
	if err := c.CancelInvite(ctx, "127.0.0.1", settings, inv.ID); err != nil {
		t.Errorf("annulation : %v", err)
	}
	if err := c.SetRole(ctx, "127.0.0.1", settings, otherPin.String(), "wake"); err != nil {
		t.Errorf("rôle : %v", err)
	}
	if err := c.Revoke(ctx, "127.0.0.1", settings, otherPin.String()); err != nil {
		t.Errorf("retrait : %v", err)
	}
	// Retrait de cet ordinateur par le PC (nettoyage) : il n'était pas le dernier administrateur si
	// d'autres tests en ont ajouté ; sinon, seul le PC le peut (force).
	force := true
	if resp, err := local.Do(ctx, protocol2.Request{Cmd: protocol2.CmdDevicesRevoke, Device: me.String(), Force: &force}); err != nil || !resp.OK {
		t.Errorf("nettoyage : %+v %v", resp, err)
	}
}

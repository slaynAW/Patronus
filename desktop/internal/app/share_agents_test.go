package app

import (
	"context"
	"crypto/tls"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol2"
	"github.com/slaynaw/wakeonlan/desktop/internal/agentclient"
	"github.com/slaynaw/wakeonlan/desktop/internal/identity"
	"github.com/slaynaw/wakeonlan/desktop/internal/model"
	"github.com/slaynaw/wakeonlan/desktop/internal/share"
	"github.com/slaynaw/wakeonlan/desktop/internal/status"
)

// v2Online est l'état d'un PC allumé, joint en wolagent/2 avec le rôle donné.
func v2Online(role string) status.DeviceStatus {
	return status.DeviceStatus{State: status.Online, Agent: &agentclient.Status{Proto: protocol2.ALPN, Role: role}}
}

// connectOwner connecte la personne qui partage à GitHub et renvoie sa clé publique de signature.
func connectOwner(t *testing.T, owner *Service) string {
	t.Helper()
	call(t, owner, "shareLogin", map[string]any{"name": "Hugo"})
	waitFor(t, "connexion GitHub", func() bool { o := owner.State().Share.Owner; return o != nil && o.Connected })
	inv, err := share.ParseInvite(call(t, owner, "shareInvite", nil)["link"].(string))
	if err != nil {
		t.Fatal(err)
	}
	return inv.Owner
}

// TestShareV2Owner suit le partage v2 du côté de la personne qui partage, administratrice d'un PC :
// invitation réservée à l'appareil de Léa, fichier d'accès sans clé, enregistrement constaté, rôle
// aligné, retrait. L'appareil de Léa est simulé par une seconde identité qui utilise l'invitation.
func TestShareV2Owner(t *testing.T) {
	gists, srv := newGistServer(t)
	ctx := context.Background()
	agent, key, agentPin := agentV2(t)
	pc := testDevice(t, "dev-1", "Bureau", "127.0.0.1", key)
	pc.Agent.Port, pc.Agent.Pin = agent.Port(), agentPin.String()
	owner := newShareService(t, srv, pc)
	me, err := owner.identity.Get()
	if err != nil {
		t.Fatal(err)
	}
	agent.Register(me.Pin, "admin")
	ownerPub := connectOwner(t, owner)

	// Appareil de Léa : clé de réception (partage) et identité wolagent/2.
	leaShare, _ := share.NewDeviceKey()
	lea, err := identity.NewStore(t.TempDir(), time.Now, t.Logf).Get()
	if err != nil {
		t.Fatal(err)
	}
	req := share.Request{Name: "Léa", Device: share.DevicePublic(leaShare), Owner: share.KeyID(ownerPub), Identity: lea.Pin.String()}
	read := call(t, owner, "shareReadRequest", map[string]any{"text": req.Link()})
	if read["ok"] != true || read["identity"] != lea.Pin.String() {
		t.Fatalf("lecture de la demande : %v", read)
	}
	devices := read["devices"].([]any)
	if d := devices[0].(map[string]any); d["pinned"] != true || d["canShutdown"] != true {
		t.Fatalf("PC proposés : %v", devices)
	}
	call(t, owner, "shareGrant", map[string]any{"device": req.Device, "name": "Léa", "identity": lea.Pin.String(),
		"rights": map[string]string{"dev-1": "view"}})
	person := owner.State().Share.Owner.People[0]
	if person.Rights["dev-1"] != share.RightView || person.Identity != lea.Pin.String() || person.Agents["dev-1"]["state"] != "pending" {
		t.Fatalf("personne : %+v", person)
	}
	// Plus personne n'a besoin de la clé de l'agent : elle pourra être effacée.
	if owner.sharedWithControl("dev-1") {
		t.Error("clé gardée pour un partage v2")
	}

	// Avant l'invitation : le PC est partagé sans agent.
	readContent := func() share.Content {
		t.Helper()
		if err := owner.publishShares(ctx); err != nil {
			t.Fatal(err)
		}
		gists.mu.Lock()
		var text string
		for _, files := range gists.files {
			text = files[share.FileName(req.Device)]
		}
		gists.mu.Unlock()
		opened, err := share.Open([]byte(text), ownerPub, leaShare, 0)
		if err != nil {
			t.Fatal(err)
		}
		return opened.Content
	}
	if c := readContent(); c.Identity != lea.Pin.String() || c.Devices[0].Agent != nil || len(c.Invites) != 0 {
		t.Fatalf("contenu avant l'invitation : %+v", c)
	}

	// PC allumé, cet ordinateur administrateur : invitation réservée à Léa, rôle « wake », 7 jours.
	owner.considerShareAgent("dev-1", v2Online("admin"))
	waitFor(t, "invitation", func() bool { return len(agent.InviteList()) == 1 && !owner.sharing.agents.isRunning("dev-1") })
	inv := agent.InviteList()[0]
	if inv.Device != lea.Pin.String() || inv.Role != "wake" || inv.Name != "Léa" || inv.Expires < time.Now().Add(6*24*time.Hour).Unix() {
		t.Fatalf("invitation : %+v", inv)
	}
	c := readContent()
	if a := c.Devices[0].Agent; a == nil || a.Key != "" || a.Pin != agentPin.String() || len(c.Invites) != 1 || c.Invites[0].Role != "wake" {
		t.Fatalf("contenu avec l'invitation : %+v", c)
	}
	if st := owner.State().Share.Owner.People[0].Agents["dev-1"]; st["state"] != "invited" {
		t.Errorf("état : %v", st)
	}
	// Rien de plus à faire : pas de nouvelle session avant la relecture périodique.
	if work, poll := owner.sharing.state.Owner.AgentWork("dev-1", time.Now()); work || !poll {
		t.Errorf("travail : %t %t", work, poll)
	}

	// Réservée à Léa, l'invitation ne sert à aucune autre identité.
	pcAgent := model.AgentSettings{Port: agent.Port(), Pin: agentPin.String()}
	intruder, err := identity.NewStore(t.TempDir(), time.Now, t.Logf).Get()
	if err != nil {
		t.Fatal(err)
	}
	other := agentclient.New()
	other.Identity = func() (tls.Certificate, error) { return intruder.Cert, nil }
	if _, err := other.Pair(ctx, "127.0.0.1", pcAgent, c.Invites[0].Code); agentclient.CodeOf(err) != agentclient.PairingFailed {
		t.Fatalf("invitation utilisée par une autre identité : %v", err)
	}
	// Léa l'utilise avec son identité : enregistrée par le PC.
	guest := agentclient.New()
	guest.Identity = func() (tls.Certificate, error) { return lea.Cert, nil }
	if _, err := guest.Pair(ctx, "127.0.0.1", pcAgent, c.Invites[0].Code); err != nil {
		t.Fatal(err)
	}
	if role, _ := agent.Device(lea.Pin); role != "wake" {
		t.Fatalf("Léa sur l'agent : %q", role)
	}
	if err := owner.syncShareAgent(ctx, "dev-1"); err != nil {
		t.Fatal(err)
	}
	if c := readContent(); len(c.Invites) != 0 || c.Devices[0].Agent == nil {
		t.Fatalf("contenu après l'enregistrement : %+v", c)
	}
	if st := owner.State().Share.Owner.People[0].Agents["dev-1"]; st["state"] != "registered" {
		t.Errorf("état : %v", st)
	}

	// Droit étendu à « démarrer et éteindre » : rôle aligné sur « control ».
	call(t, owner, "shareGrant", map[string]any{"device": req.Device, "name": "Léa", "identity": lea.Pin.String(),
		"rights": map[string]string{"dev-1": "full"}})
	if err := owner.syncShareAgent(ctx, "dev-1"); err != nil {
		t.Fatal(err)
	}
	if role, _ := agent.Device(lea.Pin); role != "control" {
		t.Fatalf("rôle après l'extension : %q", role)
	}

	// Cet ordinateur perd le rôle « admin » : le partage v2 attend, l'interface le dit.
	call(t, owner, "shareGrant", map[string]any{"device": req.Device, "name": "Léa", "identity": lea.Pin.String(),
		"rights": map[string]string{"dev-1": "view"}})
	owner.considerShareAgent("dev-1", v2Online("control"))
	if !owner.sharing.agents.notAdminOf("dev-1") {
		t.Error("non administrateur non signalé")
	}
	if role, _ := agent.Device(lea.Pin); role != "control" {
		t.Errorf("session sans le rôle admin : %q", role)
	}

	// Retrait : appareil de Léa retiré du PC dès que cet ordinateur le joint en administrateur.
	call(t, owner, "shareRevoke", map[string]any{"device": req.Device})
	if n := owner.State().Share.Owner.Revocations; n != 1 {
		t.Fatalf("retraits en attente : %d", n)
	}
	owner.considerShareAgent("dev-1", v2Online("admin"))
	waitFor(t, "retrait", func() bool { role, _ := agent.Device(lea.Pin); return role == "" })
	waitFor(t, "file vide", func() bool { return owner.State().Share.Owner.Revocations == 0 })
	if role, _ := agent.Device(me.Pin); role != "admin" {
		t.Errorf("cet ordinateur sur l'agent : %q", role)
	}
}

// Une personne dont la demande ne porte pas d'empreinte reste en partage v1 : la clé de l'agent est
// gardée et transmise pour « démarrer et éteindre » ; « démarrer et consulter » devient « démarrer ».
func TestShareV1PersonKeepsKey(t *testing.T) {
	_, srv := newGistServer(t)
	agent, key, agentPin := agentV2(t)
	pc := testDevice(t, "dev-1", "Bureau", "127.0.0.1", key)
	pc.Agent.Port, pc.Agent.Pin = agent.Port(), agentPin.String()
	owner := newShareService(t, srv, pc)
	ownerPub := connectOwner(t, owner)
	guest, _ := share.NewDeviceKey()
	req := share.Request{Name: "Paul", Device: share.DevicePublic(guest), Owner: share.KeyID(ownerPub)}
	if read := call(t, owner, "shareReadRequest", map[string]any{"text": req.Link()}); read["identity"] != "" {
		t.Fatalf("empreinte : %v", read)
	}
	call(t, owner, "shareGrant", map[string]any{"device": req.Device, "name": "Paul", "rights": map[string]string{"dev-1": "view"}})
	if r := owner.State().Share.Owner.People[0].Rights["dev-1"]; r != share.RightWake {
		t.Errorf("droit : %s", r)
	}
	call(t, owner, "shareGrant", map[string]any{"device": req.Device, "name": "Paul", "rights": map[string]string{"dev-1": "full"}})
	if !owner.sharedWithControl("dev-1") {
		t.Error("clé non gardée pour le partage v1")
	}
	// Empreinte invalide refusée.
	if _, err := owner.Call("shareGrant", []byte(`{"device":"`+req.Device+`","name":"Paul","identity":"AAAA","rights":{"dev-1":"full"}}`)); err == nil {
		t.Error("empreinte invalide acceptée")
	}
	// Aucune session du partage v2 : rien à faire sur ce PC.
	owner.considerShareAgent("dev-1", v2Online("admin"))
	if owner.sharing.agents.isRunning("dev-1") || len(agent.InviteList()) != 0 {
		t.Error("session inutile")
	}
}

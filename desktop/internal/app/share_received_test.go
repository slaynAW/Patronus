package app

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/slaynaw/wakeonlan/agent/protocol2"
	"github.com/slaynaw/wakeonlan/desktop/internal/agentclient"
	"github.com/slaynaw/wakeonlan/desktop/internal/status"
)

// TestShareV2EndToEnd suit le partage v2 entre deux applications Windows : la demande de Léa porte
// l'empreinte de son ordinateur ; Hugo, administrateur du PC, y crée l'invitation ; l'ordinateur de
// Léa l'utilise dès que le PC répond, puis le joint en wolagent/2 avec son propre rôle ; retiré par le
// PC, il le traite de nouveau en « démarrer ».
func TestShareV2EndToEnd(t *testing.T) {
	_, srv := newGistServer(t)
	ctx := context.Background()
	agent, key, agentPin := agentV2(t)
	pc := testDevice(t, "dev-1", "Bureau", "127.0.0.1", key)
	pc.Agent.Port, pc.Agent.Pin = agent.Port(), agentPin.String()
	owner := newShareService(t, srv, pc)
	guest := newShareService(t, srv)
	me, err := owner.identity.Get()
	if err != nil {
		t.Fatal(err)
	}
	agent.Register(me.Pin, "admin")
	connectOwner(t, owner)

	// Demande de Léa : son empreinte wolagent/2 dans le lien (paramètre « id »).
	link := call(t, owner, "shareInvite", nil)["link"].(string)
	req := call(t, guest, "shareRequest", map[string]any{"text": link, "name": "Léa"})
	lea, err := guest.identity.Get()
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(strings.Replace(req["link"].(string), "wolshare://", "https://", 1))
	if u.Query().Get("id") != lea.Pin.String() {
		t.Fatalf("demande sans l'empreinte de cet ordinateur : %s", req["link"])
	}
	read := call(t, owner, "shareReadRequest", map[string]any{"text": req["link"]})
	if read["identity"] != lea.Pin.String() || read["code"] != req["code"] {
		t.Fatalf("lecture de la demande : %v", read)
	}
	call(t, owner, "shareGrant", map[string]any{"device": read["device"], "name": "Léa", "identity": read["identity"],
		"rights": map[string]string{"dev-1": "view"}})
	owner.considerShareAgent("dev-1", v2Online("admin"))
	waitFor(t, "invitation", func() bool { return len(agent.InviteList()) == 1 && !owner.sharing.agents.isRunning("dev-1") })
	if err := owner.publishShares(ctx); err != nil {
		t.Fatal(err)
	}

	// Léa reçoit le PC : sans agent tant qu'elle n'est pas enregistrée (aucune connexion à l'agent).
	ownerKey := guest.State().Share.Received[0].Owner
	guest.syncAccess(ctx, ownerKey)
	st := guest.State()
	if len(st.Devices) != 1 || st.Devices[0].HasAgent || st.Share.Received[0].Mismatch {
		t.Fatalf("PC reçu avant l'enregistrement : %+v %+v", st.Devices, st.Share.Received)
	}
	sharedID := st.Devices[0].ID
	attempts := agent.Attempts()

	// PC éteint : l'invitation attend.
	guest.considerReceived(sharedID, status.DeviceStatus{State: status.Offline})
	if guest.sharing.received.isRunning(sharedID) || agent.Attempts() != attempts {
		t.Fatal("invitation utilisée PC éteint")
	}
	// PC allumé : invitation utilisée, ordinateur de Léa enregistré avec le rôle « wake ».
	guest.considerReceived(sharedID, status.DeviceStatus{State: status.Online})
	waitFor(t, "enregistrement de Léa", func() bool { role, _ := agent.Device(lea.Pin); return role == "wake" })
	waitFor(t, "PC joint en wolagent/2", func() bool { return guest.State().Devices[0].HasAgent })
	d, _ := guest.device(sharedID)
	if d.Agent.Key != "" || d.Agent.Pin != agentPin.String() {
		t.Fatalf("agent du PC reçu : %v", d.Agent)
	}
	got, err := guest.agent.Status(ctx, d.Host, *d.Agent)
	if err != nil || got.Role != "wake" || got.Proto != protocol2.ALPN {
		t.Fatalf("état lu par Léa : %+v %v", got, err)
	}
	if !roleCanControl(status.DeviceStatus{Agent: &agentclient.Status{Proto: protocol2.ALPN, Role: "control"}}) ||
		roleCanControl(status.DeviceStatus{Agent: &agentclient.Status{Proto: protocol2.ALPN, Role: "wake"}}) {
		t.Error("extinction proposée selon le rôle")
	}
	// Une seule tentative par invitation : déjà enregistrée, plus rien n'est envoyé.
	attempts = agent.Attempts()
	guest.considerReceived(sharedID, status.DeviceStatus{State: status.Online})
	if agent.Attempts() != attempts {
		t.Error("invitation réutilisée")
	}

	// Hugo constate l'enregistrement : l'invitation quitte le fichier d'accès.
	if err := owner.syncShareAgent(ctx, "dev-1"); err != nil {
		t.Fatal(err)
	}
	if st := owner.State().Share.Owner.People[0].Agents["dev-1"]; st["state"] != "registered" {
		t.Errorf("état chez Hugo : %v", st)
	}

	// Retiré par le PC (wol-agent devices revoke) : Léa le traite de nouveau en « démarrer ».
	agent.Register(lea.Pin, "")
	guest.considerReceived(sharedID, status.DeviceStatus{State: status.Online, AgentError: agentclient.Unregistered})
	waitFor(t, "PC de nouveau sans agent", func() bool { return !guest.State().Devices[0].HasAgent })
}

// Demande modifiée en route (autre empreinte) : le PC reste en « démarrer », aucune invitation n'est
// utilisée, et l'interface le signale.
func TestShareV2Mismatch(t *testing.T) {
	_, srv := newGistServer(t)
	ctx := context.Background()
	agent, key, agentPin := agentV2(t)
	pc := testDevice(t, "dev-1", "Bureau", "127.0.0.1", key)
	pc.Agent.Port, pc.Agent.Pin = agent.Port(), agentPin.String()
	owner := newShareService(t, srv, pc)
	guest := newShareService(t, srv)
	me, _ := owner.identity.Get()
	agent.Register(me.Pin, "admin")
	connectOwner(t, owner)
	link := call(t, owner, "shareInvite", nil)["link"].(string)
	req := call(t, guest, "shareRequest", map[string]any{"text": link, "name": "Léa"})
	read := call(t, owner, "shareReadRequest", map[string]any{"text": req["link"]})
	other := protocol2.PinOf([]byte("autre appareil")).String()
	call(t, owner, "shareGrant", map[string]any{"device": read["device"], "name": "Léa", "identity": other,
		"rights": map[string]string{"dev-1": "full"}})
	owner.considerShareAgent("dev-1", v2Online("admin"))
	waitFor(t, "invitation", func() bool { return len(agent.InviteList()) == 1 && !owner.sharing.agents.isRunning("dev-1") })
	_ = owner.publishShares(ctx)
	ownerKey := guest.State().Share.Received[0].Owner
	guest.syncAccess(ctx, ownerKey)
	st := guest.State()
	if !st.Share.Received[0].Mismatch || st.Devices[0].HasAgent {
		t.Fatalf("demande modifiée : %+v %+v", st.Share.Received[0], st.Devices[0])
	}
	attempts := agent.Attempts()
	guest.considerReceived(st.Devices[0].ID, status.DeviceStatus{State: status.Online})
	if agent.Attempts() != attempts || guest.sharing.received.isRunning(st.Devices[0].ID) {
		t.Error("invitation d'une autre identité utilisée")
	}
}

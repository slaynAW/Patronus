package app

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol"
	"github.com/slaynaw/wakeonlan/agent/protocol2"
	"github.com/slaynaw/wakeonlan/desktop/internal/agentclient"
	"github.com/slaynaw/wakeonlan/desktop/internal/agentclient/agenttest"
	"github.com/slaynaw/wakeonlan/desktop/internal/model"
	"github.com/slaynaw/wakeonlan/desktop/internal/share"
	"github.com/slaynaw/wakeonlan/desktop/internal/status"
)

// agentV2 démarre un faux agent qui sert wolagent/1 et wolagent/2.
func agentV2(t *testing.T) (*agenttest.Server, string, protocol2.Pin) {
	t.Helper()
	key, _ := protocol.NewKey()
	a, err := agenttest.Start(model.DecodeAgentKey(key), agenttest.Normal)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Close)
	return a, key, a.EnableV2()
}

func addAgentDevice(t *testing.T, s *Service, port int, key string) string {
	t.Helper()
	form := model.NewForm()
	form.Name, form.MAC, form.Host = "PC Bureau", "aa:bb:cc:dd:ee:07", "127.0.0.1"
	form.AgentEnabled, form.AgentPort, form.AgentKey = true, strconv.Itoa(port), key
	r := call(t, s, "saveDevice", map[string]any{"form": form})
	id, _ := r["id"].(string)
	if r["ok"] != true || id == "" {
		t.Fatalf("enregistrement : %v", r)
	}
	return id
}

// announced est l'état d'un PC dont l'agent annonce wolagent/2 dans une réponse wolagent/1.
func announced(pin protocol2.Pin) status.DeviceStatus {
	return status.DeviceStatus{State: status.Online, Agent: &agentclient.Status{Proto: protocol.Proto,
		Protocols: []string{protocol.Proto, protocol2.ALPN}, Pin: pin.String()}}
}

func agentOf(t *testing.T, s *Service, id string) model.AgentSettings {
	t.Helper()
	d, ok := s.ownDevice(id)
	if !ok || d.Agent == nil {
		t.Fatalf("PC %s sans agent", id)
	}
	return *d.Agent
}

func TestUpgradeToV2(t *testing.T) {
	s, _ := newService(t)
	agent, key, pin := agentV2(t)
	id := addAgentDevice(t, s, agent.Port(), key)
	ctx := context.Background()

	// L'agent annonce wolagent/2 : passage automatique, migration, clé effacée.
	s.considerUpgrade(id, announced(pin))
	waitFor(t, "passage en wolagent/2", func() bool { return agentOf(t, s, id).HasPin() })
	if a := agentOf(t, s, id); a.Pin != pin.String() || a.HasKey() {
		t.Fatalf("après le passage : %v", a)
	}
	me := s.identity.Existing()
	if role, name := agent.Device(me.Pin); role != "control" || name != s.agent.By {
		t.Fatalf("enregistrement sur l'agent : %q %q", role, name)
	}
	v := s.State().Devices[0]
	if v.AgentPin != pin.Short() || v.ShareControl || !v.CanShutdown || !v.HasAgent {
		t.Errorf("vue : %+v", v)
	}

	// Commandes en wolagent/2, au nom enregistré par l'agent.
	if r := call(t, s, "power", map[string]any{"id": id, "action": "shutdown"}); r["ok"] != true {
		t.Fatalf("extinction : %v", r)
	}
	reqs := agent.Requests()
	if last := reqs[len(reqs)-1]; last.Cmd != "shutdown" || last.By != s.agent.By {
		t.Errorf("requête : %+v", last)
	}
	test := map[string]any{"id": id, "host": "127.0.0.1", "port": strconv.Itoa(agent.Port()), "key": ""}
	if r := call(t, s, "testAgent", test); r["ok"] != true || r["status"].(map[string]any)["proto"] != protocol2.ALPN {
		t.Errorf("test sans clé : %v", r)
	}

	// La fiche ne montre plus de clé ; enregistrée telle quelle, l'empreinte reste.
	got := call(t, s, "getDevice", map[string]any{"id": id})
	form := got["form"].(map[string]any)
	if got["agentPin"] != pin.Short() || form["agentKey"] != "" {
		t.Fatalf("fiche : %v", got)
	}
	if r := call(t, s, "saveDevice", map[string]any{"id": id, "form": form}); r["ok"] != true || agentOf(t, s, id).Pin != pin.String() {
		t.Fatalf("fiche enregistrée : %v %v", r, agentOf(t, s, id))
	}

	// Nouvelle clé saisie (lien d'appairage collé) : l'empreinte est oubliée, puis réapprise de
	// l'agent ; déjà enregistré, cet ordinateur n'a pas à migrer à nouveau.
	form["agentKey"] = key
	call(t, s, "saveDevice", map[string]any{"id": id, "form": form})
	if a := agentOf(t, s, id); a.HasPin() || a.Key != key {
		t.Fatalf("nouvelle clé : %v", a)
	}
	if err := s.upgradeAgent(ctx, id); err != nil {
		t.Fatal(err)
	}
	if a := agentOf(t, s, id); a.Pin != pin.String() || a.HasKey() {
		t.Fatalf("après le second passage : %v", a)
	}

	// Révoqué par l'administrateur du PC : aucune nouvelle migration (plus de clé).
	agent.Register(me.Pin, "")
	before := len(agent.Commands())
	s.considerUpgrade(id, status.DeviceStatus{State: status.Online, AgentError: agentclient.Unregistered})
	time.Sleep(50 * time.Millisecond)
	if role, _ := agent.Device(me.Pin); role != "" || len(agent.Commands()) != before {
		t.Error("cet ordinateur s'est réenregistré malgré la révocation")
	}
}

func TestProbeUsesV2OncePinned(t *testing.T) {
	s, _ := newService(t)
	agent, key, pin := agentV2(t)
	id := addAgentDevice(t, s, agent.Port(), key)
	if err := s.upgradeAgent(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	d, _ := s.ownDevice(id)
	d = d.Clone() // modifiée plus bas : jamais la configuration du service
	hp := status.NewHostProber()
	hp.Agent = s.agent
	r := hp.Probe(context.Background(), d)
	if !r.Reachable || r.Method != status.MethodAgent || r.Agent == nil || r.Agent.Proto != protocol2.ALPN || r.Agent.Role != "control" {
		t.Fatalf("sonde : %+v %+v", r, r.Agent)
	}
	// Autre agent à cette adresse (empreinte différente) : signalé, la machine répond.
	d.Agent.Pin = protocol2.PinOf([]byte("autre")).String()
	if r := hp.Probe(context.Background(), d); !r.Reachable || r.AgentError != agentclient.PinMismatch {
		t.Fatalf("empreinte différente : %+v", r)
	}
	_ = pin
}

func TestUpgradeKeepsKeySharedWithControl(t *testing.T) {
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

	// Passé en wolagent/2 pour ses propres échanges, la clé est gardée : le partage la transmet.
	if err := s.upgradeAgent(context.Background(), "dev-1"); err != nil {
		t.Fatal(err)
	}
	if a := agentOf(t, s, "dev-1"); a.Pin != pin.String() || a.Key != key {
		t.Fatalf("clé non gardée : %v", a)
	}
	if v := s.State().Devices[0]; !v.ShareControl || v.AgentPin != pin.Short() {
		t.Errorf("vue : %+v", v)
	}

	// Partage retiré : la clé est effacée au passage suivant.
	s.sharing.mu.Lock()
	s.sharing.state.Owner.People = nil
	s.sharing.mu.Unlock()
	s.considerUpgrade("dev-1", status.DeviceStatus{State: status.Online, Agent: &agentclient.Status{Proto: protocol2.ALPN}})
	waitFor(t, "clé effacée", func() bool { return !agentOf(t, s, "dev-1").HasKey() })
}

func TestUpgradeNeedsAnnouncementAndBacksOff(t *testing.T) {
	s, _ := newService(t)
	key, _ := protocol.NewKey()
	v1, err := agenttest.Start(model.DecodeAgentKey(key), agenttest.Normal)
	if err != nil {
		t.Fatal(err)
	}
	defer v1.Close()
	id := addAgentDevice(t, s, v1.Port(), key)
	// Agent qui ne sert que wolagent/1 : rien ne change.
	if err := s.upgradeAgent(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if a := agentOf(t, s, id); a.HasPin() || a.Key != key {
		t.Fatalf("agent wolagent/1 : %v", a)
	}
	// Annonce sans empreinte (réponse incomplète) : rien n'est tenté.
	st := announced(protocol2.Pin{})
	st.Agent.Pin = ""
	s.considerUpgrade(id, st)
	s.upgrades.mu.Lock()
	started := len(s.upgrades.running) + len(s.upgrades.next)
	s.upgrades.mu.Unlock()
	if started != 0 {
		t.Error("passage tenté sans empreinte annoncée")
	}

	// Échec (clé refusée) : nouvel essai retardé.
	other, _ := protocol.NewKey()
	v2, err := agenttest.Start(model.DecodeAgentKey(other), agenttest.Normal)
	if err != nil {
		t.Fatal(err)
	}
	defer v2.Close()
	pin := v2.EnableV2()
	id2 := addAgentDevice(t, s, v2.Port(), key)
	s.considerUpgrade(id2, announced(pin))
	waitFor(t, "délai après échec", func() bool {
		s.upgrades.mu.Lock()
		defer s.upgrades.mu.Unlock()
		return !s.upgrades.running[id2] && s.upgrades.next[id2].After(time.Now())
	})
	s.considerUpgrade(id2, announced(pin))
	s.upgrades.mu.Lock()
	running := s.upgrades.running[id2]
	s.upgrades.mu.Unlock()
	if running || agentOf(t, s, id2).HasPin() {
		t.Error("nouvel essai avant le délai")
	}
}

// Passage automatique de bout en bout contre le VRAI agent Go, lancé en simulation par la CI.
func TestEndToEndUpgradeWithRealAgent(t *testing.T) {
	port, err := strconv.Atoi(os.Getenv("WOL_E2E_PORT"))
	if err != nil {
		t.Skip("WOL_E2E_PORT non défini (test lancé par la CI)")
	}
	s, _ := newService(t)
	id := addAgentDevice(t, s, port, os.Getenv("WOL_E2E_KEY"))
	if err := s.upgradeAgent(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	a := agentOf(t, s, id)
	if !a.HasPin() || a.HasKey() {
		t.Fatalf("après le passage : %v", a)
	}
	if r := call(t, s, "power", map[string]any{"id": id, "action": "sleep"}); r["ok"] != true {
		t.Fatalf("mise en veille simulée : %v", r)
	}
	d, _ := s.ownDevice(id)
	hp := status.NewHostProber()
	hp.Agent = s.agent
	if r := hp.Probe(context.Background(), d); r.Agent == nil || r.Agent.Proto != protocol2.ALPN || r.Agent.Role != "control" {
		t.Fatalf("sonde : %+v", r)
	}
}

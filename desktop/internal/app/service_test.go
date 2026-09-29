package app

import (
	"context"
	"encoding/json"
	"net"
	"net/netip"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol"
	"github.com/slaynaw/wakeonlan/desktop/internal/agentclient/agenttest"
	"github.com/slaynaw/wakeonlan/desktop/internal/config"
	"github.com/slaynaw/wakeonlan/desktop/internal/history"
	"github.com/slaynaw/wakeonlan/desktop/internal/model"
	"github.com/slaynaw/wakeonlan/desktop/internal/netstate"
	"github.com/slaynaw/wakeonlan/desktop/internal/status"
)

type fakePlatform struct {
	saved      []byte
	savedName  string
	installer  []string // chemin et empreinte passés à RunInstaller
	installed  []byte   // contenu du fichier au moment du lancement
	cancel     bool
	opened     []string
	clipboard  string
	relaunched atomic.Int32
}

func (f *fakePlatform) WriteClipboard(text string) error {
	f.clipboard = text
	return nil
}

func (f *fakePlatform) SaveFile(name string, content []byte) (string, error) {
	if f.cancel {
		return "", ErrCancelled
	}
	f.saved, f.savedName = content, name
	return `C:\sauvegarde.json`, nil
}

func (f *fakePlatform) OpenURL(url string) error {
	f.opened = append(f.opened, url)
	return nil
}

func (f *fakePlatform) ReadClipboard() (string, error) { return f.clipboard, nil }

func (f *fakePlatform) RunInstaller(path, sha256 string) error {
	if f.cancel {
		return ErrCancelled
	}
	f.installer = []string{path, sha256}
	f.installed, _ = os.ReadFile(path)
	return nil
}

func (f *fakePlatform) Relaunch() error {
	f.relaunched.Add(1)
	return nil
}

func newService(t *testing.T) (*Service, *fakePlatform) {
	t.Helper()
	store, err := config.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	platform := &fakePlatform{}
	s := New(Options{
		Version: "test", Store: store, Platform: platform,
		Prober:   status.ProberFunc(func(context.Context, model.Device) status.ProbeResult { return status.ProbeResult{} }),
		NetState: func() (netstate.State, error) { return netstate.State{}, nil },
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go s.Run(ctx)
	return s, platform
}

func call(t *testing.T, s *Service, method string, params any) map[string]any {
	t.Helper()
	raw, _ := json.Marshal(params)
	result, err := s.Call(method, raw)
	if err != nil {
		t.Fatalf("%s : %v", method, err)
	}
	if result == nil {
		return nil
	}
	// Aller-retour JSON, comme pour l'interface.
	data, _ := json.Marshal(result)
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	return out
}

func waitState(t *testing.T, s *Service, id string, want status.PowerState) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, d := range s.State().Devices {
			if d.ID == id && d.Status.State == want {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("état %s non atteint : %+v", want, s.State().Devices)
}

func TestDeviceLifecycleWakeAndPower(t *testing.T) {
	s, _ := newService(t)
	receiver, _ := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	defer receiver.Close()
	keyText, _ := protocol.NewKey()
	agent, err := agenttest.Start(model.DecodeAgentKey(keyText), agenttest.Normal)
	if err != nil {
		t.Fatal(err)
	}
	defer agent.Close()

	form := model.NewForm()
	form.Name, form.MAC, form.Host = "PC Test", "aa:bb:cc:dd:ee:01", "127.0.0.1"
	form.Broadcast = "127.0.0.1"
	form.WolPort = strconv.Itoa(receiver.LocalAddr().(*net.UDPAddr).Port)
	form.AgentEnabled, form.AgentPort, form.AgentKey = true, strconv.Itoa(agent.Port()), keyText

	bad := form
	bad.MAC = "zz"
	if r := call(t, s, "saveDevice", map[string]any{"form": bad}); r["ok"] != false || r["errors"].(map[string]any)["MAC"] == nil {
		t.Fatalf("erreur de validation attendue : %v", r)
	}
	r := call(t, s, "saveDevice", map[string]any{"form": form})
	id, _ := r["id"].(string)
	if r["ok"] != true || id == "" {
		t.Fatalf("enregistrement : %v", r)
	}
	state := s.State()
	if len(state.Devices) != 1 || !state.Devices[0].CanShutdown || !state.HasSecrets || state.Network.Connected {
		t.Fatalf("état : %+v", state)
	}
	if data, _ := json.Marshal(state); strings.Contains(string(data), keyText) {
		t.Fatal("la clé ne doit jamais être envoyée avec l'état de la liste")
	}
	if got := call(t, s, "getDevice", map[string]any{"id": id}); got["form"].(map[string]any)["agentKey"] != keyText {
		t.Errorf("formulaire : %v", got)
	}

	// Pas de réseau local : état inconnu plutôt que faux « éteint ».
	waitState(t, s, id, status.Unknown)

	if r := call(t, s, "wake", map[string]any{"id": id}); r["ok"] != true {
		t.Fatalf("réveil : %v", r)
	}
	_ = receiver.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 200)
	if n, err := receiver.Read(buf); err != nil || n != 102 {
		t.Fatalf("paquet magique non reçu : %v", err)
	}

	if r := call(t, s, "power", map[string]any{"id": id, "action": "shutdown", "force": true}); r["ok"] != true {
		t.Fatalf("extinction : %v", r)
	}
	if cmds := agent.Commands(); len(cmds) != 1 || cmds[0] != "shutdown" {
		t.Errorf("commandes reçues : %v", cmds)
	}
	if r := call(t, s, "testAgent", map[string]any{"host": "127.0.0.1", "port": strconv.Itoa(agent.Port()), "key": keyText}); r["ok"] != true {
		t.Errorf("test de l'agent : %v", r)
	}
	if r := call(t, s, "testAgent", map[string]any{"host": "pas valide", "port": "x"}); r["invalid"] == nil {
		t.Errorf("test invalide : %v", r)
	}
	other, _ := protocol.NewKey()
	if r := call(t, s, "testAgent", map[string]any{"host": "127.0.0.1", "port": strconv.Itoa(agent.Port()), "key": other}); r["code"] != "UNAUTHORIZED" {
		t.Errorf("mauvaise clé : %v", r)
	}

	call(t, s, "updateSettings", map[string]any{"pollIntervalSeconds": 0, "confirmPowerActions": false})
	if st := s.State().Settings; st.PollIntervalSeconds != 1 || st.ConfirmPowerActions {
		t.Errorf("réglages : %+v", st)
	}
	call(t, s, "deleteDevice", map[string]any{"id": id})
	if len(s.State().Devices) != 0 {
		t.Error("suppression")
	}
}

func TestExportImportThroughService(t *testing.T) {
	s, platform := newService(t)
	form := model.NewForm()
	form.Name, form.MAC, form.Host = "PC", "aa:bb:cc:dd:ee:01", "10.0.0.2"
	key, _ := protocol.NewKey()
	form.AgentEnabled, form.AgentKey = true, key
	call(t, s, "saveDevice", map[string]any{"form": form})

	if _, err := s.Call("exportConfig", json.RawMessage(`{"withSecrets":true,"password":"court"}`)); err == nil {
		t.Error("mot de passe trop court accepté")
	}
	platform.cancel = true
	if r := call(t, s, "exportConfig", map[string]any{"withSecrets": false}); r["cancelled"] != true {
		t.Errorf("annulation : %v", r)
	}
	platform.cancel = false
	if r := call(t, s, "exportConfig", map[string]any{"withSecrets": true, "password": "mot de passe"}); r["ok"] != true || r["count"] != 1.0 {
		t.Fatalf("export : %v", r)
	}
	exported := string(platform.saved)

	call(t, s, "deleteDevice", map[string]any{"id": s.State().Devices[0].ID})
	if r := call(t, s, "importFile", map[string]any{"text": exported}); r["step"] != "password" {
		t.Fatalf("import : %v", r)
	}
	if r := call(t, s, "importPassword", map[string]any{"password": "mauvais !"}); r["wrongPassword"] != true {
		t.Fatalf("mauvais mot de passe : %v", r)
	}
	if r := call(t, s, "importPassword", map[string]any{"password": "mot de passe"}); r["step"] != "confirm" || r["count"] != 1.0 || r["missingKeys"] != false {
		t.Fatalf("mot de passe : %v", r)
	}
	if r := call(t, s, "importConfirm", map[string]any{"replace": true}); r["ok"] != true {
		t.Fatalf("confirmation : %v", r)
	}
	if got := call(t, s, "getDevice", map[string]any{"id": s.State().Devices[0].ID}); got["form"].(map[string]any)["agentKey"] != key {
		t.Error("clé non restaurée")
	}
	if _, err := s.Call("importFile", json.RawMessage(`{"text":"{\"hello\":1}"}`)); err == nil ||
		!strings.Contains(err.Error(), "n'est pas une sauvegarde") {
		t.Errorf("fichier étranger : %v", err)
	}
}

func TestOpenURLWhitelistAndPairing(t *testing.T) {
	s, platform := newService(t)
	call(t, s, "openUrl", map[string]any{"url": ReleasesURL})
	if _, err := s.Call("openUrl", json.RawMessage(`{"url":"https://example.com"}`)); err == nil {
		t.Error("adresse extérieure acceptée")
	}
	if len(platform.opened) != 1 {
		t.Errorf("ouvertures : %v", platform.opened)
	}
	key, _ := protocol.NewKey()
	r := call(t, s, "parsePairing", map[string]any{"text": "wolagent://pair?v=1&n=PC&h=192.168.1.9&k=" + key})
	if r["ok"] != true || r["info"].(map[string]any)["host"] != "192.168.1.9" {
		t.Errorf("appairage : %v", r)
	}
	if r := call(t, s, "parsePairing", map[string]any{"text": "abc"}); r["ok"] != false {
		t.Errorf("lien invalide : %v", r)
	}
	if _, err := s.Call("inconnue", nil); err == nil {
		t.Error("méthode inconnue acceptée")
	}
}

func TestHistoryRecordingAndAgentJournal(t *testing.T) {
	store, err := config.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	histDir := t.TempDir()
	histStore, err := history.NewStore(histDir)
	if err != nil {
		t.Fatal(err)
	}
	keyText, _ := protocol.NewKey()
	agent, err := agenttest.Start(model.DecodeAgentKey(keyText), agenttest.Normal)
	if err != nil {
		t.Fatal(err)
	}
	defer agent.Close()
	bootAt := time.Now().Add(-2 * time.Hour).Unix()
	agent.SetHistory(&protocol.History{From: bootAt - 3600, Events: []protocol.HistoryEvent{{T: bootAt, K: protocol.HistoryBoot}}})

	var reachable atomic.Bool
	lan := netstate.State{Interfaces: []netstate.Interface{{Name: "eth0", Transport: netstate.Ethernet,
		Addresses: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/8")}, HasGateway: true}}}
	s := New(Options{
		Version: "test", Store: store, Platform: &fakePlatform{}, History: histStore,
		Prober: status.ProberFunc(func(context.Context, model.Device) status.ProbeResult {
			return status.ProbeResult{Reachable: reachable.Load(), LatencyMs: 1, Method: status.MethodTCP}
		}),
		NetState: func() (netstate.State, error) { return lan, nil },
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)

	form := model.NewForm()
	form.Name, form.MAC, form.Host = "PC", "aa:bb:cc:dd:ee:01", "127.0.0.1"
	form.AgentEnabled, form.AgentPort, form.AgentKey = true, strconv.Itoa(agent.Port()), keyText
	id := call(t, s, "saveDevice", map[string]any{"form": form})["id"].(string)
	waitState(t, s, id, status.Offline)

	// Démarrage demandé d'ici : l'agent ne le voit pas, il lui sera signalé une fois le PC joignable.
	if r := call(t, s, "wake", map[string]any{"id": id}); r["ok"] != true {
		t.Fatalf("démarrage : %v", r)
	}
	wakeAt := time.Now().Unix()

	// Le PC s'allume : allumage constaté, puis lecture du journal de l'agent.
	reachable.Store(true)
	s.monitor.Refresh("")
	waitState(t, s, id, status.Online)
	kindsOf := func() []string {
		h := call(t, s, "getHistory", map[string]any{"id": id})
		var out []string
		for _, e := range h["events"].([]any) {
			ev := e.(map[string]any)
			out = append(out, ev["kind"].(string)+"/"+ev["source"].(string))
		}
		return out
	}
	deadline := time.Now().Add(3 * time.Second)
	for !slices.Contains(kindsOf(), "on/agent") && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	got := kindsOf()
	// Le constat de l'application (couvert par le journal de l'agent) est masqué au profit de l'heure exacte.
	if !slices.Contains(got, "on/agent") || slices.Contains(got, "on/app") {
		t.Fatalf("historique après allumage : %v", got)
	}
	if r := call(t, s, "getHistory", map[string]any{"id": id}); r["agent"] != AgentHistoryOK || r["hasAgent"] != true {
		t.Errorf("état du journal : %v", r)
	}
	// Démarrage signalé à l'agent (avec le nom de ce PC), affiché une seule fois.
	var reported *protocol.RequestBody
	for deadline := time.Now().Add(3 * time.Second); reported == nil && time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		for _, r := range agent.Requests() {
			if r.Cmd == protocol.CmdWakes {
				reported = &r
			}
		}
	}
	host, _ := os.Hostname()
	if reported == nil || len(reported.Wakes) != 1 || reported.Wakes[0] < wakeAt-5 || reported.Wakes[0] > wakeAt || reported.By != host {
		t.Fatalf("démarrage signalé : %+v", reported)
	}
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if slices.ContainsFunc(histStore.Load().Events, func(e history.Event) bool { return e.Kind == history.WakeSent && e.Source == history.Agent }) {
			break
		}
	}
	if n := strings.Count(strings.Join(kindsOf(), ","), "wake/"); n != 1 || !slices.Contains(kindsOf(), "wake/app") {
		t.Errorf("démarrage affiché %d fois : %v", n, kindsOf())
	}

	// Extinction demandée depuis l'application, puis PC éteint.
	if r := call(t, s, "power", map[string]any{"id": id, "action": "shutdown"}); r["ok"] != true {
		t.Fatalf("extinction : %v", r)
	}
	reachable.Store(false)
	deadline = time.Now().Add(5 * time.Second)
	for !slices.Contains(kindsOf(), "off/app") && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	got = kindsOf()
	if len(got) < 3 || got[0] != "off/app" || got[1] != "shutdown_req/app" {
		t.Fatalf("historique après extinction : %v", got)
	}
	if s.State().HistoryVersion == 0 {
		t.Error("version d'historique non incrémentée")
	}

	// Historique enregistré sur le disque, puis supprimé avec le PC.
	if len(histStore.Load().Events) == 0 {
		t.Error("historique non enregistré")
	}
	call(t, s, "deleteDevice", map[string]any{"id": id})
	if h := call(t, s, "getHistory", nil); len(h["events"].([]any)) != 0 {
		t.Errorf("historique d'un PC supprimé : %v", h)
	}
	call(t, s, "clearHistory", nil)
}

func TestLatencyInStateAndLive(t *testing.T) {
	store, err := config.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var probes atomic.Int64
	lan := netstate.State{Interfaces: []netstate.Interface{{Name: "eth0", Transport: netstate.Ethernet,
		Addresses: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/8")}, HasGateway: true}}}
	s := New(Options{
		Version: "test", Store: store, Platform: &fakePlatform{},
		Prober: status.ProberFunc(func(context.Context, model.Device) status.ProbeResult {
			n := probes.Add(1)
			if n == 2 {
				return status.ProbeResult{} // une sonde sans réponse
			}
			return status.ProbeResult{Reachable: true, LatencyMs: n, Method: status.MethodTCP}
		}),
		NetState: func() (netstate.State, error) { return lan, nil },
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)

	form := model.NewForm()
	form.Name, form.MAC, form.Host = "PC", "aa:bb:cc:dd:ee:01", "127.0.0.1"
	id := call(t, s, "saveDevice", map[string]any{"form": form})["id"].(string)
	waitState(t, s, id, status.Online)

	// Affiché en détail : une mesure par seconde (l'intervalle normal est de 3 s).
	call(t, s, "setLive", map[string]any{"id": id})
	time.Sleep(2500 * time.Millisecond)
	samples := call(t, s, "getState", nil)["devices"].([]any)[0].(map[string]any)["latency"].([]any)
	if len(samples) < 3 {
		t.Fatalf("mesures en direct attendues : %v", samples)
	}
	first, second := samples[0].(map[string]any), samples[1].(map[string]any)
	if first["ms"] != float64(1) || second["ms"] != nil || first["t"].(float64) <= 0 {
		t.Errorf("mesures : %v", samples)
	}
	call(t, s, "setLive", map[string]any{"id": ""})
}

package status

import (
	"context"
	"net"
	"net/netip"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol"
	"github.com/slaynaw/wakeonlan/desktop/internal/agentclient"
	"github.com/slaynaw/wakeonlan/desktop/internal/agentclient/agenttest"
	"github.com/slaynaw/wakeonlan/desktop/internal/model"
)

// --- Machine à états (mêmes scénarios que StatusTrackerTest côté Android) ---

type fixture struct {
	now     int64
	tracker *Tracker
}

func newFixture() *fixture {
	f := &fixture{now: 1_000_000}
	f.tracker = NewTracker(func() int64 { return f.now })
	f.tracker.WakeTimeoutMs = 60_000
	f.tracker.ShutdownTimeoutMs = 30_000
	return f
}

func (f *fixture) tick(ms int64) { f.now += ms }

var (
	up   = ProbeResult{Reachable: true, LatencyMs: 3, Method: MethodTCP}
	down = ProbeResult{}
)

func expect(t *testing.T, got DeviceStatus, want PowerState, msg string) {
	t.Helper()
	if got.State != want {
		t.Fatalf("%s : état %s, attendu %s", msg, got.State, want)
	}
}

func TestFirstFailureShowsOffline(t *testing.T) {
	f := newFixture()
	expect(t, f.tracker.Status(), Unknown, "initial")
	expect(t, f.tracker.OnProbe(down), Offline, "premier échec")
}

func TestHysteresis(t *testing.T) {
	f := newFixture()
	f.tracker.OnProbe(up)
	f.tick(3000)
	expect(t, f.tracker.OnProbe(down), Online, "un seul paquet perdu ne suffit pas")
	f.tick(3000)
	expect(t, f.tracker.OnProbe(up), Online, "retour")
	f.tick(3000)
	f.tracker.OnProbe(down)
	f.tick(3000)
	s := f.tracker.OnProbe(down)
	expect(t, s, Offline, "deux échecs")
	if s.Since != f.now || *s.LastSeen != f.now-6000 {
		t.Errorf("horodatages : %+v", s)
	}
}

func TestWakeSucceeds(t *testing.T) {
	f := newFixture()
	f.tracker.OnProbe(down)
	f.tick(3000)
	expect(t, f.tracker.OnWakeSent(), Waking, "réveil")
	for range 5 {
		f.tick(3000)
		expect(t, f.tracker.OnProbe(down), Waking, "attente")
	}
	f.tick(3000)
	s := f.tracker.OnProbe(up)
	expect(t, s, Online, "réveillé")
	if s.ActionStartedAt != nil || s.Notice != "" {
		t.Errorf("action non terminée : %+v", s)
	}
}

func TestWakeTimeout(t *testing.T) {
	f := newFixture()
	f.tracker.OnWakeSent()
	f.tick(61_000)
	s := f.tracker.OnProbe(down)
	expect(t, s, Offline, "délai dépassé")
	if s.Notice != WakeTimeout || f.tracker.ClearNotice().Notice != "" {
		t.Errorf("notification : %+v", s)
	}
}

func TestShutdownConfirmedThenTimeout(t *testing.T) {
	f := newFixture()
	f.tracker.OnProbe(up)
	f.tracker.OnShutdownSent()
	f.tick(3000)
	expect(t, f.tracker.OnProbe(up), ShuttingDown, "encore là")
	f.tick(3000)
	expect(t, f.tracker.OnProbe(down), ShuttingDown, "premier échec")
	f.tick(3000)
	expect(t, f.tracker.OnProbe(down), Offline, "éteint")

	f.tracker.OnProbe(up)
	f.tracker.OnShutdownSent()
	f.tick(31_000)
	s := f.tracker.OnProbe(up)
	expect(t, s, Online, "toujours allumé")
	if s.Notice != ShutdownTimeout {
		t.Errorf("notification : %+v", s)
	}
}

func TestRestartWaitsForDisappearance(t *testing.T) {
	f := newFixture()
	f.tracker.OnProbe(up)
	f.tracker.OnRestartSent()
	f.tick(3000)
	expect(t, f.tracker.OnProbe(up), Restarting, "pas encore éteint")
	f.tick(3000)
	expect(t, f.tracker.OnProbe(down), Restarting, "éteint")
	f.tick(3000)
	expect(t, f.tracker.OnProbe(down), Restarting, "toujours éteint")
	f.tick(3000)
	expect(t, f.tracker.OnProbe(up), Online, "revenu")
}

func TestUnknownRatherThanFalseOffline(t *testing.T) {
	f := newFixture()
	f.tracker.OnProbe(up)
	s := f.tracker.OnUnavailable(NoNetwork)
	expect(t, s, Unknown, "sans réseau")
	if s.UnknownReason != NoNetwork || s.LastSeen == nil || s.LatencyMs != nil {
		t.Errorf("état inconnu : %+v", s)
	}
}

// --- Sondes ---

func device(ports []int, host string, agent *model.AgentSettings) model.Device {
	mac, _ := model.ParseMAC("AA:BB:CC:DD:EE:01")
	return model.Device{ID: "x", Name: "x", MAC: mac, Host: host, WolPort: 9, ProbePorts: ports, Agent: agent}
}

func noPing(context.Context, netip.Addr, time.Duration) bool { return false }

func testProber() *HostProber {
	p := NewHostProber()
	p.Ping = noPing
	return p
}

func TestOpenOrRefusedPortMeansOnline(t *testing.T) {
	l, _ := net.Listen("tcp4", "127.0.0.1:0")
	defer l.Close()
	r := testProber().Probe(context.Background(), device([]int{l.Addr().(*net.TCPAddr).Port}, "127.0.0.1", nil))
	if !r.Reachable || r.Method != MethodTCP {
		t.Errorf("port ouvert : %+v", r)
	}
	closed, _ := net.Listen("tcp4", "127.0.0.1:0")
	port := closed.Addr().(*net.TCPAddr).Port
	closed.Close()
	if r := testProber().Probe(context.Background(), device([]int{port}, "127.0.0.1", nil)); !r.Reachable {
		t.Errorf("connexion refusée = machine allumée : %+v", r)
	}
}

func TestSilentMachineIsOfflineWithinTimeout(t *testing.T) {
	p := testProber()
	p.Timeout = 400 * time.Millisecond
	silent := func(ctx context.Context, _ netip.Addr, _ int, _ time.Duration) bool {
		<-ctx.Done()
		return false
	}
	p.TCP = silent
	p.Ping = func(ctx context.Context, _ netip.Addr, _ time.Duration) bool { <-ctx.Done(); return false }
	start := time.Now()
	r := p.Probe(context.Background(), device([]int{9, 22}, "127.0.0.1", nil))
	if r.Reachable || time.Since(start) > 2*time.Second {
		t.Errorf("machine muette : %+v en %v", r, time.Since(start))
	}
}

func TestAgentHasPriority(t *testing.T) {
	keyText, _ := protocol.NewKey()
	s, err := agenttest.Start(model.DecodeAgentKey(keyText), agenttest.Normal)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r := testProber().Probe(context.Background(), device([]int{s.Port()}, "127.0.0.1", &model.AgentSettings{Port: s.Port(), Key: keyText}))
	if !r.Reachable || r.Method != MethodAgent || r.Agent == nil || r.Agent.Hostname != "PC-TEST" {
		t.Errorf("agent : %+v", r)
	}
	// Mauvaise clé : la machine est allumée (elle a répondu) et l'erreur est remontée.
	other, _ := protocol.NewKey()
	r = testProber().Probe(context.Background(), device(nil, "127.0.0.1", &model.AgentSettings{Port: s.Port(), Key: other}))
	if !r.Reachable || r.AgentError != agentclient.Unauthorized {
		t.Errorf("mauvaise clé : %+v", r)
	}
}

func TestPingAloneAndNoHost(t *testing.T) {
	p := testProber()
	p.Ping = func(context.Context, netip.Addr, time.Duration) bool { return true }
	if r := p.Probe(context.Background(), device(nil, "127.0.0.1", nil)); !r.Reachable || r.Method != MethodPing {
		t.Errorf("ping : %+v", r)
	}
	if r := testProber().Probe(context.Background(), device([]int{1}, "", nil)); r.Reachable {
		t.Errorf("sans adresse : %+v", r)
	}
}

// --- Surveillance ---

type fakeProber struct {
	reachable atomic.Bool
	probes    atomic.Int32
}

func (f *fakeProber) Probe(context.Context, model.Device) ProbeResult {
	f.probes.Add(1)
	if f.reachable.Load() {
		return up
	}
	return down
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("délai dépassé : %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestMonitor(t *testing.T) {
	prober := &fakeProber{}
	var changes atomic.Int32
	m := NewMonitor(prober, nil, func() { changes.Add(1) })
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); m.Run(ctx) }()
	defer func() { cancel(); wg.Wait() }()

	cfg := model.NewConfig()
	cfg.Settings.PollIntervalSeconds = 60
	cfg.Devices = []model.Device{device(nil, "10.0.0.2", nil)}
	state := func() PowerState { return m.Statuses()["x"].State }

	m.Update(cfg, Availability{CanProbe: true}, true)
	waitFor(t, "première sonde", func() bool { return state() == Offline })

	prober.reachable.Store(true)
	m.OnWakeSent("x")
	waitFor(t, "rafraîchissement immédiat après action", func() bool { return state() == Online })

	m.Update(cfg, Availability{CanProbe: false, Reason: NoNetwork}, true)
	waitFor(t, "inconnu sans réseau", func() bool { return state() == Unknown })

	// Fenêtre réduite : plus aucune sonde.
	m.Update(cfg, Availability{CanProbe: true}, false)
	time.Sleep(100 * time.Millisecond)
	before := prober.probes.Load()
	m.Refresh("")
	time.Sleep(100 * time.Millisecond)
	if prober.probes.Load() != before {
		t.Error("sonde alors que la fenêtre est réduite")
	}

	m.Update(model.NewConfig(), Availability{CanProbe: true}, true)
	waitFor(t, "PC supprimé", func() bool { return len(m.Statuses()) == 0 })
	if changes.Load() == 0 {
		t.Error("aucune notification de changement")
	}
}

func TestMonitorFastDuringTransition(t *testing.T) {
	prober := &fakeProber{}
	m := NewMonitor(prober, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Run(ctx)
	cfg := model.NewConfig()
	cfg.Settings.PollIntervalSeconds = 60
	cfg.Devices = []model.Device{device(nil, "10.0.0.2", nil)}
	m.Update(cfg, Availability{CanProbe: true}, true)
	waitFor(t, "première sonde", func() bool { return prober.probes.Load() >= 1 })
	m.OnWakeSent("x")
	start := prober.probes.Load()
	time.Sleep(2500 * time.Millisecond)
	if n := prober.probes.Load() - start; n < 3 {
		t.Errorf("une sonde par seconde pendant le réveil attendue (%d)", n)
	}
}

func TestMonitorLatencyAndLive(t *testing.T) {
	prober := &fakeProber{}
	m := NewMonitor(prober, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Run(ctx)
	cfg := model.NewConfig()
	cfg.Settings.PollIntervalSeconds = 60
	cfg.Devices = []model.Device{device(nil, "10.0.0.2", nil)}
	m.Update(cfg, Availability{CanProbe: true}, true)
	waitFor(t, "première sonde", func() bool { return len(m.Latency("x", 0)) == 1 })
	if s := m.Latency("x", 0)[0]; s.Ms != nil {
		t.Errorf("sonde sans réponse : latence nulle attendue (%v)", *s.Ms)
	}

	prober.reachable.Store(true)
	m.SetLive("x")
	waitFor(t, "sonde immédiate à l'ouverture du détail", func() bool { return len(m.Latency("x", 0)) == 2 })
	if s := m.Latency("x", 0)[1]; s.Ms == nil || *s.Ms != 3 {
		t.Errorf("latence mesurée attendue : %+v", s)
	}
	start := prober.probes.Load()
	time.Sleep(2500 * time.Millisecond)
	if n := prober.probes.Load() - start; n < 2 {
		t.Errorf("une sonde par seconde pour le PC affiché en détail attendue (%d)", n)
	}
	m.SetLive("")
	time.Sleep(1200 * time.Millisecond)
	before := prober.probes.Load()
	time.Sleep(1500 * time.Millisecond)
	if n := prober.probes.Load() - before; n != 0 {
		t.Errorf("retour à l'intervalle normal attendu (%d sondes)", n)
	}
	if got := m.Latency("x", m.Latency("x", 0)[1].T); len(got) == 0 || got[0].T < m.Latency("x", 0)[1].T {
		t.Error("filtre par date")
	}

	m.Update(model.NewConfig(), Availability{CanProbe: true}, true)
	waitFor(t, "mesures du PC supprimé oubliées", func() bool { return len(m.Latency("x", 0)) == 0 })
}

func TestAppendLatency(t *testing.T) {
	var samples []LatencySample
	for i := range 601 {
		ms := int64(i)
		samples = AppendLatency(samples, LatencySample{T: int64(i) * 1000, Ms: &ms})
	}
	if len(samples) != 301 || samples[0].T != 300_000 {
		t.Fatalf("fenêtre de 5 minutes : %d relevés, premier à %d", len(samples), samples[0].T)
	}
	for i := range 500 {
		samples = AppendLatency(samples, LatencySample{T: 600_000 + int64(i)})
	}
	if len(samples) != LatencyMaxSamples {
		t.Errorf("nombre borné attendu : %d", len(samples))
	}
}

func TestAppendTemp(t *testing.T) {
	v := func(x float64) *float64 { return &x }
	var samples []TempSample
	// Sondé chaque seconde, mesures renouvelées par l'agent toutes les 5 s : seuls les changements
	// (et une répétition toutes les 10 s) sont gardés.
	for i := range 30 {
		cpu := 50 + float64(i/5)
		if i >= 20 {
			cpu = 53
		}
		samples = AppendTemp(samples, TempSample{T: int64(i) * 1000, CPU: v(cpu), GPU: v(60)})
	}
	var times []int64
	for _, s := range samples {
		times = append(times, s.T/1000)
	}
	if want := []int64{0, 5, 10, 15, 25}; !reflect.DeepEqual(times, want) {
		t.Fatalf("relevés gardés à %v s, attendu %v", times, want)
	}
	// Une valeur qui disparaît (ou apparaît) est un changement.
	samples = AppendTemp(samples, TempSample{T: 26_000, CPU: v(53)})
	if len(samples) != 6 || samples[5].GPU != nil {
		t.Fatalf("%+v", samples[len(samples)-1])
	}
	// Fenêtre de 5 minutes, nombre borné.
	samples = AppendTemp(samples, TempSample{T: 26_000 + TempWindow.Milliseconds(), CPU: v(40)})
	if len(samples) != 2 || samples[0].T != 26_000 {
		t.Fatalf("fenêtre : %+v", samples)
	}
	for i := range 1000 {
		samples = AppendTemp(samples, TempSample{T: 400_000 + int64(i), CPU: v(float64(i))})
	}
	if len(samples) != TempMaxSamples {
		t.Errorf("nombre borné attendu : %d", len(samples))
	}
}

func TestTempOf(t *testing.T) {
	v := func(x float64) *float64 { return &x }
	if _, ok := TempOf(1, nil); ok {
		t.Error("sans agent")
	}
	if _, ok := TempOf(1, &agentclient.Status{Temperatures: &protocol.Temperatures{CPULoad: v(5)}}); ok {
		t.Error("utilisation seule : pas de relevé de température")
	}
	// Puce graphique intégrée : sa température (celle de la puce) est gardée, marquée.
	s, ok := TempOf(7, &agentclient.Status{Temperatures: &protocol.Temperatures{CPU: v(55), GPU: v(55), GPUShared: true}})
	if !ok || s.T != 7 || *s.CPU != 55 || s.GPU == nil || *s.GPU != 55 || !s.Shared {
		t.Errorf("puce intégrée : %+v", s)
	}
	// Passage de la puce intégrée à la carte dédiée (même valeur) : nouveau relevé.
	samples := AppendTemp([]TempSample{s}, TempSample{T: 8, CPU: v(55), GPU: v(55)})
	if len(samples) != 2 || samples[1].Shared {
		t.Errorf("changement de carte ignoré : %+v", samples)
	}
	s, ok = TempOf(8, &agentclient.Status{Temperatures: &protocol.Temperatures{GPU: v(70)}})
	if !ok || s.CPU != nil || *s.GPU != 70 {
		t.Errorf("carte graphique seule : %+v", s)
	}
}

func TestMonitorTemps(t *testing.T) {
	cpu := 61.0
	prober := ProberFunc(func(context.Context, model.Device) ProbeResult {
		return ProbeResult{Reachable: true, LatencyMs: 2, Method: MethodAgent,
			Agent: &agentclient.Status{Temperatures: &protocol.Temperatures{CPU: &cpu}}}
	})
	m := NewMonitor(prober, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Run(ctx)
	cfg := model.NewConfig()
	cfg.Devices = []model.Device{device(nil, "10.0.0.2", nil)}
	m.Update(cfg, Availability{CanProbe: true}, true)
	waitFor(t, "relevé de températures", func() bool { return len(m.Temps("x", 0)) == 1 })
	if s := m.Temps("x", 0)[0]; *s.CPU != 61 || s.GPU != nil {
		t.Errorf("%+v", s)
	}
	m.SetLive("x")
	if m.Live() != "x" {
		t.Error("PC affiché en détail")
	}
	m.Update(model.NewConfig(), Availability{CanProbe: true}, true)
	waitFor(t, "relevés du PC supprimé oubliés", func() bool { return len(m.Temps("x", 0)) == 0 })
}

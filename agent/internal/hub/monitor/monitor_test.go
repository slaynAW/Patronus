package monitor

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/hubprotocol"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/agents"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/archive"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/fleet"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/hubtest"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/store"
	"github.com/slaynaw/wakeonlan/agent/internal/identity"
	"github.com/slaynaw/wakeonlan/agent/protocol"
	"github.com/slaynaw/wakeonlan/agent/protocol2"
)

type memStorage struct{ data []byte }

func (m *memStorage) Load() ([]byte, error) {
	if m.data == nil {
		return nil, os.ErrNotExist
	}
	return m.data, nil
}

func (m *memStorage) Save(data []byte) error { m.data = append([]byte(nil), data...); return nil }

type env struct {
	hub     *identity.Identity
	fleet   *fleet.Fleet
	dialer  *agents.Dialer
	archive *archive.Archive
	mon     *Monitor
}

func start(t *testing.T) *env {
	t.Helper()
	return startWith(t, append(agents.TargetNetworks(), netip.MustParsePrefix("127.0.0.0/8")))
}

// startWith démarre la surveillance avec les réseaux permis networks.
func startWith(t *testing.T, networks []netip.Prefix) *env {
	t.Helper()
	hub, _, err := identity.LoadOrCreate(t.TempDir(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	list, err := fleet.Open(&memStorage{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	dialer := &agents.Dialer{Identity: hub, Networks: networks}
	st, err := store.Open(t.TempDir(), store.Key{3})
	if err != nil {
		t.Fatal(err)
	}
	arch := archive.New(st, time.Now)
	m := New(list, dialer, arch, log.New(io.Discard, "", 0))
	// Sous Windows, une connexion refusée sur l'adresse de bouclage n'échoue qu'après 2 s environ.
	m.agentEvery, m.probeEvery, m.timeout = 50*time.Millisecond, 50*time.Millisecond, 4*time.Second
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go m.Run(ctx)
	return &env{hub: hub, fleet: list, dialer: dialer, archive: arch, mon: m}
}

// entrust confie au hub l'agent a, comme hub.pcs.add.
func (e *env) entrust(t *testing.T, a *hubtest.Agent) fleet.PC {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	role, err := e.dialer.Pair(ctx, "127.0.0.1", a.Port, a.ID.Pin, a.Invite(t, protocol2.RoleWake, e.hub.Pin), "Hub NAS")
	if err != nil {
		t.Fatal(err)
	}
	pin := a.ID.Pin
	p, err := e.fleet.Add(fleet.PC{Name: "PC Bureau", Host: "127.0.0.1", Port: a.Port, Agent: &pin, Role: role})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func (e *env) probe(t *testing.T, name string, probe hubprotocol.Probe) fleet.PC {
	t.Helper()
	p, err := e.fleet.Add(fleet.PC{Name: name, Host: "127.0.0.1", Probe: &probe})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// wait attend que l'état du PC id vérifie ok.
func (e *env) wait(t *testing.T, id string, what string, ok func(hubprotocol.PCState) bool) hubprotocol.PCState {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	var last hubprotocol.PCState
	for time.Now().Before(deadline) {
		for _, s := range e.mon.States() {
			if s.ID == id {
				if last = s; ok(s) {
					return s
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s : dernier état %+v", what, last)
	return last
}

func TestAgentStates(t *testing.T) {
	e := start(t)
	a := hubtest.StartAgent(t)
	pc := e.entrust(t, a)
	online := e.wait(t, pc.ID, "en ligne", func(s hubprotocol.PCState) bool {
		return s.State == hubprotocol.StateOnline && s.Agent != nil && s.Agent.Version == hubtest.Version
	})
	if online.Problem != "" || online.Since == 0 || online.Checked == 0 || online.Agent.Hostname == "" {
		t.Fatalf("état en ligne : %+v", online)
	}
	// La session est gardée : plusieurs relevés, toujours en ligne.
	first := online.Checked
	e.wait(t, pc.ID, "relevés suivants", func(s hubprotocol.PCState) bool {
		return s.State == hubprotocol.StateOnline && s.Checked >= first+1
	})
	// Agent arrêté : le port refuse la connexion, le PC est allumé mais l'agent non.
	a.Stop()
	down := e.wait(t, pc.ID, "agent arrêté", func(s hubprotocol.PCState) bool { return s.State == hubprotocol.StateAgentDown })
	if down.Agent != nil || down.Since < online.Since {
		t.Fatalf("état agent arrêté : %+v", down)
	}
}

func TestRevokedHub(t *testing.T) {
	e := start(t)
	a := hubtest.StartAgent(t)
	pc := e.entrust(t, a)
	e.wait(t, pc.ID, "en ligne", func(s hubprotocol.PCState) bool { return s.State == hubprotocol.StateOnline && s.Agent != nil })
	if _, err := a.Registry.Remove(e.hub.Pin, true); err != nil {
		t.Fatal(err)
	}
	revoked := e.wait(t, pc.ID, "hub retiré", func(s hubprotocol.PCState) bool { return s.Problem == hubprotocol.ProblemRevoked })
	// Le refus espace les relevés : plus de relevé dans la seconde qui suit.
	time.Sleep(1200 * time.Millisecond)
	for _, s := range e.mon.States() {
		if s.ID == pc.ID && s.Checked != revoked.Checked {
			t.Fatalf("relevé malgré le refus : %+v", s)
		}
	}
}

func TestPinMismatch(t *testing.T) {
	e := start(t)
	a, other := hubtest.StartAgent(t), hubtest.StartAgent(t)
	wrong := other.ID.Pin
	p, err := e.fleet.Add(fleet.PC{Name: "PC", Host: "127.0.0.1", Port: a.Port, Agent: &wrong, Role: protocol2.RoleWake})
	if err != nil {
		t.Fatal(err)
	}
	e.wait(t, p.ID, "empreinte différente", func(s hubprotocol.PCState) bool { return s.Problem == hubprotocol.ProblemPinMismatch })
}

func TestProbes(t *testing.T) {
	e := start(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	closed, _ := net.Listen("tcp", "127.0.0.1:0")
	closedPort := closed.Addr().(*net.TCPAddr).Port
	_ = closed.Close()
	var heads atomic.Int32
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead && r.Header.Get("Cookie") == "" {
			heads.Add(1)
		}
		http.Redirect(w, r, "https://example.com/", http.StatusFound) // jamais suivi
	}))
	t.Cleanup(web.Close)
	secure := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) }))
	t.Cleanup(secure.Close)
	port := func(raw string) int {
		u, _ := url.Parse(raw)
		n, _ := strconv.Atoi(u.Port())
		return n
	}

	tcpOpen := e.probe(t, "Box", hubprotocol.Probe{Kind: "tcp", Port: ln.Addr().(*net.TCPAddr).Port})
	tcpClosed := e.probe(t, "Imprimante", hubprotocol.Probe{Kind: "tcp", Port: closedPort})
	httpUp := e.probe(t, "Caméra", hubprotocol.Probe{Kind: "http", Port: port(web.URL)})
	httpsUp := e.probe(t, "NAS", hubprotocol.Probe{Kind: "https", Port: port(secure.URL)})
	httpDown := e.probe(t, "Page arrêtée", hubprotocol.Probe{Kind: "http", Port: closedPort})

	for pc, want := range map[fleet.PC]string{
		tcpOpen: hubprotocol.StateOnline, tcpClosed: hubprotocol.StateOffline, httpUp: hubprotocol.StateOnline,
		httpsUp: hubprotocol.StateOnline, httpDown: hubprotocol.StateOffline,
	} {
		s := e.wait(t, pc.ID, pc.Name, func(s hubprotocol.PCState) bool { return s.State == want })
		if s.Agent != nil || s.Problem != "" || want == hubprotocol.StateOnline && s.Latency < 0 {
			t.Errorf("%s : %+v", pc.Name, s)
		}
	}
	if heads.Load() == 0 {
		t.Error("la page web n'a reçu aucune requête HEAD sans cookie")
	}

	// Un PC retiré de la liste disparaît des états.
	if _, err := e.fleet.Remove(tcpOpen.ID); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		gone := true
		for _, s := range e.mon.States() {
			if s.ID == tcpOpen.ID {
				gone = false
			}
		}
		if gone && len(e.mon.States()) == 4 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("PC retiré toujours suivi : %+v", e.mon.States())
}

// TestPrivateProbe : une sonde vers une adresse hors des réseaux permis ne part jamais.
func TestPrivateProbe(t *testing.T) {
	e := startWith(t, agents.TargetNetworks()) // sans l'adresse de bouclage, comme en service
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	accepted := make(chan struct{}, 1)
	go func() {
		if c, err := ln.Accept(); err == nil {
			_ = c.Close()
			accepted <- struct{}{}
		}
	}()
	p := e.probe(t, "Local", hubprotocol.Probe{Kind: "tcp", Port: ln.Addr().(*net.TCPAddr).Port})
	e.wait(t, p.ID, "hors réseau permis", func(s hubprotocol.PCState) bool { return s.State == hubprotocol.StateOffline })
	select {
	case <-accepted:
		t.Fatal("connexion ouverte vers l'adresse de bouclage")
	default:
	}
}

// TestArchive : le journal de l'agent et ses jours de mesures passés sont recopiés dans l'archive du hub ;
// le jour en cours ne l'est pas encore.
func TestArchive(t *testing.T) {
	e := start(t)
	a := hubtest.StartAgent(t)
	boot := time.Now().Add(-48 * time.Hour).Unix()
	if err := a.History.Add(protocol.HistoryEvent{T: boot, K: protocol.HistoryBoot}); err != nil {
		t.Fatal(err)
	}
	today := time.Now().UTC().Format("2006-01-02")
	past := []string{time.Now().UTC().AddDate(0, 0, -3).Format("2006-01-02"), time.Now().UTC().AddDate(0, 0, -2).Format("2006-01-02")}
	for _, day := range append(past, today) {
		a.Metrics.Set(day, []protocol.MetricsRow{{T: boot, N: 6}})
	}
	pc := e.entrust(t, a)
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		events, _, _, _ := e.archive.Events(pc.ID, 0)
		days, _ := e.archive.Days(pc.ID)
		if len(days) == len(past) && hasBoot(events, boot) {
			if days[0] != past[0] || days[1] != past[1] {
				t.Fatalf("jours gardés : %v", days)
			}
			rows, err := e.archive.Day(pc.ID, past[0])
			if err != nil || len(rows) != 1 || rows[0].N != 6 {
				t.Fatalf("mesures gardées : %+v, %v", rows, err)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	events, _, _, _ := e.archive.Events(pc.ID, 0)
	days, _ := e.archive.Days(pc.ID)
	t.Fatalf("archive incomplète : %+v, %v", events, days)
}

func hasBoot(events []protocol.HistoryEvent, t int64) bool {
	for _, e := range events {
		if e.T == t && e.K == protocol.HistoryBoot {
			return true
		}
	}
	return false
}

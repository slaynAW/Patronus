// Package hubtest aide les tests du hub : un vrai agent wolagent/2 sur l'adresse de bouclage, des
// identités d'appareils et des invitations, comme les créerait une application administrateur du PC.
package hubtest

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"io"
	"log"
	"net"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/internal/config"
	"github.com/slaynaw/wakeonlan/agent/internal/devices"
	"github.com/slaynaw/wakeonlan/agent/internal/history"
	"github.com/slaynaw/wakeonlan/agent/internal/identity"
	"github.com/slaynaw/wakeonlan/agent/internal/power"
	"github.com/slaynaw/wakeonlan/agent/internal/server"
	"github.com/slaynaw/wakeonlan/agent/protocol"
	"github.com/slaynaw/wakeonlan/agent/protocol2"
	"github.com/slaynaw/wakeonlan/agent/protocol2/client"
)

// Version est la version annoncée par l'agent de test.
const Version = "1.13.0"

type noPower struct{}

func (noPower) Do(power.Action, bool) error { return nil }

// Agent est un vrai agent de test.
type Agent struct {
	Port     int
	ID       *identity.Identity
	Registry *devices.Registry
	History  *history.Log // journal du PC (commande history)
	Metrics  *Metrics     // mesures enregistrées (commande metrics)
	stop     context.CancelFunc
}

// Metrics sont les mesures enregistrées de l'agent de test, jour par jour.
type Metrics struct {
	mu   sync.Mutex
	days map[string][]protocol.MetricsRow
}

// Set enregistre les mesures d'un jour.
func (m *Metrics) Set(day string, rows []protocol.MetricsRow) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.days[day] = rows
}

// Days et Day répondent comme l'enregistreur de l'agent.
func (m *Metrics) Days() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for d := range m.days {
		out = append(out, d)
	}
	slices.Sort(out)
	return out
}

func (m *Metrics) Day(day string) ([]protocol.MetricsRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.days[day], nil
}

// StartAgent démarre un agent sur 127.0.0.1, arrêté à la fin du test.
func StartAgent(t testing.TB) *Agent {
	t.Helper()
	dir := t.TempDir()
	cfg, err := config.New("PC Bureau", 1)
	if err != nil {
		t.Fatal(err)
	}
	id, _, err := identity.LoadOrCreate(dir, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	reg, err := devices.Open(devices.Path(dir), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := server.New(cfg, noPower{}, Version, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	srv.EnableV2(id, reg)
	journal, err := history.Open(filepath.Join(dir, "history.json"), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	srv.SetHistory(journal)
	metrics := &Metrics{days: map[string][]protocol.MetricsRow{}}
	srv.SetMetrics(metrics)
	raw, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ln := &trackingListener{Listener: raw}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = srv.Serve(ctx, ln) }()
	return &Agent{
		Port: raw.Addr().(*net.TCPAddr).Port, ID: id, Registry: reg, History: journal, Metrics: metrics,
		stop: func() { cancel(); ln.closeAll() },
	}
}

// Stop arrête l'agent comme l'arrêt de son service : connexions ouvertes coupées, puis port fermé.
func (a *Agent) Stop() { a.stop() }

// trackingListener garde les connexions acceptées, pour les couper à l'arrêt.
type trackingListener struct {
	net.Listener
	mu    sync.Mutex
	conns []net.Conn
}

func (l *trackingListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err == nil {
		l.mu.Lock()
		l.conns = append(l.conns, c)
		l.mu.Unlock()
	}
	return c, err
}

func (l *trackingListener) closeAll() {
	_ = l.Listener.Close()
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, c := range l.conns {
		_ = c.Close()
	}
}

// Invite crée sur l'agent une invitation réservée à l'identité only, comme une application administrateur
// du PC ; elle renvoie le code.
func (a *Agent) Invite(t testing.TB, role protocol2.Role, only protocol2.Pin) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pin := a.ID.Pin
	sess, err := client.Dial(ctx, net.JoinHostPort("127.0.0.1", strconv.Itoa(a.Port)), client.Options{Identity: a.ID.Cert, Agent: &pin})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	resp, err := sess.Do(ctx, protocol2.Request{Cmd: protocol2.CmdInvite, Role: role.String(), Device: only.String()})
	if err != nil || !resp.OK {
		t.Fatalf("invitation de l'agent : %+v, %v", resp, err)
	}
	return resp.Invite.Code
}

// NewDevice crée l'identité d'un appareil.
func NewDevice(t testing.TB) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := protocol2.NewCertificate(key, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// PinOf renvoie l'empreinte d'une identité.
func PinOf(t testing.TB, c tls.Certificate) protocol2.Pin {
	t.Helper()
	pin, err := protocol2.PinFromCertificate(c.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	return pin
}

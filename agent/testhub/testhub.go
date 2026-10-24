// Package testhub démarre, pour les tests, un vrai hub patronus-hub/1 (docs/HUB.md) et de vrais agents
// wolagent/2 sur l'adresse de bouclage : les applications s'éprouvent contre le code servi sur le NAS,
// sans rien simuler. Seule différence avec le service : le hub y joint aussi les agents sur l'adresse
// de bouclage. Jamais utilisé hors des tests.
package testhub

import (
	"context"
	"log"
	"net"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/hubprotocol"
	"github.com/slaynaw/wakeonlan/agent/internal/devices"
	"github.com/slaynaw/wakeonlan/agent/internal/hub"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/agents"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/hubtest"
	"github.com/slaynaw/wakeonlan/agent/protocol2"
)

// Name et Version sont le nom et la version annoncés par le hub de test.
const (
	Name    = "Hub de test"
	Version = "2.0.0-test"
)

// Hub est un vrai hub, arrêté à la fin du test.
type Hub struct {
	Port int
	Pin  protocol2.Pin
	hub  *hub.Hub
	log  *logBuffer
}

// Start démarre un hub sur 127.0.0.1, avec ses données dans un dossier temporaire.
func Start(t testing.TB) *Hub {
	t.Helper()
	dir := t.TempDir()
	buf := &logBuffer{}
	h, err := hub.Open(hub.Config{
		Data: dir + "/data", Keys: dir + "/keys", Name: Name, Version: Version, Logger: log.New(buf, "", 0),
		Networks: append(agents.TargetNetworks(), netip.MustParsePrefix("127.0.0.0/8")),
	})
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = h.Run(ctx, ln)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return &Hub{Port: ln.Addr().(*net.TCPAddr).Port, Pin: h.Identity.Pin, hub: h, log: buf}
}

// Link crée une invitation à usage unique, comme « patronus-hub pair » sur le NAS, et renvoie son lien
// d'appairage (adresse 127.0.0.1).
func (h *Hub) Link(t testing.TB, role protocol2.Role) string {
	t.Helper()
	inv, code, err := h.hub.Registry.CreateInvite(role, "", protocol2.DefaultInviteTTL*time.Second, nil, devices.ByLocal)
	if err != nil {
		t.Fatal(err)
	}
	link := protocol2.Link{Name: Name, Host: "127.0.0.1", Port: h.Port, Agent: h.Pin, Code: code, Role: role, Expires: inv.Expires}
	return link.Format(hubprotocol.LinkFormat)
}

// Device est un appareil autorisé par le hub, tel que son registre le garde.
type Device = devices.Device

// Devices renvoie les appareils autorisés par le hub.
func (h *Hub) Devices() []Device { return h.hub.Registry.Devices() }

// Log renvoie le journal du hub.
func (h *Hub) Log() string { return h.log.String() }

// Agent est un vrai agent wolagent/2 sur 127.0.0.1 : ses champs et méthodes sont ceux de
// l'agent des tests du hub (identité, registre, journal, mesures, invitations).
type Agent = hubtest.Agent

// StartAgent démarre un agent sur 127.0.0.1, arrêté à la fin du test.
func StartAgent(t testing.TB) *Agent { return hubtest.StartAgent(t) }

type logBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (l *logBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *logBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

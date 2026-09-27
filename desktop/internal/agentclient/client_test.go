package agentclient

import (
	"context"
	"net"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol"
	"github.com/slaynaw/wakeonlan/desktop/internal/agentclient/agenttest"
	"github.com/slaynaw/wakeonlan/desktop/internal/model"
)

var (
	keyText, _ = protocol.NewKey()
	keyBytes   = model.DecodeAgentKey(keyText)
)

func start(t *testing.T, b agenttest.Behavior) (*agenttest.Server, model.AgentSettings) {
	t.Helper()
	s, err := agenttest.Start(keyBytes, b)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s, model.AgentSettings{Port: s.Port(), Key: keyText}
}

func TestStatusAndPower(t *testing.T) {
	s, settings := start(t, agenttest.Normal)
	c := New()
	st, err := c.Status(context.Background(), "127.0.0.1", settings)
	if err != nil || st.Hostname != "PC-TEST" || st.Uptime != 3600 || st.OS != "windows" {
		t.Fatalf("statut : %+v %v", st, err)
	}
	msg, err := c.Power(context.Background(), "127.0.0.1", settings, Shutdown, 0, true)
	if err != nil || msg != "OK" {
		t.Fatalf("extinction : %q %v", msg, err)
	}
	if !reflect.DeepEqual(s.Commands(), []string{"status", "shutdown"}) {
		t.Errorf("commandes : %v", s.Commands())
	}
}

func TestWrongKey(t *testing.T) {
	s, settings := start(t, agenttest.Normal)
	other, _ := protocol.NewKey()
	settings.Key = other
	_, err := New().Status(context.Background(), "127.0.0.1", settings)
	if CodeOf(err) != Unauthorized || !HostAnswered(err) || len(s.Commands()) != 0 {
		t.Errorf("mauvaise clé : %v", err)
	}
}

func TestFailures(t *testing.T) {
	cases := map[agenttest.Behavior]Code{
		agenttest.BadResponseMAC: Protocol,
		agenttest.Reject:         Rejected,
		agenttest.RateLimited:    RateLimited,
		agenttest.WrongProto:     Protocol,
	}
	for behavior, want := range cases {
		_, settings := start(t, behavior)
		_, err := New().Power(context.Background(), "127.0.0.1", settings, Reboot, 0, false)
		if CodeOf(err) != want || !HostAnswered(err) {
			t.Errorf("comportement %d : %v (attendu %s)", behavior, err, want)
		}
	}
}

func TestClosedPortSilentAgentAndNoKey(t *testing.T) {
	l, _ := net.Listen("tcp4", "127.0.0.1:0")
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	_, err := New().Status(context.Background(), "127.0.0.1", model.AgentSettings{Port: port, Key: keyText})
	if CodeOf(err) != Refused || !HostAnswered(err) {
		t.Errorf("port fermé : %v", err)
	}

	_, settings := start(t, agenttest.Hang)
	c := &Client{ConnectTimeout: time.Second, ReadTimeout: 300 * time.Millisecond}
	_, err = c.Status(context.Background(), "127.0.0.1", settings)
	if CodeOf(err) != Protocol || !HostAnswered(err) {
		t.Errorf("agent muet : %v", err)
	}

	_, err = New().Status(context.Background(), "127.0.0.1", model.AgentSettings{Port: 9770, Key: ""})
	if CodeOf(err) != NoKey || HostAnswered(err) {
		t.Errorf("sans clé : %v", err)
	}
	_, err = New().Status(context.Background(), "hote-inexistant.invalid", model.AgentSettings{Port: 9770, Key: keyText})
	if c := CodeOf(err); c != UnknownHost && c != Unreachable {
		t.Errorf("hôte inconnu : %v", err)
	}
}

func TestCancellationIsImmediate(t *testing.T) {
	_, settings := start(t, agenttest.Hang)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	begin := time.Now()
	_, err := New().Status(ctx, "127.0.0.1", settings)
	if err == nil || time.Since(begin) > time.Second {
		t.Errorf("annulation lente : %v en %v", err, time.Since(begin))
	}
}

// Test de bout en bout contre le VRAI agent Go, lancé en mode simulation par la CI.
func TestEndToEndWithRealAgent(t *testing.T) {
	port, err := strconv.Atoi(os.Getenv("WOL_E2E_PORT"))
	if err != nil {
		t.Skip("WOL_E2E_PORT non défini (test lancé par la CI)")
	}
	settings := model.AgentSettings{Port: port, Key: os.Getenv("WOL_E2E_KEY")}
	st, err := New().Status(context.Background(), "127.0.0.1", settings)
	if err != nil || st.Hostname == "" {
		t.Fatalf("statut : %+v %v", st, err)
	}
	if _, err := New().Power(context.Background(), "127.0.0.1", settings, Shutdown, 0, false); err != nil {
		t.Fatalf("extinction simulée : %v", err)
	}
	other, _ := protocol.NewKey()
	if _, err := New().Status(context.Background(), "127.0.0.1", model.AgentSettings{Port: port, Key: other}); CodeOf(err) != Unauthorized {
		t.Errorf("mauvaise clé : %v", err)
	}
}

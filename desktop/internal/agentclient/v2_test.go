package agentclient

import (
	"context"
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol"
	"github.com/slaynaw/wakeonlan/agent/protocol2"
	"github.com/slaynaw/wakeonlan/agent/protocol2/client"
	"github.com/slaynaw/wakeonlan/desktop/internal/agentclient/agenttest"
	"github.com/slaynaw/wakeonlan/desktop/internal/identity"
	"github.com/slaynaw/wakeonlan/desktop/internal/model"
)

// newClient renvoie un client doté d'une identité wolagent/2 neuve (celle d'une installation).
func newClient(t *testing.T) (*Client, protocol2.Pin) {
	t.Helper()
	store := identity.NewStore(t.TempDir(), nil, nil)
	id, err := store.Get()
	if err != nil {
		t.Fatal(err)
	}
	c := New()
	c.By = "PC-WINDOWS"
	c.Identity = func() (tls.Certificate, error) { return id.Cert, nil }
	return c, id.Pin
}

func startV2(t *testing.T, b agenttest.Behavior) (*agenttest.Server, model.AgentSettings, protocol2.Pin) {
	t.Helper()
	s, settings := start(t, b)
	return s, settings, s.EnableV2()
}

func TestV2PinAnnouncedBySignedV1Response(t *testing.T) {
	_, settings, agent := startV2(t, agenttest.Normal)
	c, _ := newClient(t)
	st, err := c.Status(context.Background(), "127.0.0.1", settings)
	if err != nil || st.Proto != protocol.Proto || st.Pin != agent.String() ||
		!slices.Equal(st.Protocols, []string{protocol.Proto, protocol2.ALPN}) {
		t.Fatalf("statut wolagent/1 : %+v %v", st, err)
	}
}

func TestV2MigrationThenCommands(t *testing.T) {
	s, settings, agent := startV2(t, agenttest.Normal)
	s.SetHistory(&protocol.History{Events: []protocol.HistoryEvent{}})
	c, device := newClient(t)
	settings.Pin = agent.String()
	ctx := context.Background()

	// Empreinte connue, cet ordinateur pas encore enregistré : refus, sans retour à wolagent/1.
	if _, err := c.Status(ctx, "127.0.0.1", settings); CodeOf(err) != Unregistered || !HostAnswered(err) {
		t.Fatalf("avant la migration : %v", err)
	}
	m, err := c.Migrate(ctx, "127.0.0.1", settings)
	if err != nil || m.Role != "control" {
		t.Fatalf("migration : %+v %v", m, err)
	}
	if role, name := s.Device(device); role != "control" || name != "PC-WINDOWS" {
		t.Fatalf("registre du faux agent : %q %q", role, name)
	}
	// Une seconde migration est refusée : l'appareil est déjà connu.
	if _, err := c.Migrate(ctx, "127.0.0.1", settings); CodeOf(err) != Rejected || ReasonOf(err) != protocol2.CodeAlreadyPaired {
		t.Errorf("seconde migration : %v", err)
	}

	// Ensuite, la clé n'est plus nécessaire.
	settings.Key = ""
	st, err := c.Status(ctx, "127.0.0.1", settings)
	if err != nil || st.Proto != protocol2.ALPN || st.Role != "control" || st.Hostname != "PC-TEST" {
		t.Fatalf("statut wolagent/2 : %+v %v", st, err)
	}
	if _, err := c.Power(ctx, "127.0.0.1", settings, Sleep, 0, false); err != nil {
		t.Fatalf("mise en veille : %v", err)
	}
	h, err := c.ReportWakes(ctx, "127.0.0.1", settings, []int64{1_700_000_000})
	if err != nil || len(h.Events) != 1 || h.Events[0].B != "PC-WINDOWS" {
		t.Fatalf("démarrages signalés : %+v %v", h, err)
	}
	reqs := s.Requests()
	if last := reqs[len(reqs)-2]; last.Cmd != "sleep" || last.Delay == nil || *last.Delay != 0 || last.Force == nil || *last.Force {
		t.Errorf("requête de mise en veille : %+v", last)
	}
}

func TestV2PinMismatchSendsNothing(t *testing.T) {
	s, settings, _ := startV2(t, agenttest.Normal)
	c, device := newClient(t)
	s.Register(device, "admin")
	settings.Pin = protocol2.PinOf([]byte("autre agent")).String()
	_, err := c.Status(context.Background(), "127.0.0.1", settings)
	if CodeOf(err) != PinMismatch || !HostAnswered(err) {
		t.Fatalf("empreinte différente : %v", err)
	}
	if _, err := c.Migrate(context.Background(), "127.0.0.1", settings); CodeOf(err) != PinMismatch {
		t.Fatalf("migration vers une autre empreinte : %v", err)
	}
	if len(s.Commands()) != 0 {
		t.Errorf("commandes reçues malgré l'empreinte différente : %v", s.Commands())
	}
}

func TestV2NeverFallsBackToV1(t *testing.T) {
	// Agent qui ne sert pas wolagent/2 (ou intermédiaire qui le bloque) : échec, jamais wolagent/1.
	s, settings := start(t, agenttest.Normal)
	c, _ := newClient(t)
	c.ConnectTimeout, c.ReadTimeout = 300*time.Millisecond, 300*time.Millisecond
	settings.Pin = protocol2.PinOf([]byte("agent")).String()
	begin := time.Now()
	_, err := c.Status(context.Background(), "127.0.0.1", settings)
	if CodeOf(err) != Protocol || time.Since(begin) > 3*time.Second {
		t.Fatalf("agent sans wolagent/2 : %v en %v", err, time.Since(begin))
	}
	if len(s.Commands()) != 0 {
		t.Errorf("repli sur wolagent/1 : %v", s.Commands())
	}
}

func TestV2MigrationWithWrongKey(t *testing.T) {
	s, settings, agent := startV2(t, agenttest.Normal)
	c, device := newClient(t)
	other, _ := protocol.NewKey()
	settings.Key, settings.Pin = other, agent.String()
	_, err := c.Migrate(context.Background(), "127.0.0.1", settings)
	if CodeOf(err) != Protocol || !strings.Contains(err.Error(), "preuve de l'agent invalide") {
		t.Fatalf("mauvaise clé : %v", err)
	}
	if role, _ := s.Device(device); role != "" {
		t.Errorf("appareil enregistré malgré la preuve fausse : %q", role)
	}
}

func TestV2RolesAndRefusals(t *testing.T) {
	s, settings, agent := startV2(t, agenttest.Normal)
	c, device := newClient(t)
	settings.Key, settings.Pin = "", agent.String()
	s.Register(device, "view")
	if _, err := c.Status(context.Background(), "127.0.0.1", settings); err != nil {
		t.Fatalf("consultation : %v", err)
	}
	_, err := c.Power(context.Background(), "127.0.0.1", settings, Shutdown, 0, false)
	if CodeOf(err) != Rejected || ReasonOf(err) != protocol2.CodeInsufficientRole {
		t.Fatalf("extinction avec le rôle view : %v", err)
	}
	// Révoqué : plus reconnu.
	s.Register(device, "")
	if _, err := c.Status(context.Background(), "127.0.0.1", settings); CodeOf(err) != Unregistered {
		t.Fatalf("après révocation : %v", err)
	}

	limited, settings2, agent2 := startV2(t, agenttest.RateLimited)
	_ = limited
	settings2.Pin = agent2.String()
	if _, err := c.Status(context.Background(), "127.0.0.1", settings2); CodeOf(err) != RateLimited {
		t.Errorf("adresse bloquée : %v", err)
	}

	rejecting, settings3, agent3 := startV2(t, agenttest.Reject)
	rejecting.Register(device, "admin")
	settings3.Pin = agent3.String()
	if _, err := c.Status(context.Background(), "127.0.0.1", settings3); CodeOf(err) != Rejected || ReasonOf(err) != "forbidden" {
		t.Errorf("commande désactivée : %v", err)
	}
}

// TestV2Pairing : appairage avec un code (§7.3) ; le code n'est envoyé qu'à l'agent qui présente
// l'empreinte du lien, et ne sert qu'une fois.
func TestV2Pairing(t *testing.T) {
	s, settings, agent := startV2(t, agenttest.Normal)
	ctx := context.Background()
	c, device := newClient(t)
	settings.Key, settings.Pin = "", agent.String()
	code := s.Invite("wake")
	p, err := c.Pair(ctx, "127.0.0.1", settings, strings.ToLower(protocol2.FormatCode(code)))
	if err != nil || p.Role != "wake" || p.Already {
		t.Fatalf("appairage : %+v %v", p, err)
	}
	if role, name := s.Device(device); role != "wake" || name != "PC-WINDOWS" {
		t.Errorf("enregistré : %q %q", role, name)
	}
	if st, err := c.Status(ctx, "127.0.0.1", settings); err != nil || st.Role != "wake" || st.Proto != protocol2.ALPN {
		t.Fatalf("statut : %+v %v", st, err)
	}
	if _, err := c.Power(ctx, "127.0.0.1", settings, Shutdown, 0, false); ReasonOf(err) != protocol2.CodeInsufficientRole {
		t.Errorf("extinction avec le rôle wake : %v", err)
	}

	// Déjà enregistré : réponse already_paired, le second code reste valable.
	second := s.Invite("admin")
	if again, err := c.Pair(ctx, "127.0.0.1", settings, second); err != nil || !again.Already || again.Role != "wake" {
		t.Errorf("déjà enregistré : %+v %v", again, err)
	}
	// Code déjà utilisé : refusé, la machine a bien répondu.
	other, otherPin := newClient(t)
	if _, err := other.Pair(ctx, "127.0.0.1", settings, code); CodeOf(err) != PairingFailed || !HostAnswered(err) {
		t.Errorf("code déjà utilisé : %v", err)
	}
	// Autre empreinte à cette adresse : rien n'est envoyé, le code reste valable.
	wrong := settings
	wrong.Pin = protocol2.PinOf([]byte("autre agent")).String()
	if _, err := other.Pair(ctx, "127.0.0.1", wrong, second); CodeOf(err) != PinMismatch {
		t.Errorf("empreinte différente : %v", err)
	}
	if p, err := other.Pair(ctx, "127.0.0.1", settings, second); err != nil || p.Role != "admin" {
		t.Errorf("code resté valable : %+v %v", p, err)
	}
	if role, _ := s.Device(otherPin); role != "admin" {
		t.Errorf("second appareil : %q", role)
	}
	// Code mal écrit ou empreinte absente : refus sans connexion.
	attempts := s.Attempts()
	if _, err := other.Pair(ctx, "127.0.0.1", settings, "7KQ2-9MX"); CodeOf(err) != PairingFailed {
		t.Errorf("code mal écrit : %v", err)
	}
	if _, err := other.Pair(ctx, "127.0.0.1", model.AgentSettings{Port: settings.Port}, s.Invite("view")); CodeOf(err) != PairingFailed {
		t.Errorf("sans empreinte : %v", err)
	}
	if s.Attempts() != attempts {
		t.Error("connexion ouverte malgré un code ou une empreinte invalide")
	}
}

func TestV2WithoutIdentity(t *testing.T) {
	_, settings, agent := startV2(t, agenttest.Normal)
	settings.Pin = agent.String()
	_, err := New().Status(context.Background(), "127.0.0.1", settings)
	if CodeOf(err) != NoIdentity || HostAnswered(err) {
		t.Fatalf("sans identité : %v", err)
	}
	if _, err := New().Status(context.Background(), "127.0.0.1", model.AgentSettings{Port: settings.Port}); CodeOf(err) != NoKey {
		t.Fatalf("ni clé ni empreinte : %v", err)
	}
}

// wolagent/2 de bout en bout contre le VRAI agent Go, lancé en simulation par la CI : empreinte
// annoncée en wolagent/1, migration, commandes, journal, empreinte différente.
func TestEndToEndV2WithRealAgent(t *testing.T) {
	port, err := strconv.Atoi(os.Getenv("WOL_E2E_PORT"))
	if err != nil {
		t.Skip("WOL_E2E_PORT non défini (test lancé par la CI)")
	}
	ctx := context.Background()
	c, device := newClient(t)
	settings := model.AgentSettings{Port: port, Key: os.Getenv("WOL_E2E_KEY")}
	st, err := c.Status(ctx, "127.0.0.1", settings)
	if err != nil || st.Pin == "" || !slices.Contains(st.Protocols, protocol2.ALPN) {
		t.Fatalf("annonce de wolagent/2 : %+v %v", st, err)
	}
	settings.Pin = st.Pin
	if _, err := c.Status(ctx, "127.0.0.1", settings); CodeOf(err) != Unregistered {
		t.Fatalf("avant la migration : %v", err)
	}
	m, err := c.Migrate(ctx, "127.0.0.1", settings)
	if err != nil || m.Role != "control" {
		t.Fatalf("migration : %+v %v", m, err)
	}
	settings.Key = ""
	st2, err := c.Status(ctx, "127.0.0.1", settings)
	if err != nil || st2.Proto != protocol2.ALPN || st2.Role != "control" || st2.Hostname != st.Hostname {
		t.Fatalf("statut wolagent/2 : %+v %v", st2, err)
	}
	if _, err := c.Power(ctx, "127.0.0.1", settings, Sleep, 0, false); err != nil {
		t.Fatalf("mise en veille simulée : %v", err)
	}
	h, err := c.History(ctx, "127.0.0.1", settings)
	if err != nil {
		t.Fatal(err)
	}
	found := slices.ContainsFunc(h.Events, func(e protocol.HistoryEvent) bool {
		return e.K == protocol.HistoryCommand && e.A == "sleep" && e.B == c.By && e.I == device.Short()
	})
	if !found {
		t.Errorf("commande wolagent/2 absente du journal : %+v", h.Events)
	}
	settings.Pin = protocol2.PinOf([]byte("autre agent")).String()
	if _, err := c.Status(ctx, "127.0.0.1", settings); CodeOf(err) != PinMismatch {
		t.Errorf("empreinte différente : %v", err)
	}
}

// agentAdmin ouvre une session avec le vrai agent sous son identité (fichier identity.key, en PEM hors
// Windows), depuis l'adresse de bouclage : celle de « wol-agent pair » sur le PC, rôle admin.
func agentAdmin(t *testing.T, path string, port int) (*client.Session, protocol2.Pin) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		t.Fatal("identité de l'agent illisible")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	key, ok := parsed.(*ecdsa.PrivateKey)
	if err != nil || !ok {
		t.Fatalf("identité de l'agent illisible : %v", err)
	}
	der, err := protocol2.NewCertificate(key, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	pin, _ := protocol2.PinFromCertificate(der)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s, err := client.Dial(ctx, net.JoinHostPort("127.0.0.1", strconv.Itoa(port)),
		client.Options{Identity: tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, Agent: &pin})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, pin
}

// Appairage de bout en bout contre le VRAI agent : invitation créée comme le fait « wol-agent pair »,
// code envoyé par l'application, rôle de l'invitation appliqué, retrait par le PC.
func TestEndToEndPairingWithRealAgent(t *testing.T) {
	port, err := strconv.Atoi(os.Getenv("WOL_E2E_PORT"))
	path := os.Getenv("WOL_E2E_IDENTITY")
	if err != nil || path == "" {
		t.Skip("WOL_E2E_PORT ou WOL_E2E_IDENTITY non défini (test lancé par la CI)")
	}
	ctx := context.Background()
	admin, agent := agentAdmin(t, path, port)
	invite, err := admin.Do(ctx, protocol2.Request{Cmd: protocol2.CmdInvite, Role: "wake", TTL: 120})
	if err != nil || !invite.OK || invite.Invite == nil || invite.Invite.Agent != agent.String() {
		t.Fatalf("invitation : %+v %v", invite, err)
	}
	c, device := newClient(t)
	settings := model.AgentSettings{Port: port, Pin: agent.String()}
	p, err := c.Pair(ctx, "127.0.0.1", settings, protocol2.FormatCode(invite.Invite.Code))
	if err != nil || p.Role != "wake" || p.Already {
		t.Fatalf("appairage : %+v %v", p, err)
	}
	st, err := c.Status(ctx, "127.0.0.1", settings)
	if err != nil || st.Proto != protocol2.ALPN || st.Role != "wake" {
		t.Fatalf("statut : %+v %v", st, err)
	}
	if _, err := c.Power(ctx, "127.0.0.1", settings, Shutdown, 0, false); ReasonOf(err) != protocol2.CodeInsufficientRole {
		t.Errorf("extinction avec le rôle wake : %v", err)
	}
	if again, err := c.Pair(ctx, "127.0.0.1", settings, invite.Invite.Code); err != nil || !again.Already {
		t.Errorf("déjà enregistré : %+v %v", again, err)
	}
	revoked, err := admin.Do(ctx, protocol2.Request{Cmd: protocol2.CmdDevicesRevoke, Device: device.String()})
	if err != nil || !revoked.OK {
		t.Fatalf("retrait : %+v %v", revoked, err)
	}
	if _, err := c.Status(ctx, "127.0.0.1", settings); CodeOf(err) != Unregistered {
		t.Errorf("après le retrait : %v", err)
	}
}

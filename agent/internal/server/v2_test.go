package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/internal/config"
	"github.com/slaynaw/wakeonlan/agent/internal/devices"
	"github.com/slaynaw/wakeonlan/agent/internal/guard"
	"github.com/slaynaw/wakeonlan/agent/internal/history"
	"github.com/slaynaw/wakeonlan/agent/internal/identity"
	"github.com/slaynaw/wakeonlan/agent/internal/power"
	"github.com/slaynaw/wakeonlan/agent/protocol"
	"github.com/slaynaw/wakeonlan/agent/protocol2"
	"github.com/slaynaw/wakeonlan/agent/protocol2/client"
)

// v2Env est un agent de test qui sert wolagent/1 et wolagent/2.
type v2Env struct {
	addr    string
	cfg     *config.Config
	id      *identity.Identity
	reg     *devices.Registry
	journal *history.Log
	rec     *recorder
	srv     *Server
}

func startV2(t *testing.T, mutate func(*config.Config), setup func(*Server)) *v2Env {
	t.Helper()
	minActionDelay = 10 * time.Millisecond
	dir := t.TempDir()
	env := &v2Env{}
	var err error
	if env.id, _, err = identity.LoadOrCreate(dir, time.Now()); err != nil {
		t.Fatal(err)
	}
	if env.reg, err = devices.Open(devices.Path(dir), time.Now); err != nil {
		t.Fatal(err)
	}
	if env.journal, err = history.Open(filepath.Join(dir, "history.json"), time.Now); err != nil {
		t.Fatal(err)
	}
	env.cfg, env.rec, env.addr = startServer(t, mutate, func(s *Server) {
		s.EnableV2(env.id, env.reg)
		s.SetHistory(env.journal)
		env.srv = s
		if setup != nil {
			setup(s)
		}
	})
	return env
}

// lenient retire le blocage des adresses, pour enchaîner les refus dans un test.
func lenient(s *Server) { s.limiter = guard.NewLimiter(1000, time.Minute, time.Minute) }

func newIdentity(t *testing.T) tls.Certificate {
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

func pinOf(t *testing.T, c tls.Certificate) protocol2.Pin {
	t.Helper()
	pin, err := protocol2.PinFromCertificate(c.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	return pin
}

func (e *v2Env) enroll(t *testing.T, c tls.Certificate, role protocol2.Role, name string) protocol2.Pin {
	t.Helper()
	d, err := e.reg.Add(devices.Device{Pin: pinOf(t, c), Name: name, Role: role, Via: devices.ViaLocal, By: devices.ByLocal})
	if err != nil {
		t.Fatal(err)
	}
	return d.Pin
}

func ctx5(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func (e *v2Env) dialAs(t *testing.T, c tls.Certificate, agent *protocol2.Pin) (*client.Session, error) {
	t.Helper()
	s, err := client.Dial(ctx5(t), e.addr, client.Options{Identity: c, Agent: agent})
	if err == nil {
		t.Cleanup(func() { _ = s.Close() })
	}
	return s, err
}

func (e *v2Env) dial(t *testing.T, c tls.Certificate) *client.Session {
	t.Helper()
	s, err := e.dialAs(t, c, &e.id.Pin)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// local ouvre la session de wol-agent sur le PC : l'identité de l'agent, depuis l'adresse de bouclage.
func (e *v2Env) local(t *testing.T) *client.Session { return e.dial(t, e.id.Cert) }

func do(t *testing.T, s *client.Session, req protocol2.Request) protocol2.Response {
	t.Helper()
	resp, err := s.Do(ctx5(t), req)
	if err != nil {
		t.Fatalf("%s : %v", req.Cmd, err)
	}
	return resp
}

// closed vérifie que l'agent a fermé la session.
func closed(t *testing.T, s *client.Session) {
	t.Helper()
	if _, err := s.Do(ctx5(t), protocol2.Request{Cmd: "status"}); err == nil {
		t.Error("session encore ouverte")
	}
}

func TestV2StatusAndProtocols(t *testing.T) {
	env := startV2(t, nil, nil)
	phone := newIdentity(t)
	env.enroll(t, phone, protocol2.RoleView, "Pixel 8")
	s := env.dial(t, phone)
	for range 2 { // plusieurs requêtes par connexion
		resp := do(t, s, protocol2.Request{Cmd: "status"})
		if !resp.OK || resp.Role != "view" || resp.Hostname == "" || resp.Version != "test" ||
			!slices.Equal(resp.Protocols, []string{config.ProtocolV1, config.ProtocolV2}) {
			t.Fatalf("état : %+v", resp)
		}
	}
	if s.AgentPin() != env.id.Pin {
		t.Error("empreinte de l'agent")
	}
	// wolagent/1 inchangé, et annonce wolagent/2 et l'empreinte de l'agent dans ses réponses signées.
	body := decodeBody(t, first(exchange(t, env.addr, env.cfg.Key, `{"cmd":"status"}`)))
	if !body.OK || !slices.Equal(body.Protocols, []string{config.ProtocolV1, config.ProtocolV2}) ||
		body.Pin != env.id.Pin.String() {
		t.Fatalf("wolagent/1 : %+v", body)
	}
}

func TestV1OnlyAgentAnnouncesNoPin(t *testing.T) {
	cfg, _, addr := startServer(t, nil, nil)
	body := decodeBody(t, first(exchange(t, addr, cfg.Key, `{"cmd":"status"}`)))
	if !body.OK || !slices.Equal(body.Protocols, []string{config.ProtocolV1}) || body.Pin != "" {
		t.Fatalf("wolagent/1 seul : %+v", body)
	}
}

func TestV2OnlyAgent(t *testing.T) {
	env := startV2(t, func(c *config.Config) { c.Protocols = []string{config.ProtocolV2}; c.Key = "" }, nil)
	// Un client wolagent/1 lit un salut qu'il ne connaît pas ; sa requête reste sans réponse.
	conn, err := net.Dial("tcp", env.addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	hello := readLine(t, conn)
	if hello != `{"proto":"wolagent/2"}` {
		t.Fatalf("salut %q", hello)
	}
	_, _ = conn.Write([]byte(`{"cnonce":"x","body":"{}","mac":"x"}` + "\n"))
	if n, err := conn.Read(make([]byte, 1)); n != 0 || err == nil {
		t.Error("requête wolagent/1 acceptée")
	}
	phone := newIdentity(t)
	env.enroll(t, phone, protocol2.RoleControl, "Pixel 8")
	if resp := do(t, env.dial(t, phone), protocol2.Request{Cmd: "status"}); !resp.OK ||
		!slices.Equal(resp.Protocols, []string{config.ProtocolV2}) {
		t.Fatalf("wolagent/2 seul : %+v", resp)
	}
}

func readLine(t *testing.T, conn net.Conn) string {
	t.Helper()
	var line []byte
	b := make([]byte, 1)
	for {
		if _, err := io.ReadFull(conn, b); err != nil {
			t.Fatal(err)
		}
		if b[0] == '\n' {
			return string(line)
		}
		line = append(line, b[0])
	}
}

// rawTLS ouvre TLS à la main (après le salut) avec la configuration de client donnée.
func rawTLS(t *testing.T, addr string, cfg *tls.Config) (*tls.Conn, error) {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	readLine(t, conn)
	tc := tls.Client(conn, cfg)
	return tc, tc.Handshake()
}

func baseClient(certs ...tls.Certificate) *tls.Config {
	return &tls.Config{MinVersion: tls.VersionTLS13, NextProtos: []string{protocol2.ALPN}, InsecureSkipVerify: true, Certificates: certs}
}

// refusedTLS vérifie qu'une connexion TLS a été refusée : pendant la poignée de main, ou à la première
// lecture (en TLS 1.3, le client peut finir la sienne avant que l'agent refuse son certificat).
func refusedTLS(t *testing.T, name string, tc *tls.Conn, err error) {
	t.Helper()
	if err != nil {
		return
	}
	_ = protocol2.WriteFrame(tc, []byte(`{"id":1,"cmd":"status"}`), protocol2.MaxRequestBytes)
	if _, err := protocol2.ReadFrame(tc, protocol2.MaxResponseBytes); err == nil {
		t.Errorf("%s : connexion acceptée", name)
	}
}

func selfSigned(t *testing.T, pub, priv any) tls.Certificate {
	t.Helper()
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "wolagent"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, priv)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: priv}
}

func TestV2TLSRefusals(t *testing.T) {
	env := startV2(t, nil, lenient)
	phone := newIdentity(t)
	env.enroll(t, phone, protocol2.RoleAdmin, "Pixel 8")

	old := baseClient(phone)
	old.MinVersion, old.MaxVersion = tls.VersionTLS12, tls.VersionTLS12
	tc, err := rawTLS(t, env.addr, old)
	refusedTLS(t, "TLS 1.2", tc, err)

	noALPN := baseClient(phone)
	noALPN.NextProtos = nil
	tc, err = rawTLS(t, env.addr, noALPN)
	refusedTLS(t, "sans ALPN", tc, err)

	otherALPN := baseClient(phone)
	otherALPN.NextProtos = []string{"h2"}
	tc, err = rawTLS(t, env.addr, otherALPN)
	refusedTLS(t, "autre ALPN", tc, err)

	tc, err = rawTLS(t, env.addr, baseClient())
	refusedTLS(t, "sans certificat", tc, err)

	rsaKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	tc, err = rawTLS(t, env.addr, baseClient(selfSigned(t, rsaKey.Public(), rsaKey)))
	refusedTLS(t, "clé RSA", tc, err)

	p384, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	tc, err = rawTLS(t, env.addr, baseClient(selfSigned(t, p384.Public(), p384)))
	refusedTLS(t, "clé P-384", tc, err)

	chain := phone
	chain.Certificate = append(slices.Clone(phone.Certificate), newIdentity(t).Certificate[0])
	tc, err = rawTLS(t, env.addr, baseClient(chain))
	refusedTLS(t, "deux certificats", tc, err)

	// Le bon appareil passe toujours.
	if resp := do(t, env.dial(t, phone), protocol2.Request{Cmd: "status"}); !resp.OK {
		t.Fatalf("appareil autorisé : %+v", resp)
	}
}

func TestV2RefusalsBlockTheAddress(t *testing.T) {
	env := startV2(t, nil, nil)
	for range 5 {
		tc, err := rawTLS(t, env.addr, baseClient())
		refusedTLS(t, "sans certificat", tc, err)
	}
	if _, err := env.dialAs(t, newIdentity(t), &env.id.Pin); !errors.Is(err, client.ErrRateLimited) {
		t.Fatalf("adresse non bloquée : %v", err)
	}
}

func TestV2ClientRejectsWrongAgent(t *testing.T) {
	env := startV2(t, nil, nil)
	phone := newIdentity(t)
	env.enroll(t, phone, protocol2.RoleView, "Pixel 8")
	wrong := protocol2.PinOf([]byte("autre agent"))
	for range 6 {
		if _, err := env.dialAs(t, phone, &wrong); !errors.Is(err, client.ErrPinMismatch) {
			t.Fatalf("empreinte différente : %v", err)
		}
	}
	// Le refus vient du client : l'agent ne le compte pas contre l'adresse.
	if resp := do(t, env.dial(t, phone), protocol2.Request{Cmd: "status"}); !resp.OK {
		t.Fatalf("appareil autorisé : %+v", resp)
	}
}

func TestV2UnknownIdentity(t *testing.T) {
	env := startV2(t, nil, nil)
	for i := range 5 {
		s := env.dial(t, newIdentity(t))
		resp := do(t, s, protocol2.Request{Cmd: "status"})
		if resp.Code != protocol2.CodeUnknownDevice || resp.OK || resp.Hostname != "" || resp.Version != "" || resp.Role != "" {
			t.Fatalf("identité inconnue %d : %+v", i, resp)
		}
		closed(t, s)
	}
	if _, err := env.dialAs(t, newIdentity(t), &env.id.Pin); !errors.Is(err, client.ErrRateLimited) {
		t.Fatalf("échecs non comptés : %v", err)
	}
}

func TestV2Roles(t *testing.T) {
	env := startV2(t, nil, nil)
	sessions := map[protocol2.Role]*client.Session{}
	for _, role := range protocol2.Roles {
		c := newIdentity(t)
		env.enroll(t, c, role, "appareil "+role.String())
		sessions[role] = env.dial(t, c)
	}
	now := time.Now().Unix()
	for _, c := range []struct {
		role protocol2.Role
		req  protocol2.Request
		code string
	}{
		{protocol2.RoleView, protocol2.Request{Cmd: "history"}, protocol2.CodeOK},
		{protocol2.RoleView, protocol2.Request{Cmd: "wakes", Wakes: []int64{now}}, protocol2.CodeInsufficientRole},
		{protocol2.RoleView, protocol2.Request{Cmd: "sleep"}, protocol2.CodeInsufficientRole},
		{protocol2.RoleWake, protocol2.Request{Cmd: "wakes", Wakes: []int64{now}}, protocol2.CodeOK},
		{protocol2.RoleWake, protocol2.Request{Cmd: "reboot"}, protocol2.CodeInsufficientRole},
		{protocol2.RoleControl, protocol2.Request{Cmd: protocol2.CmdDevices}, protocol2.CodeInsufficientRole},
		{protocol2.RoleControl, protocol2.Request{Cmd: protocol2.CmdInvite}, protocol2.CodeInsufficientRole},
		{protocol2.RoleAdmin, protocol2.Request{Cmd: "format"}, protocol2.CodeUnsupported},
		{protocol2.RoleAdmin, protocol2.Request{Cmd: protocol2.CmdPair, Code: "7KQ29MXP"}, protocol2.CodeAlreadyPaired},
	} {
		resp := do(t, sessions[c.role], c.req)
		if resp.Code != c.code || resp.Role != c.role.String() {
			t.Errorf("%s / %s : %s (%s)", c.role, c.req.Cmd, resp.Code, resp.Message)
		}
	}
	resp := do(t, sessions[protocol2.RoleControl], protocol2.Request{Cmd: "shutdown", Delay: new(int)})
	if !resp.OK {
		t.Fatalf("extinction : %+v", resp)
	}
	select {
	case <-env.rec.done:
	case <-time.After(2 * time.Second):
		t.Fatal("extinction non exécutée")
	}
	if env.rec.actions[0] != power.Shutdown {
		t.Fatalf("action %v", env.rec.actions)
	}
	// Journal : la commande est attribuée à l'appareil authentifié.
	events := env.journal.Snapshot().Events
	i := slices.IndexFunc(events, func(e protocol.HistoryEvent) bool { return e.K == protocol.HistoryCommand })
	if i < 0 || events[i].B != "appareil control" || events[i].I == "" || events[i].C != "127.0.0.1" {
		t.Errorf("journal : %+v", events)
	}
	list := do(t, sessions[protocol2.RoleAdmin], protocol2.Request{Cmd: protocol2.CmdDevices})
	if len(list.Devices) != 4 || !slices.ContainsFunc(list.Devices, func(d protocol2.DeviceInfo) bool {
		return d.Self && d.Role == "admin" && d.Name == "appareil admin"
	}) {
		t.Errorf("liste des appareils : %+v", list.Devices)
	}
}

func TestV2Whitelist(t *testing.T) {
	env := startV2(t, func(c *config.Config) { c.Commands = []string{"status"} }, nil)
	c := newIdentity(t)
	env.enroll(t, c, protocol2.RoleAdmin, "Pixel 8")
	if resp := do(t, env.dial(t, c), protocol2.Request{Cmd: "shutdown"}); resp.Code != protocol2.CodeForbidden {
		t.Fatalf("liste blanche : %+v", resp)
	}
}

func TestV2RevocationAndLastAdmin(t *testing.T) {
	env := startV2(t, nil, nil)
	adminID, phoneID := newIdentity(t), newIdentity(t)
	adminPin := env.enroll(t, adminID, protocol2.RoleAdmin, "PC portable")
	phonePin := env.enroll(t, phoneID, protocol2.RoleControl, "Pixel 8")
	admin, phone := env.dial(t, adminID), env.dial(t, phoneID)
	do(t, phone, protocol2.Request{Cmd: "status"})

	if resp := do(t, admin, protocol2.Request{Cmd: protocol2.CmdDevicesRename, Device: phonePin.String(), Name: "  Pixel de Léa "}); !resp.OK {
		t.Fatalf("renommage : %+v", resp)
	}
	if resp := do(t, admin, protocol2.Request{Cmd: protocol2.CmdDevicesRevoke, Device: phonePin.String()}); !resp.OK {
		t.Fatalf("révocation : %+v", resp)
	}
	closed(t, phone) // connexion ouverte fermée sur-le-champ
	if resp := do(t, env.dial(t, phoneID), protocol2.Request{Cmd: "status"}); resp.Code != protocol2.CodeUnknownDevice {
		t.Fatalf("appareil révoqué : %+v", resp)
	}
	events := env.journal.Snapshot().Events
	if !slices.ContainsFunc(events, func(e protocol.HistoryEvent) bool {
		return e.K == protocol.HistoryDevice && e.A == protocol.DeviceRevoked && e.B == "Pixel de Léa" && e.I == phonePin.Short() && e.X == "control"
	}) {
		t.Errorf("révocation non notée : %+v", events)
	}

	// Dernier administrateur : ni retiré ni rétrogradé par le réseau…
	for _, req := range []protocol2.Request{
		{Cmd: protocol2.CmdDevicesRevoke, Device: adminPin.String()},
		{Cmd: protocol2.CmdDevicesRole, Device: adminPin.String(), Role: "view"},
		{Cmd: protocol2.CmdDevicesRevoke, Device: adminPin.String(), Force: new(true)},
	} {
		if resp := do(t, admin, req); resp.Code != protocol2.CodeLastAdmin {
			t.Fatalf("%s : %+v", req.Cmd, resp)
		}
	}
	// … mais depuis le PC, après confirmation.
	local := env.local(t)
	if resp := do(t, local, protocol2.Request{Cmd: protocol2.CmdDevicesRevoke, Device: adminPin.String()}); resp.Code != protocol2.CodeLastAdmin {
		t.Fatalf("sans confirmation : %+v", resp)
	}
	if resp := do(t, local, protocol2.Request{Cmd: protocol2.CmdDevicesRevoke, Device: adminPin.String(), Force: new(true)}); !resp.OK {
		t.Fatalf("avec confirmation : %+v", resp)
	}
	closed(t, admin)
	if bad := do(t, local, protocol2.Request{Cmd: protocol2.CmdDevicesRevoke, Device: "abc"}); bad.Code != protocol2.CodeBadRequest {
		t.Errorf("empreinte invalide : %+v", bad)
	}
}

func TestV2Pairing(t *testing.T) {
	env := startV2(t, nil, nil)
	local := env.local(t)
	created := do(t, local, protocol2.Request{Cmd: protocol2.CmdInvite, Role: "control", Name: "Pixel de Léa"})
	inv := created.Invite
	if !created.OK || inv == nil || len(inv.Code) != protocol2.CodeLength || inv.Agent != env.id.Pin.String() || inv.Role != "control" {
		t.Fatalf("invitation : %+v", created)
	}
	if expires := time.Unix(inv.Expires, 0); time.Until(expires) < 9*time.Minute || time.Until(expires) > 11*time.Minute {
		t.Errorf("validité : %s", expires)
	}
	if list := do(t, local, protocol2.Request{Cmd: protocol2.CmdInvites}); len(list.Invites) != 1 || list.Invites[0].Code != "" {
		t.Errorf("invitations listées avec leur code : %+v", list.Invites)
	}

	newcomer := newIdentity(t)
	s := env.dial(t, newcomer)
	failed := do(t, s, protocol2.Request{Cmd: protocol2.CmdPair, Code: "0000-0000", Name: "Pixel 8"})
	if failed.Code != protocol2.CodePairingFailed || failed.Message != msgPairingFailed || failed.Hostname != "" {
		t.Fatalf("mauvais code : %+v", failed)
	}
	closed(t, s)

	s = env.dial(t, newcomer)
	resp := do(t, s, protocol2.Request{Cmd: protocol2.CmdPair, Code: strings.ToLower(protocol2.FormatCode(inv.Code)), Name: "Pixel 8"})
	if !resp.OK || resp.Role != "control" || resp.Device != pinOf(t, newcomer).String() || resp.Hostname == "" {
		t.Fatalf("appairage : %+v", resp)
	}
	if again := do(t, s, protocol2.Request{Cmd: "status"}); !again.OK || again.Role != "control" {
		t.Fatalf("après l'appairage : %+v", again)
	}
	if d, _ := env.reg.Lookup(pinOf(t, newcomer)); d.Name != "Pixel de Léa" || d.Via != devices.ViaPair || d.By != devices.ByLocal {
		t.Errorf("registre : %+v", d)
	}

	// Code déjà utilisé, puis invitation réservée à un autre appareil : même refus.
	reused := do(t, env.dial(t, newIdentity(t)), protocol2.Request{Cmd: protocol2.CmdPair, Code: inv.Code})
	reserved := newIdentity(t)
	bound := do(t, local, protocol2.Request{Cmd: protocol2.CmdInvite, Role: "view", Device: pinOf(t, reserved).String(), TTL: 604800})
	intruder := do(t, env.dial(t, newIdentity(t)), protocol2.Request{Cmd: protocol2.CmdPair, Code: bound.Invite.Code})
	for _, r := range []protocol2.Response{reused, intruder} {
		if r.Code != protocol2.CodePairingFailed || r.Message != msgPairingFailed {
			t.Errorf("refus : %+v", r)
		}
	}
	if ok := do(t, env.dial(t, reserved), protocol2.Request{Cmd: protocol2.CmdPair, Code: bound.Invite.Code, Name: "Tablette"}); !ok.OK || ok.Role != "view" {
		t.Errorf("invitation réservée : %+v", ok)
	}

	for _, bad := range []protocol2.Request{
		{Cmd: protocol2.CmdInvite, Role: "owner"},
		{Cmd: protocol2.CmdInvite, TTL: 30},
		{Cmd: protocol2.CmdInvite, TTL: 604801},
		{Cmd: protocol2.CmdInvite, Device: "abc"},
		{Cmd: protocol2.CmdInvitesCancel, Invite: "inconnue"},
	} {
		if r := do(t, local, bad); r.Code != protocol2.CodeBadRequest {
			t.Errorf("%+v : %+v", bad, r)
		}
	}
	added := 0
	for _, e := range env.journal.Snapshot().Events {
		if e.K == protocol.HistoryDevice && e.A == protocol.DeviceAdded {
			added++
		}
	}
	if added != 2 {
		t.Errorf("%d ajout(s) noté(s) au journal", added)
	}
}

func TestV2PairingSuspension(t *testing.T) {
	env := startV2(t, nil, lenient)
	inv := do(t, env.local(t), protocol2.Request{Cmd: protocol2.CmdInvite}).Invite
	for range guard.PairingMaxFailures + 1 {
		do(t, env.dial(t, newIdentity(t)), protocol2.Request{Cmd: protocol2.CmdPair, Code: "0000-0000"})
	}
	// Invitations annulées et appairage suspendu : même le bon code est refusé.
	if r := do(t, env.dial(t, newIdentity(t)), protocol2.Request{Cmd: protocol2.CmdPair, Code: inv.Code}); r.Code != protocol2.CodePairingFailed {
		t.Fatalf("appairage suspendu : %+v", r)
	}
	if n := len(env.reg.Invites()); n != 0 {
		t.Errorf("%d invitation(s) restante(s)", n)
	}
}

func TestV2Migration(t *testing.T) {
	env := startV2(t, nil, lenient)
	key, _ := env.cfg.KeyBytes()
	phone := newIdentity(t)
	s, err := env.dialAs(t, phone, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Do(ctx5(t), protocol2.Request{Cmd: "status"}); !errors.Is(err, client.ErrUnpinned) {
		t.Fatalf("requête avant la preuve de l'agent : %v", err)
	}
	resp, err := s.Migrate(ctx5(t), key, "Pixel 8")
	if err != nil || !resp.OK || resp.Role != "control" || resp.Device != pinOf(t, phone).String() || s.AgentPin() != env.id.Pin {
		t.Fatalf("migration : %+v %v", resp, err)
	}
	if again := do(t, s, protocol2.Request{Cmd: "status"}); !again.OK {
		t.Fatalf("après la migration : %+v", again)
	}
	if d, _ := env.reg.Lookup(pinOf(t, phone)); d.Via != devices.ViaMigrate || d.Role != protocol2.RoleControl || d.Name != "Pixel 8" {
		t.Errorf("registre : %+v", d)
	}

	// Mauvaise clé : la preuve de l'agent ne passe pas, l'appareil n'envoie rien d'autre.
	stranger := newIdentity(t)
	s, _ = env.dialAs(t, stranger, nil)
	wrong := slices.Clone(key)
	wrong[0] ^= 1
	if _, err := s.Migrate(ctx5(t), wrong, "x"); !errors.Is(err, client.ErrMigration) {
		t.Fatalf("mauvaise clé : %v", err)
	}
	if _, ok := env.reg.Lookup(pinOf(t, stranger)); ok {
		t.Fatal("appareil enregistré sans preuve")
	}

	// Fausse preuve de l'appareil, et seconde étape sans la première.
	s, _ = env.dialAs(t, stranger, nil)
	if r := do(t, s, protocol2.Request{Cmd: protocol2.CmdMigrate, Nonce: protocol2.NewNonce().String()}); !r.OK || r.Nonce == "" || r.Proof == "" || r.Hostname != "" {
		t.Fatalf("première étape : %+v", r)
	}
	if r := do(t, s, protocol2.Request{Cmd: protocol2.CmdMigrate, Proof: protocol2.Proof{}.String()}); r.Code != protocol2.CodeMigrationFailed {
		t.Fatalf("fausse preuve : %+v", r)
	}
	closed(t, s)
	s, _ = env.dialAs(t, stranger, nil)
	if r := do(t, s, protocol2.Request{Cmd: protocol2.CmdMigrate, Proof: protocol2.Proof{}.String()}); r.Code != protocol2.CodeMigrationFailed {
		t.Fatalf("seconde étape seule : %+v", r)
	}
	if _, ok := env.reg.Lookup(pinOf(t, stranger)); ok {
		t.Fatal("appareil enregistré avec une fausse preuve")
	}
}

func TestV2MigrationNeedsV1(t *testing.T) {
	env := startV2(t, func(c *config.Config) { c.Protocols = []string{config.ProtocolV2} }, nil)
	key, _ := env.cfg.KeyBytes()
	s, err := env.dialAs(t, newIdentity(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp, err := s.Migrate(ctx5(t), key, "x"); err != nil || resp.Code != protocol2.CodeMigrationFailed {
		t.Fatalf("migration sans wolagent/1 : %+v %v", resp, err)
	}
}

// TestV2MigrationDetectsRelay : un intermédiaire relaie la migration entre l'appareil et l'agent avec
// ses propres identités. Il connaît tout sauf la clé wolagent/1 : la preuve de l'agent, calculée avec
// les empreintes vues par l'agent, ne passe pas chez l'appareil (§9).
func TestV2MigrationDetectsRelay(t *testing.T) {
	env := startV2(t, nil, lenient)
	key, _ := env.cfg.KeyBytes()
	towardDevice, towardAgent := newIdentity(t), newIdentity(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = conn.Write([]byte(`{"proto":"wolagent/1","nonce":"x"}` + "\n"))
		down := tls.Server(conn, &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{towardDevice},
			ClientAuth: tls.RequireAnyClientCert, NextProtos: []string{protocol2.ALPN}})
		up, err := client.Dial(context.Background(), env.addr, client.Options{Identity: towardAgent})
		if err != nil {
			return
		}
		defer up.Close()
		for {
			payload, err := protocol2.ReadFrame(down, protocol2.MaxRequestBytes)
			if err != nil {
				return
			}
			var req protocol2.Request
			_ = json.Unmarshal(payload, &req)
			resp, err := up.Do(context.Background(), req)
			if err != nil {
				return
			}
			raw, _ := json.Marshal(resp)
			if protocol2.WriteFrame(down, raw, protocol2.MaxResponseBytes) != nil {
				return
			}
		}
	}()
	phone := newIdentity(t)
	s, err := client.Dial(ctx5(t), ln.Addr().String(), client.Options{Identity: phone})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.Migrate(ctx5(t), key, "Pixel 8"); !errors.Is(err, client.ErrMigration) {
		t.Fatalf("intermédiaire non détecté : %v", err)
	}
	if len(env.reg.Devices()) != 0 {
		t.Fatalf("appareil enregistré : %+v", env.reg.Devices())
	}
}

// frame envoie une trame brute et lit la réponse (nil si l'agent a fermé sans répondre).
func frame(t *testing.T, tc *tls.Conn, payload []byte) *protocol2.Response {
	t.Helper()
	header := binary.BigEndian.AppendUint32(nil, uint32(len(payload)))
	_, _ = tc.Write(append(header, payload...))
	raw, err := protocol2.ReadFrame(tc, protocol2.MaxResponseBytes)
	if err != nil {
		return nil
	}
	var resp protocol2.Response
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatal(err)
	}
	return &resp
}

func TestV2BadFramesAndRequests(t *testing.T) {
	env := startV2(t, nil, lenient)
	phone := newIdentity(t)
	env.enroll(t, phone, protocol2.RoleView, "Pixel 8")
	open := func() *tls.Conn {
		tc, err := rawTLS(t, env.addr, baseClient(phone))
		if err != nil {
			t.Fatal(err)
		}
		return tc
	}
	if resp := frame(t, open(), nil); resp != nil {
		t.Errorf("trame vide : %+v", resp)
	}
	tc := open()
	_, _ = tc.Write(binary.BigEndian.AppendUint32(nil, protocol2.MaxRequestBytes+1))
	if _, err := protocol2.ReadFrame(tc, protocol2.MaxResponseBytes); err == nil {
		t.Error("trame trop longue acceptée")
	}
	tc = open()
	_, _ = tc.Write([]byte{0, 0, 0, 10, '{'})
	_ = tc.CloseWrite()
	if _, err := protocol2.ReadFrame(tc, protocol2.MaxResponseBytes); err == nil {
		t.Error("trame tronquée acceptée")
	}
	for _, payload := range []string{`{`, `[1]`, `"status"`, `null`, `{"id":0,"cmd":"status"}`, `{"id":2147483648,"cmd":"status"}`, `{"id":1.5,"cmd":"status"}`} {
		tc := open()
		resp := frame(t, tc, []byte(payload))
		if resp == nil || resp.Code != protocol2.CodeBadRequest {
			t.Errorf("%s : %+v", payload, resp)
		}
		if again := frame(t, tc, []byte(`{"id":9,"cmd":"status"}`)); again != nil {
			t.Errorf("%s : connexion gardée", payload)
		}
	}
	// Identifiants strictement croissants.
	tc = open()
	if resp := frame(t, tc, []byte(`{"id":5,"cmd":"status"}`)); resp == nil || !resp.OK || resp.ID != 5 {
		t.Fatalf("requête 5 : %+v", resp)
	}
	if resp := frame(t, tc, []byte(`{"id":5,"cmd":"status"}`)); resp == nil || resp.Code != protocol2.CodeBadRequest {
		t.Errorf("identifiant répété : %+v", resp)
	}
}

func TestV2RequestsPerConnection(t *testing.T) {
	env := startV2(t, nil, nil)
	phone := newIdentity(t)
	env.enroll(t, phone, protocol2.RoleView, "Pixel 8")
	s := env.dial(t, phone)
	for i := range protocol2.MaxRequestsPerConnection {
		if _, err := s.Do(ctx5(t), protocol2.Request{Cmd: "status"}); err != nil {
			t.Fatalf("requête %d : %v", i+1, err)
		}
	}
	closed(t, s)
}

func TestTCPProbesAreNotFailures(t *testing.T) {
	env := startV2(t, nil, nil)
	for range 10 {
		conn, err := net.Dial("tcp", env.addr)
		if err != nil {
			t.Fatal(err)
		}
		_ = conn.Close()
	}
	phone := newIdentity(t)
	env.enroll(t, phone, protocol2.RoleView, "Pixel 8")
	if resp := do(t, env.dial(t, phone), protocol2.Request{Cmd: "status"}); !resp.OK {
		t.Fatalf("après des sondes TCP : %+v", resp)
	}
}

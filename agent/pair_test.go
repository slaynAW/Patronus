package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"io"
	"log"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/internal/config"
	"github.com/slaynaw/wakeonlan/agent/internal/devices"
	"github.com/slaynaw/wakeonlan/agent/internal/identity"
	"github.com/slaynaw/wakeonlan/agent/internal/netinfo"
	"github.com/slaynaw/wakeonlan/agent/internal/power"
	"github.com/slaynaw/wakeonlan/agent/internal/server"
	"github.com/slaynaw/wakeonlan/agent/protocol2"
	"github.com/slaynaw/wakeonlan/agent/protocol2/client"
)

// deviceCert crée l'identité wolagent/2 d'un appareil (téléphone ou PC).
func deviceCert(t *testing.T) tls.Certificate {
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

// pairEnv : configuration, identité et registre d'un agent dans un dossier temporaire, avec un port
// réservé pour son service.
type pairEnv struct {
	dir, cfgPath string
	cfg          *config.Config
	id           *identity.Identity
	port         int
}

func newPairEnv(t *testing.T) *pairEnv {
	t.Helper()
	dir := t.TempDir()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	cfg, err := config.New("PC Bureau", port)
	if err != nil {
		t.Fatal(err)
	}
	e := &pairEnv{dir: dir, cfgPath: filepath.Join(dir, "config.json"), cfg: cfg, port: port}
	if err := config.Save(e.cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	if e.id, _, err = identity.LoadOrCreate(dir, time.Now()); err != nil {
		t.Fatal(err)
	}
	return e
}

// serve lance le service sur le port de l'environnement, avec le registre tel qu'il est sur le disque ;
// la fonction renvoyée l'arrête.
func (e *pairEnv) serve(t *testing.T) (*devices.Registry, func()) {
	t.Helper()
	registry, err := devices.Open(devices.Path(e.dir), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := server.New(e.cfg, power.DryRun{Logger: log.New(io.Discard, "", 0)}, "test", log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	srv.EnableV2(e.id, registry)
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(e.port)))
	if err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithCancel(context.Background())
	served := make(chan struct{})
	go func() { _ = srv.Serve(ctx, ln); close(served) }()
	return registry, func() { stop(); <-served }
}

// pairWith appaire une nouvelle identité avec le lien, comme le fera une application : empreinte de
// l'agent épinglée d'après le lien, puis pair avec le code.
func pairWith(t *testing.T, link protocol2.Link, port int) (protocol2.Response, protocol2.Pin) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	session, err := client.Dial(ctx, net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), client.Options{Identity: deviceCert(t), Agent: &link.Agent})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	resp, err := session.Do(ctx, protocol2.Request{Cmd: protocol2.CmdPair, Code: protocol2.FormatCode(link.Code), Name: "Pixel 8"})
	if err != nil {
		t.Fatal(err)
	}
	return resp, session.DevicePin()
}

var testIface = &netinfo.Interface{Name: "Ethernet", IP: net.IPv4(192, 168, 1, 20).To4(), MAC: net.HardwareAddr{0x30, 0x9c, 0x23, 0x01, 0x02, 0x03}}

// TestPairInvitesThroughService : wol-agent pair fait créer l'invitation par le service ; le lien,
// sans aucun secret durable, suffit pour s'appairer avec le rôle prévu, une seule fois.
func TestPairInvitesThroughService(t *testing.T) {
	e := newPairEnv(t)
	registry, stop := e.serve(t)
	defer stop()

	before := time.Now()
	p, err := invitePairing(e.cfg, e.cfgPath, testIface, pairingOptions{Role: protocol2.RoleAdmin, TTL: 10 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	l := p.Link
	if p.Offline || l.Agent != e.id.Pin || l.Role != protocol2.RoleAdmin || l.Host != "192.168.1.20" || l.Port != e.port ||
		l.MAC != "30:9C:23:01:02:03" || l.Name != "PC Bureau" {
		t.Fatalf("appairage : %+v", p)
	}
	if l.Expires < before.Add(10*time.Minute).Unix() || l.Expires > time.Now().Add(10*time.Minute).Unix() {
		t.Errorf("expiration %d", l.Expires)
	}
	parsed, err := protocol2.ParseLink(l.String())
	if err != nil || parsed != l {
		t.Fatalf("lien relu : %+v %v (attendu %+v)", parsed, err, l)
	}

	// Affichage : le code, l'empreinte courte et le lien ; jamais la clé wolagent/1.
	var out bytes.Buffer
	p.Legacy = true
	if err := printPairingV2(&out, p, false, false, time.Now()); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{protocol2.FormatCode(l.Code), e.id.Pin.Short(), l.String(), "Rôle donné : admin", "wol-agent pair --v1"} {
		if !strings.Contains(text, want) {
			t.Errorf("affichage sans %q :\n%s", want, text)
		}
	}
	if strings.Contains(text, e.cfg.Key) || strings.Contains(text, "Service arrêté") {
		t.Errorf("affichage :\n%s", text)
	}

	resp, device := pairWith(t, parsed, e.port)
	if !resp.OK || resp.Role != "admin" {
		t.Fatalf("appairage refusé : %+v", resp)
	}
	if d, ok := registry.Lookup(device); !ok || d.Role != protocol2.RoleAdmin || d.Via != devices.ViaPair || d.By != devices.ByLocal || d.Name != "Pixel 8" {
		t.Errorf("registre : %+v", d)
	}
	if again, _ := pairWith(t, parsed, e.port); again.OK || again.Code != protocol2.CodePairingFailed {
		t.Errorf("code réutilisé : %+v", again)
	}
}

// TestPairOffline : service arrêté, l'invitation est écrite dans le registre et acceptée au démarrage ;
// après install (wait), un service qui ne répond pas est une erreur, le registre n'étant pas touché.
func TestPairOffline(t *testing.T) {
	e := newPairEnv(t)
	p, err := invitePairing(e.cfg, e.cfgPath, testIface, pairingOptions{Role: protocol2.RoleControl, TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if !p.Offline || p.Link.Role != protocol2.RoleControl {
		t.Fatalf("appairage : %+v", p)
	}
	var out bytes.Buffer
	if err := printPairingV2(&out, p, false, false, time.Now()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Service arrêté") || strings.Contains(out.String(), "--v1") {
		t.Errorf("affichage :\n%s", out.String())
	}

	start := time.Now()
	if _, err := invitePairing(e.cfg, e.cfgPath, testIface, pairingOptions{Role: protocol2.RoleAdmin, TTL: time.Hour, Wait: 300 * time.Millisecond}); err == nil ||
		!strings.Contains(err.Error(), "ne répond pas") {
		t.Errorf("service absent après install : %v", err)
	}
	if time.Since(start) < 300*time.Millisecond {
		t.Error("pas d'attente du service")
	}

	registry, stop := e.serve(t)
	defer stop()
	if n := len(registry.Invites()); n != 1 {
		t.Fatalf("%d invitation(s) dans le registre", n)
	}
	resp, device := pairWith(t, p.Link, e.port)
	if !resp.OK || resp.Role != "control" {
		t.Fatalf("appairage refusé : %+v", resp)
	}
	if d, ok := registry.Lookup(device); !ok || d.Role != protocol2.RoleControl {
		t.Errorf("registre : %+v", d)
	}
}

// TestPairWaitsForService : juste après install, l'invitation attend que le service écoute, et passe
// par lui (le registre qu'il a déjà lu ne doit plus être modifié directement).
func TestPairWaitsForService(t *testing.T) {
	e := newPairEnv(t)
	type result struct {
		p   pairingV2
		err error
	}
	done := make(chan result, 1)
	go func() {
		p, err := invitePairing(e.cfg, e.cfgPath, testIface, pairingOptions{Role: protocol2.RoleAdmin, TTL: time.Hour, Wait: 10 * time.Second})
		done <- result{p, err}
	}()
	time.Sleep(400 * time.Millisecond)
	registry, stop := e.serve(t)
	defer stop()
	r := <-done
	if r.err != nil || r.p.Offline {
		t.Fatalf("appairage : %+v %v", r.p, r.err)
	}
	if n := len(registry.Invites()); n != 1 {
		t.Fatalf("%d invitation(s) connue(s) du service", n)
	}
	if resp, _ := pairWith(t, r.p.Link, e.port); !resp.OK || resp.Role != "admin" {
		t.Fatalf("appairage refusé : %+v", resp)
	}
}

// TestPairNeedsIdentity : sans identité (service jamais démarré), wol-agent pair le dit ; --v1 est
// refusé si wolagent/1 est désactivé.
func TestPairNeedsIdentity(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	cfg, err := config.New("PC", config.DefaultPort)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := invitePairing(cfg, cfgPath, testIface, pairingOptions{Role: protocol2.RoleAdmin, TTL: time.Hour}); err != errNoIdentity {
		t.Errorf("sans identité : %v", err)
	}
	cfg.Protocols = []string{config.ProtocolV2}
	if err := showPairingFor(cfg, cfgPath, pairingOptions{V1: true}); err == nil || !strings.Contains(err.Error(), "wolagent/1 est désactivé") {
		t.Errorf("--v1 sans wolagent/1 : %v", err)
	}
}

func TestParseTTL(t *testing.T) {
	for text, want := range map[string]time.Duration{
		"600": 10 * time.Minute, "60": time.Minute, "10m": 10 * time.Minute, "2h": 2 * time.Hour, "168h": 7 * 24 * time.Hour,
		"604800": 7 * 24 * time.Hour, "90.5s": 90 * time.Second,
	} {
		if got, err := parseTTL(text); err != nil || got != want {
			t.Errorf("%q : %v %v", text, got, err)
		}
	}
	for _, text := range []string{"", "59", "59s", "604801", "169h", "7d", "-600", "1e3", "99999999999999999999", "10 m"} {
		if got, err := parseTTL(text); err == nil {
			t.Errorf("%q accepté : %v", text, got)
		}
	}
}

func TestUntilText(t *testing.T) {
	now := time.Date(2026, 10, 13, 14, 22, 0, 0, time.Local)
	if got := untilText(now.Add(10*time.Minute), now); got != "à 14:32" {
		t.Errorf("même jour : %q", got)
	}
	if got := untilText(now.Add(48*time.Hour), now); got != "au 15/10/2026 à 14:22" {
		t.Errorf("autre jour : %q", got)
	}
}

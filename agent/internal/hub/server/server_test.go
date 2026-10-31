package server

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"io"
	"log"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/hubprotocol"
	"github.com/slaynaw/wakeonlan/agent/internal/devices"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/agents"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/archive"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/fleet"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/settings"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/store"
	"github.com/slaynaw/wakeonlan/agent/internal/identity"
	"github.com/slaynaw/wakeonlan/agent/protocol2"
	"github.com/slaynaw/wakeonlan/agent/protocol2/client"
)

// syncBuffer est le journal du hub pendant un test.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

type env struct {
	dir  string
	srv  *Server
	addr string
	hub  *identity.Identity
	st   *store.Store
	key  store.Key
	log  *syncBuffer
}

func start(t *testing.T) *env {
	t.Helper()
	dir := t.TempDir()
	key, _, err := store.LoadOrCreateKey(dir + "/keys")
	if err != nil {
		t.Fatal(err)
	}
	hub, _, err := identity.LoadOrCreate(dir+"/keys", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(dir+"/data", key)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := devices.OpenStorage(st.File("devices.db"), "devices.db", time.Now)
	if err != nil {
		t.Fatal(err)
	}
	pcs, err := fleet.Open(st.File(fleet.FileName), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	// Les tests joignent les agents sur l'adresse de bouclage, que le hub refuse en service.
	dialer := &agents.Dialer{Identity: hub, Networks: append(agents.TargetNetworks(), netip.MustParsePrefix("127.0.0.0/8"))}
	buf := &syncBuffer{}
	prefs, err := settings.Open(st.File(settings.FileName))
	if err != nil {
		t.Fatal(err)
	}
	srv, err := New(Config{
		Name: "Hub NAS", Version: "2.0.0-test", Identity: hub, Registry: registry, Fleet: pcs, Agents: dialer,
		Archive: archive.New(st, time.Now), Settings: prefs, Logger: log.New(buf, "", 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = ln.Close() })
	return &env{dir: dir, srv: srv, addr: ln.Addr().String(), hub: hub, st: st, key: key, log: buf}
}

func newDevice(t *testing.T) tls.Certificate {
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

func (e *env) dial(t *testing.T, id tls.Certificate) *client.Session {
	t.Helper()
	pin := e.hub.Pin
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sess, err := client.Dial(ctx, e.addr, client.Options{Identity: id, Agent: &pin, Protocol: hubprotocol.ALPN})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	return sess
}

func call(t *testing.T, sess *client.Session, req hubprotocol.Request) hubprotocol.Response {
	t.Helper()
	resp, err := try(sess, req)
	if err != nil {
		t.Fatalf("%s : %v", req.Cmd, err)
	}
	return resp
}

func try(sess *client.Session, req hubprotocol.Request) (hubprotocol.Response, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var resp hubprotocol.Response
	err := sess.Call(ctx, req, &resp)
	return resp, err
}

// local ouvre la session de la commande patronus-hub sur le NAS : l'identité du hub, depuis 127.0.0.1.
func (e *env) local(t *testing.T) *client.Session { return e.dial(t, e.hub.Cert) }

// enroll appaire un appareil avec une invitation créée sur le NAS.
func (e *env) enroll(t *testing.T, id tls.Certificate, role protocol2.Role, name string) {
	t.Helper()
	inv := call(t, e.local(t), hubprotocol.Request{Cmd: protocol2.CmdInvite, Role: role.String()})
	if !inv.OK || inv.Invite == nil {
		t.Fatalf("invitation : %+v", inv)
	}
	resp := call(t, e.dial(t, id), hubprotocol.Request{Cmd: protocol2.CmdPair, Code: inv.Invite.Code, Name: name})
	if !resp.OK || resp.Role != role.String() || resp.Device != pinOf(t, id).String() {
		t.Fatalf("appairage : %+v", resp)
	}
}

func TestPairingAndStatus(t *testing.T) {
	e := start(t)
	local := e.local(t)
	status := call(t, local, hubprotocol.Request{Cmd: hubprotocol.CmdStatus})
	if !status.OK || status.Role != "admin" || status.Hub == nil || status.Hub.Name != "Hub NAS" ||
		status.Hub.Version != "2.0.0-test" || status.Hub.Pin != e.hub.Pin.String() || status.Hub.Started == 0 {
		t.Fatalf("état vu du NAS : %+v %+v", status, status.Hub)
	}
	inv := call(t, local, hubprotocol.Request{Cmd: protocol2.CmdInvite, Role: "admin", Name: "Pixel 8"})
	if !inv.OK || inv.Invite == nil || inv.Invite.Hub != e.hub.Pin.String() || inv.Invite.Role != "admin" || len(inv.Invite.Code) != 8 {
		t.Fatalf("invitation : %+v", inv.Invite)
	}
	if strings.Contains(e.log.String(), inv.Invite.Code) {
		t.Fatal("code d'invitation écrit au journal du hub")
	}

	phone := newDevice(t)
	sess := e.dial(t, phone)
	resp := call(t, sess, hubprotocol.Request{Cmd: protocol2.CmdPair, Code: protocol2.FormatCode(inv.Invite.Code), Name: "autre nom"})
	if !resp.OK || resp.Role != "admin" {
		t.Fatalf("appairage : %+v", resp)
	}
	// Même connexion : l'appareil est maintenant enregistré, sous le nom choisi par l'invitation.
	list := call(t, sess, hubprotocol.Request{Cmd: protocol2.CmdDevices})
	if !list.OK || len(list.Devices) != 1 || list.Devices[0].Name != "Pixel 8" || !list.Devices[0].Self || list.Devices[0].Via != "pair" {
		t.Fatalf("appareils : %+v", list.Devices)
	}
	if again := call(t, sess, hubprotocol.Request{Cmd: protocol2.CmdPair, Code: inv.Invite.Code}); again.Code != protocol2.CodeAlreadyPaired {
		t.Fatalf("second appairage : %+v", again)
	}
	// Code déjà utilisé : refusé, connexion fermée.
	tablet := e.dial(t, newDevice(t))
	if r := call(t, tablet, hubprotocol.Request{Cmd: protocol2.CmdPair, Code: inv.Invite.Code}); r.Code != protocol2.CodePairingFailed {
		t.Fatalf("code réutilisé : %+v", r)
	}
	if _, err := try(tablet, hubprotocol.Request{Cmd: hubprotocol.CmdStatus}); err == nil {
		t.Fatal("connexion gardée après un appairage refusé")
	}

	// Le registre est chiffré dans le stockage, et relu avec la clé.
	raw, err := e.st.ReadFile("devices.db")
	if err != nil || !strings.Contains(string(raw), pinOf(t, phone).String()) {
		t.Fatalf("registre relu : %v", err)
	}
	// Avec une autre clé, le registre est illisible, et jamais remplacé par un registre vide.
	sealed, err := os.ReadFile(filepath.Join(e.dir, "data", "devices.db"))
	if err != nil {
		t.Fatal(err)
	}
	otherDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(otherDir, "devices.db"), sealed, 0o600); err != nil {
		t.Fatal(err)
	}
	other, _ := store.Open(otherDir, store.Key{1})
	if _, err := devices.OpenStorage(other.File("devices.db"), "devices.db", time.Now); !errors.Is(err, store.ErrCorrupted) {
		t.Fatalf("registre lu avec une autre clé : %v", err)
	}
}

func TestReservedInvite(t *testing.T) {
	e := start(t)
	phone, intruder := newDevice(t), newDevice(t)
	inv := call(t, e.local(t), hubprotocol.Request{Cmd: protocol2.CmdInvite, Device: pinOf(t, phone).String(), TTL: 3600})
	if !inv.OK || inv.Invite.Role != "view" || inv.Invite.Device != pinOf(t, phone).String() {
		t.Fatalf("invitation réservée : %+v", inv)
	}
	if r := call(t, e.dial(t, intruder), hubprotocol.Request{Cmd: protocol2.CmdPair, Code: inv.Invite.Code}); r.Code != protocol2.CodePairingFailed {
		t.Fatalf("autre appareil : %+v", r)
	}
	if r := call(t, e.dial(t, phone), hubprotocol.Request{Cmd: protocol2.CmdPair, Code: inv.Invite.Code}); !r.OK || r.Role != "view" {
		t.Fatalf("appareil prévu : %+v", r)
	}
}

func TestRoles(t *testing.T) {
	e := start(t)
	viewer, waker := newDevice(t), newDevice(t)
	e.enroll(t, viewer, protocol2.RoleView, "Tablette")
	e.enroll(t, waker, protocol2.RoleWake, "PC portable")
	v, w := e.dial(t, viewer), e.dial(t, waker)
	if r := call(t, v, hubprotocol.Request{Cmd: hubprotocol.CmdStatus}); !r.OK || r.Role != "view" {
		t.Fatalf("état (view) : %+v", r)
	}
	for _, cmd := range []string{hubprotocol.CmdWake, protocol2.CmdDevices, protocol2.CmdInvite, hubprotocol.CmdPCs} {
		if r := call(t, v, hubprotocol.Request{Cmd: cmd}); r.Code != protocol2.CodeInsufficientRole {
			t.Errorf("%s (view) : %+v", cmd, r)
		}
	}
	// Rôle suffisant : la commande passe (ici, PC inconnu) ; programmations réservées aux administrateurs ;
	// commande inconnue ou de l'agent.
	if r := call(t, w, hubprotocol.Request{Cmd: hubprotocol.CmdWake, PC: "inconnu"}); r.Code != protocol2.CodeBadRequest || r.Role != "wake" {
		t.Errorf("hub.wake (wake) : %+v", r)
	}
	if r := call(t, w, hubprotocol.Request{Cmd: hubprotocol.CmdSchedules}); r.Code != protocol2.CodeInsufficientRole {
		t.Errorf("hub.schedules (wake) : %+v", r)
	}
	if r := call(t, e.local(t), hubprotocol.Request{Cmd: hubprotocol.CmdSchedules}); !r.OK || len(r.Schedules) != 0 || r.Settings == nil {
		t.Errorf("hub.schedules : %+v", r)
	}
	for _, cmd := range []string{"shutdown", "status", "migrate", "HUB.STATUS"} {
		if r := call(t, w, hubprotocol.Request{Cmd: cmd}); r.Code != protocol2.CodeUnsupported {
			t.Errorf("%s : %+v", cmd, r)
		}
	}
	if r := call(t, w, hubprotocol.Request{Cmd: protocol2.CmdDevices}); r.Code != protocol2.CodeInsufficientRole {
		t.Errorf("devices (wake) : %+v", r)
	}
	local := e.local(t)
	for _, role := range []string{"control", "root", ""} {
		if r := call(t, local, hubprotocol.Request{Cmd: protocol2.CmdDevicesRole, Device: pinOf(t, viewer).String(), Role: role}); r.Code != protocol2.CodeBadRequest {
			t.Errorf("rôle %q : %+v", role, r)
		}
		if role != "" {
			if r := call(t, local, hubprotocol.Request{Cmd: protocol2.CmdInvite, Role: role}); r.Code != protocol2.CodeBadRequest {
				t.Errorf("invitation %q : %+v", role, r)
			}
		}
	}
	if r := call(t, local, hubprotocol.Request{Cmd: protocol2.CmdDevicesRole, Device: pinOf(t, viewer).String(), Role: "wake"}); !r.OK {
		t.Fatalf("passage en wake : %+v", r)
	}
	if r := call(t, v, hubprotocol.Request{Cmd: hubprotocol.CmdWake, PC: "inconnu"}); r.Code != protocol2.CodeBadRequest || r.Role != "wake" {
		t.Errorf("rôle appliqué sur la connexion ouverte : %+v", r)
	}
}

func TestRevocationAndLastAdmin(t *testing.T) {
	e := start(t)
	admin, tablet := newDevice(t), newDevice(t)
	e.enroll(t, admin, protocol2.RoleAdmin, "Pixel 8")
	e.enroll(t, tablet, protocol2.RoleView, "Tablette")
	a, tab := e.dial(t, admin), e.dial(t, tablet)
	if r := call(t, tab, hubprotocol.Request{Cmd: hubprotocol.CmdStatus}); !r.OK {
		t.Fatal(r)
	}
	if r := call(t, a, hubprotocol.Request{Cmd: protocol2.CmdDevicesRevoke, Device: pinOf(t, tablet).String()}); !r.OK {
		t.Fatalf("révocation : %+v", r)
	}
	if _, err := try(tab, hubprotocol.Request{Cmd: hubprotocol.CmdStatus}); err == nil {
		t.Fatal("connexion de l'appareil révoqué gardée ouverte")
	}
	if r := call(t, e.dial(t, tablet), hubprotocol.Request{Cmd: hubprotocol.CmdStatus}); r.Code != protocol2.CodeUnknownDevice {
		t.Fatalf("appareil révoqué : %+v", r)
	}
	// Dernier administrateur : protégé depuis une application, retirable sur le NAS avec confirmation.
	if r := call(t, a, hubprotocol.Request{Cmd: protocol2.CmdDevicesRevoke, Device: pinOf(t, admin).String()}); r.Code != protocol2.CodeLastAdmin {
		t.Fatalf("dernier administrateur : %+v", r)
	}
	if r := call(t, a, hubprotocol.Request{Cmd: protocol2.CmdDevicesRole, Device: pinOf(t, admin).String(), Role: "view"}); r.Code != protocol2.CodeLastAdmin {
		t.Fatalf("rétrogradation du dernier administrateur : %+v", r)
	}
	force := true
	notForced := call(t, e.local(t), hubprotocol.Request{Cmd: protocol2.CmdDevicesRevoke, Device: pinOf(t, admin).String()})
	if notForced.Code != protocol2.CodeLastAdmin {
		t.Fatalf("sur le NAS sans confirmation : %+v", notForced)
	}
	if r := call(t, e.local(t), hubprotocol.Request{Cmd: protocol2.CmdDevicesRevoke, Device: pinOf(t, admin).String(), Force: &force}); !r.OK {
		t.Fatalf("sur le NAS avec confirmation : %+v", r)
	}
	// La confirmation n'a d'effet que sur le NAS.
	other := newDevice(t)
	e.enroll(t, other, protocol2.RoleAdmin, "PC")
	o := e.dial(t, other)
	if r := call(t, o, hubprotocol.Request{Cmd: protocol2.CmdDevicesRevoke, Device: pinOf(t, other).String(), Force: &force}); r.Code != protocol2.CodeLastAdmin {
		t.Fatalf("confirmation depuis une application : %+v", r)
	}
}

func TestInvitesAndRename(t *testing.T) {
	e := start(t)
	local := e.local(t)
	inv := call(t, local, hubprotocol.Request{Cmd: protocol2.CmdInvite, Role: "wake", Name: "Pixel de Léa", TTL: 7200})
	list := call(t, local, hubprotocol.Request{Cmd: protocol2.CmdInvites})
	if !list.OK || len(list.Invites) != 1 || list.Invites[0].ID != inv.Invite.ID || list.Invites[0].Code != "" || list.Invites[0].Role != "wake" {
		t.Fatalf("invitations : %+v", list.Invites)
	}
	if r := call(t, local, hubprotocol.Request{Cmd: protocol2.CmdInvitesCancel, Invite: inv.Invite.ID}); !r.OK {
		t.Fatal(r)
	}
	if r := call(t, local, hubprotocol.Request{Cmd: protocol2.CmdInvitesCancel, Invite: inv.Invite.ID}); r.Code != protocol2.CodeBadRequest {
		t.Fatalf("invitation déjà annulée : %+v", r)
	}
	for _, ttl := range []int64{59, 7*24*3600 + 1, -1} {
		if r := call(t, local, hubprotocol.Request{Cmd: protocol2.CmdInvite, TTL: ttl}); r.Code != protocol2.CodeBadRequest {
			t.Errorf("durée %d : %+v", ttl, r)
		}
	}
	phone := newDevice(t)
	e.enroll(t, phone, protocol2.RoleView, "Pixel")
	if r := call(t, local, hubprotocol.Request{Cmd: protocol2.CmdDevicesRename, Device: pinOf(t, phone).String(), Name: "  Pixel 8\u0007 "}); !r.OK {
		t.Fatal(r)
	}
	devs := call(t, local, hubprotocol.Request{Cmd: protocol2.CmdDevices})
	if len(devs.Devices) != 1 || devs.Devices[0].Name != "Pixel 8" {
		t.Fatalf("nom nettoyé : %+v", devs.Devices)
	}
	if r := call(t, local, hubprotocol.Request{Cmd: protocol2.CmdDevicesRename, Device: pinOf(t, phone).String(), Name: " \u0001 "}); r.Code != protocol2.CodeBadRequest {
		t.Fatalf("nom vide : %+v", r)
	}
	if r := call(t, local, hubprotocol.Request{Cmd: protocol2.CmdDevicesRevoke, Device: "abc"}); r.Code != protocol2.CodeBadRequest {
		t.Fatalf("empreinte invalide : %+v", r)
	}
}

// TestRefusals : identité inconnue, mauvais protocole et requêtes mal formées comptent comme des échecs ;
// après 5, l'adresse est bloquée et le salut l'annonce.
func TestRefusals(t *testing.T) {
	e := start(t)
	stranger := newDevice(t)
	sess := e.dial(t, stranger)
	if r := call(t, sess, hubprotocol.Request{Cmd: hubprotocol.CmdStatus}); r.Code != protocol2.CodeUnknownDevice || r.Hub != nil || r.Role != "" {
		t.Fatalf("identité inconnue : %+v", r)
	}
	if _, err := try(sess, hubprotocol.Request{Cmd: hubprotocol.CmdStatus}); err == nil {
		t.Fatal("connexion gardée après une identité inconnue")
	}
	// Mauvais protocole applicatif : poignée de main refusée.
	raw, err := net.Dial("tcp", e.addr)
	if err != nil {
		t.Fatal(err)
	}
	hello := make([]byte, len(hubprotocol.Hello)+1)
	if _, err := io.ReadFull(raw, hello); err != nil || string(hello) != hubprotocol.Hello+"\n" {
		t.Fatalf("salut : %q, %v", hello, err)
	}
	conn := tls.Client(raw, &tls.Config{
		MinVersion: tls.VersionTLS13, NextProtos: []string{protocol2.ALPN}, Certificates: []tls.Certificate{stranger},
		InsecureSkipVerify: true,
	})
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if err := conn.Handshake(); err == nil {
		if _, err := conn.Read(make([]byte, 1)); err == nil {
			t.Fatal("ALPN wolagent/2 accepté par le hub")
		}
	}
	_ = conn.Close()
	// Un client wolagent/2 ordinaire refuse le hub dès le salut.
	pin := e.hub.Pin
	if _, err := client.Dial(context.Background(), e.addr, client.Options{Identity: stranger, Agent: &pin}); err == nil {
		t.Fatal("hub pris pour un agent")
	}
	// Requête mal formée ou identifiant non croissant : bad_request et fermeture.
	admin := newDevice(t)
	e.enroll(t, admin, protocol2.RoleAdmin, "Pixel")
	a := e.dial(t, admin)
	call(t, a, hubprotocol.Request{Cmd: hubprotocol.CmdStatus})

	// Les échecs de l'identité inconnue s'accumulent : l'adresse finit bloquée, et le salut l'annonce.
	blocked := false
	for range 10 {
		s, err := client.Dial(context.Background(), e.addr, client.Options{Identity: stranger, Agent: &pin, Protocol: hubprotocol.ALPN})
		if errors.Is(err, client.ErrRateLimited) {
			blocked = true
			break
		}
		if err == nil {
			_, _ = try(s, hubprotocol.Request{Cmd: hubprotocol.CmdStatus})
			_ = s.Close()
		}
	}
	if !blocked || !strings.Contains(e.log.String(), "bloqué 5 minutes") {
		t.Fatalf("adresse non bloquée : %s", e.log.String())
	}
	// Le blocage vaut pour l'adresse, appareils autorisés compris, pendant 5 minutes.
	if _, err := client.Dial(context.Background(), e.addr, client.Options{Identity: admin, Agent: &pin, Protocol: hubprotocol.ALPN}); !errors.Is(err, client.ErrRateLimited) {
		t.Fatalf("appareil autorisé depuis l'adresse bloquée : %v", err)
	}
}

func TestAllowedNetworks(t *testing.T) {
	e := start(t)
	for ip, want := range map[string]bool{
		"192.168.1.20": true, "10.0.0.5": true, "172.20.1.1": true, "100.100.1.1": true, "127.0.0.1": true,
		"169.254.3.4": true, "::1": true, "fd12::1": true, "fe80::1": true,
		"8.8.8.8": false, "203.0.113.9": false, "2001:db8::1": false, "172.32.0.1": false,
	} {
		if got := e.srv.allowed(net.ParseIP(ip)); got != want {
			t.Errorf("%s : %v", ip, got)
		}
	}
}

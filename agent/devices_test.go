package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"io"
	"log"
	"net"
	"path/filepath"
	"slices"
	"strings"
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
)

func devicePin(t *testing.T) protocol2.Pin {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := protocol2.NewCertificate(key, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	pin, _ := protocol2.PinFromCertificate(der)
	return pin
}

// TestDevicesCommand : wol-agent devices passe par le service quand il tourne (révocation immédiate),
// et modifie le registre directement quand il est arrêté.
func TestDevicesCommand(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.New("PC Bureau", ln.Addr().(*net.TCPAddr).Port)
	if err != nil {
		t.Fatal(err)
	}
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	registry, err := devices.Open(devices.Path(dir), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	admin, phone := devicePin(t), devicePin(t)
	for _, d := range []devices.Device{
		{Pin: admin, Name: "PC portable", Role: protocol2.RoleAdmin, Via: devices.ViaPair},
		{Pin: phone, Name: "Pixel 8", Role: protocol2.RoleControl, Via: devices.ViaMigrate},
	} {
		if _, err := registry.Add(d); err != nil {
			t.Fatal(err)
		}
	}
	id, _, err := identity.LoadOrCreate(dir, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	journal, err := history.Open(historyPath(cfgPath), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := server.New(cfg, power.DryRun{Logger: log.New(io.Discard, "", 0)}, "test", log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	srv.EnableV2(id, registry)
	srv.SetHistory(journal)
	ctx, stop := context.WithCancel(context.Background())
	served := make(chan struct{})
	go func() { _ = srv.Serve(ctx, ln); close(served) }()

	// Service lancé : liste, puis retrait par le début de l'empreinte courte.
	if err := cmdDevices([]string{"--config=" + cfgPath}); err != nil {
		t.Fatal(err)
	}
	if err := cmdDevices([]string{"--config=" + cfgPath, "revoke", phone.Short()[:6]}); err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Lookup(phone); ok {
		t.Fatal("appareil non retiré")
	}
	if !slices.ContainsFunc(journal.Snapshot().Events, func(e protocol.HistoryEvent) bool {
		return e.K == protocol.HistoryDevice && e.A == protocol.DeviceRevoked && e.I == phone.Short()
	}) {
		t.Error("retrait non noté au journal")
	}
	if err := cmdDevices([]string{"--config=" + cfgPath, "revoke", "ZZZZZZ"}); err == nil {
		t.Error("appareil inconnu accepté")
	}

	// Service arrêté : le registre est modifié directement ; dernier administrateur avec --yes.
	stop()
	<-served
	if err := cmdDevices([]string{"--config=" + cfgPath, "rename", admin.String()[:8], "Portable", "de", "Léa"}); err != nil {
		t.Fatal(err)
	}
	if err := cmdDevices([]string{"--config=" + cfgPath, "--yes", "role", admin.Short(), "view"}); err != nil {
		t.Fatal(err)
	}
	reread, err := devices.Open(devices.Path(dir), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if d, _ := reread.Lookup(admin); d.Name != "Portable de Léa" || d.Role != protocol2.RoleView {
		t.Errorf("registre : %+v", d)
	}
	if err := cmdDevices([]string{"--config=" + cfgPath, "role", admin.Short(), "owner"}); err == nil {
		t.Error("rôle inconnu accepté")
	}
}

func TestResolveDevice(t *testing.T) {
	a, b := devicePin(t), devicePin(t)
	list := []protocol2.DeviceInfo{{Device: a.String(), Name: "a"}, {Device: b.String(), Name: "b"}}
	// Une empreinte complète dont le début contient « - » (alphabet B64) : elle doit rester trouvable.
	for !strings.Contains(a.String()[:10], "-") {
		a = devicePin(t)
	}
	list = []protocol2.DeviceInfo{{Device: a.String(), Name: "a"}, {Device: b.String(), Name: "b"}}
	for _, prefix := range []string{a.Short(), a.Short()[:4], " " + a.Short()[:9] + " ", strings.ToLower(a.Short()[:6]), a.String()[:10]} {
		if d, err := resolveDevice(list, prefix); err != nil || d.Name != "a" {
			t.Errorf("%q : %+v %v", prefix, d, err)
		}
	}
	for _, prefix := range []string{"", "AB", "?????"} {
		if _, err := resolveDevice(list, prefix); err == nil {
			t.Errorf("%q accepté", prefix)
		}
	}
}

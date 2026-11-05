package main

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/slaynaw/wakeonlan/agent/internal/config"
)

// stdoutOf renvoie ce que f écrit sur la sortie standard.
func stdoutOf(t *testing.T, f func() error) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stdout
	os.Stdout = w
	done := make(chan string)
	go func() {
		raw, _ := io.ReadAll(r)
		done <- string(raw)
	}()
	ferr := f()
	os.Stdout = saved
	_ = w.Close()
	return <-done, ferr
}

// TestProtocolsCommand : wol-agent protocols rétablit puis refuse wolagent/1, sans jamais afficher la
// clé, et ne redémarre le service que si la configuration change.
func TestProtocolsCommand(t *testing.T) {
	restarts := 0
	saved := restartService
	restartService = func() error { restarts++; return nil }
	t.Cleanup(func() { restartService = saved })

	cfgPath := filepath.Join(t.TempDir(), "config.json")
	cfg, err := config.New("PC Bureau", config.DefaultPort)
	if err != nil {
		t.Fatal(err)
	}
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) (string, *config.Config) {
		t.Helper()
		out, err := stdoutOf(t, func() error { return cmdProtocols(append([]string{"--config", cfgPath}, args...)) })
		if err != nil {
			t.Fatalf("protocols %v : %v", args, err)
		}
		loaded, err := config.Load(cfgPath)
		if err != nil {
			t.Fatal(err)
		}
		return out, loaded
	}

	// Nouvelle installation : wolagent/2 seulement, et le moyen de rétablir wolagent/1.
	out, got := run()
	if !strings.Contains(out, "wolagent/2") || strings.Contains(out, "wolagent/1 et") || !strings.Contains(out, "protocols v1 on") {
		t.Errorf("affichage :\n%s", out)
	}
	if restarts != 0 || got.Accepts(config.ProtocolV1) {
		t.Fatalf("simple affichage : %d redémarrage(s), %v", restarts, got.Protocols)
	}

	out, got = run("v1", "on")
	if !slices.Equal(got.Protocols, []string{config.ProtocolV1, config.ProtocolV2}) || got.Key == "" || restarts != 1 {
		t.Fatalf("v1 on : %v, %d redémarrage(s)", got.Protocols, restarts)
	}
	if strings.Contains(out, got.Key) || !strings.Contains(out, "pair --v1") {
		t.Errorf("v1 on, affichage :\n%s", out)
	}
	key := got.Key

	// Déjà accepté : rien ne change, la clé est gardée.
	if _, got = run("v1", "on"); got.Key != key || restarts != 1 {
		t.Fatalf("v1 on deux fois : clé changée ou %d redémarrage(s)", restarts)
	}
	if out, _ := run(); strings.Contains(out, key) || !strings.Contains(out, "wolagent/1 et wolagent/2") {
		t.Errorf("affichage en coexistence :\n%s", out)
	}

	out, got = run("v1", "off")
	if !slices.Equal(got.Protocols, []string{config.ProtocolV2}) || got.Key != "" || restarts != 2 {
		t.Fatalf("v1 off : %v, clé %t, %d redémarrage(s)", got.Protocols, got.Key != "", restarts)
	}
	if strings.Contains(out, key) || !strings.Contains(out, "clé effacée") {
		t.Errorf("v1 off, affichage :\n%s", out)
	}
	run("v1", "off")
	if restarts != 2 {
		t.Fatalf("v1 off deux fois : %d redémarrage(s)", restarts)
	}

	// Installation antérieure à 1.14.0 (« protocols » absent : les deux) : v1 off efface la clé.
	legacy, err := config.New("PC Bureau", config.DefaultPort)
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.EnableV1(); err != nil {
		t.Fatal(err)
	}
	legacy.Protocols = nil
	if err := config.Save(cfgPath, legacy); err != nil {
		t.Fatal(err)
	}
	if _, got = run("v1", "off"); got.Accepts(config.ProtocolV1) || got.Key != "" || restarts != 3 {
		t.Fatalf("installation antérieure, v1 off : %v, %d redémarrage(s)", got.Protocols, restarts)
	}

	for _, bad := range [][]string{{"v1"}, {"v2", "on"}, {"v1", "oui"}, {"v1", "on", "off"}} {
		if _, err := stdoutOf(t, func() error { return cmdProtocols(append([]string{"--config", cfgPath}, bad...)) }); err == nil {
			t.Errorf("protocols %v accepté", bad)
		}
	}
	if restarts != 3 {
		t.Errorf("arguments refusés : %d redémarrage(s)", restarts)
	}
}

// TestRotateKeyNeedsV1 : sans wolagent/1, rotate-key ne crée pas de clé inutile.
func TestRotateKeyNeedsV1(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	cfg, err := config.New("PC Bureau", config.DefaultPort)
	if err != nil {
		t.Fatal(err)
	}
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	if err := cmdRotateKey([]string{"--config", cfgPath}); err == nil || !strings.Contains(err.Error(), "protocols v1 on") {
		t.Errorf("rotate-key sans wolagent/1 : %v", err)
	}
	if got, err := config.Load(cfgPath); err != nil || got.Key != "" {
		t.Errorf("clé créée : %v", err)
	}
}

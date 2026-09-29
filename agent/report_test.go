package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/diagnostic"
	"github.com/slaynaw/wakeonlan/agent/internal/config"
	"github.com/slaynaw/wakeonlan/agent/internal/history"
	"github.com/slaynaw/wakeonlan/agent/protocol"
)

func TestDiagnosticCommand(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	cfg, err := config.New("PC Bureau", 9770)
	if err != nil {
		t.Fatal(err)
	}
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	journal, err := history.Open(historyPath(cfgPath), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.Add(protocol.HistoryEvent{T: time.Now().Unix(), K: protocol.HistoryWake, C: "192.168.1.30", B: "Téléphone de Léa"}); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(dir, "rapport.diag")
	t.Setenv("PATRONUS_DIAG_PASSWORD", "court")
	if err := cmdDiagnostic([]string{"--config", cfgPath, "--out", out}); err == nil {
		t.Error("mot de passe trop court accepté")
	}
	t.Setenv("PATRONUS_DIAG_PASSWORD", "mot de passe du rapport")
	if err := cmdDiagnostic([]string{"--config", cfgPath, "--out", out}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	env, report, err := diagnostic.Open(data, "mot de passe du rapport")
	if err != nil {
		t.Fatal(err)
	}
	if env.App != "wol-agent "+version {
		t.Errorf("programme : %q", env.App)
	}
	for _, want := range []string{
		"Nom « PC Bureau », port 9770, clé présente",
		"démarrage demandé par Téléphone de Léa (192.168.1.30)",
		"- config.json :",
		"- history.json :",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("rapport : %q manquant\n%s", want, report)
		}
	}
	if strings.Contains(report, cfg.Key) {
		t.Error("clé de l'agent dans le rapport")
	}
}

func TestReadTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.log")
	var content strings.Builder
	for i := 0; i < 100; i++ {
		content.WriteString("ligne numéro ")
		content.WriteString(strings.Repeat("x", i%7))
		content.WriteString("\n")
	}
	_ = os.WriteFile(path, []byte(content.String()), 0o600)
	tail, err := readTail(path, 100)
	if err != nil || len(tail) > 100 || !strings.HasPrefix(string(tail), "ligne numéro") || !strings.HasSuffix(string(tail), "\n") {
		t.Errorf("fin du journal : %q %v", tail, err)
	}
	all, _ := readTail(path, 1<<20)
	if string(all) != content.String() {
		t.Error("journal court non repris en entier")
	}
}

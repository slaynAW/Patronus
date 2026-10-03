package app

import (
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol"
	"github.com/slaynaw/wakeonlan/desktop/internal/agentclient/agenttest"
	"github.com/slaynaw/wakeonlan/desktop/internal/archive"
	"github.com/slaynaw/wakeonlan/desktop/internal/model"
)

func fp(v float64) *float64 { return &v }

func minute(day string, m int64) int64 {
	start, _ := archive.DayStart(day)
	return start + m*60
}

// Archives : l'agent garde les mesures, l'application les range sur GitHub (un Gist chiffré par
// mois), puis les relit quand le PC est éteint.
func TestArchiveMetricsEndToEnd(t *testing.T) {
	gists, srv := newGistServer(t)
	clock := &testClock{now: time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)}
	keyText, _ := protocol.NewKey()
	agent, err := agenttest.Start(model.DecodeAgentKey(keyText), agenttest.Normal)
	if err != nil {
		t.Fatal(err)
	}
	defer agent.Close()
	agent.SetMetrics(map[string][]protocol.MetricsRow{
		"2026-09-30": {{T: minute("2026-09-30", 600), N: 6, CPUTemp: fp(50), CPUTempMax: fp(52), CPULoad: fp(10), CPULoadMax: fp(20)}},
		"2026-10-03": {
			{T: minute("2026-10-03", 0), N: 6, CPUTemp: fp(60), CPUTempMax: fp(70), GPULoad: fp(90), GPULoadMax: fp(100)},
			{T: minute("2026-10-03", 1), N: 6, CPUTemp: fp(62), CPUTempMax: fp(64), GPULoad: fp(80), GPULoadMax: fp(85)},
		},
	})
	agent.SetHistory(&protocol.History{From: minute("2026-09-29", 0), Events: []protocol.HistoryEvent{
		{T: minute("2026-09-30", 500), K: protocol.HistoryBoot},
		{T: minute("2026-10-03", 2), K: protocol.HistoryCommand, A: "shutdown", C: "192.168.1.30", B: "Pixel 8"},
	}})
	d := testDevice(t, "dev-1", "PC Bureau", "127.0.0.1", keyText)
	d.Agent.Port = agent.Port()
	s, _ := newBackupService(t, srv, t.TempDir(), clock, d)

	// Prérequis : sauvegarde automatique (mot de passe) et GitHub.
	if _, err := callJSON(t, s, "archiveEnable", `{"enabled":true}`); err == nil {
		t.Fatal("archives activées sans sauvegarde automatique")
	}
	if _, err := callJSON(t, s, "backupEnable", `{"password":"`+backupPassword+`"}`); err != nil {
		t.Fatal(err)
	}
	if _, err := callJSON(t, s, "archiveEnable", `{"enabled":true}`); err == nil {
		t.Fatal("archives activées sans GitHub")
	}
	if _, err := callJSON(t, s, "backupConnect", `{}`); err != nil {
		t.Fatal(err)
	}
	waitBackupGitHub(t, s)
	if _, err := callJSON(t, s, "archiveEnable", `{"enabled":true}`); err != nil {
		t.Fatal(err)
	}
	if v := s.State().Backup.Archive; v == nil || !v.Enabled {
		t.Fatalf("état : %+v", v)
	}

	// Premier archivage : deux mois, donc deux Gists chiffrés.
	r, err := callJSON(t, s, "archiveNow", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if r["pcs"] != 1.0 || r["minutes"] != 3.0 || r["files"] != 4.0 {
		t.Fatalf("archivage : %v", r)
	}
	monthGist := func(month string) map[string]string {
		gists.mu.Lock()
		defer gists.mu.Unlock()
		for id, desc := range gists.descriptions {
			if desc == archive.Description(month) {
				out := map[string]string{}
				for k, v := range gists.files[id] {
					out[k] = v
				}
				return out
			}
		}
		t.Fatalf("Gist %s absent", month)
		return nil
	}
	october := monthGist("2026-10")
	manifest, err := archive.ParseManifest(october[archive.ManifestFile])
	if err != nil {
		t.Fatal(err)
	}
	key, err := manifest.Unlock(backupPassword)
	if err != nil {
		t.Fatal(err)
	}
	var day archive.Day
	if err := key.Open(archive.DayFile("2026-10-03"), october[archive.DayFile("2026-10-03")], &day); err != nil {
		t.Fatal(err)
	}
	if rows := day.Rows(d.MAC.String()); len(rows) != 2 || *rows[1].CPUTemp != 62 || day.PCs[0].Name != "PC Bureau" {
		t.Fatalf("mesures archivées : %+v", day)
	}
	var journal archive.Journal
	if err := key.Open(archive.JournalFile("2026-10"), october[archive.JournalFile("2026-10")], &journal); err != nil {
		t.Fatal(err)
	}
	if events := journal.Events(d.MAC.String()); len(events) != 1 || events[0].B != "Pixel 8" {
		t.Fatalf("journal archivé : %+v", journal)
	}
	if _, ok := monthGist("2026-09")[archive.DayFile("2026-09-30")]; !ok {
		t.Error("septembre non archivé")
	}

	// Rien de nouveau : aucun fichier réécrit ; une minute de plus : seul son jour l'est.
	if r, _ := callJSON(t, s, "archiveNow", `{}`); r["files"] != 0.0 {
		t.Errorf("archivage sans nouveauté : %v", r)
	}
	agent.SetMetrics(map[string][]protocol.MetricsRow{
		"2026-10-03": {
			{T: minute("2026-10-03", 1), N: 6, CPUTemp: fp(62), CPUTempMax: fp(64), GPULoad: fp(80), GPULoadMax: fp(85)},
			{T: minute("2026-10-03", 2), N: 6, CPUTemp: fp(58)},
		},
	})
	if r, _ := callJSON(t, s, "archiveNow", `{}`); r["files"] != 1.0 || r["minutes"] != 1.0 {
		t.Errorf("archivage d'une minute : %v", r)
	}

	// Écran « Mesures », PC allumé : lu sur l'agent.
	from := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC).UnixMilli()
	to := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC).UnixMilli()
	view, err := s.metricsRange(d.ID, from, to, 1000)
	if err != nil || !view.Agent || view.Minutes != 3 || *view.Summary.CPUTempMax != 64 {
		t.Fatalf("mesures (agent) : %+v, %v", view, err)
	}

	// Autre appareil, PC éteint : relu dans les archives (même compte, même mot de passe).
	agent.Close()
	other, _ := newBackupService(t, srv, t.TempDir(), clock, d)
	if _, err := callJSON(t, other, "backupEnable", `{"password":"`+backupPassword+`"}`); err != nil {
		t.Fatal(err)
	}
	if _, err := callJSON(t, other, "backupConnect", `{}`); err != nil {
		t.Fatal(err)
	}
	waitBackupGitHub(t, other)
	view, err = other.metricsRange(d.ID, from, to, 1000)
	if err != nil || !view.Archive || view.Agent || view.Minutes != 4 || len(view.Points) != 2 {
		t.Fatalf("mesures (archives) : %+v, %v", view, err)
	}
	if view.Summary.GPULoadMax == nil || *view.Summary.GPULoadMax != 100 {
		t.Errorf("résumé : %+v", view.Summary)
	}
	months, err := callJSON(t, other, "metricsMonths", `{}`)
	if err != nil || len(months["months"].([]any)) != 2 || months["months"].([]any)[0] != "2026-10" {
		t.Errorf("mois archivés : %v, %v", months, err)
	}
	events, err := other.metricsJournal(d.ID, from, to)
	if err != nil {
		t.Fatal(err)
	}
	items := events.(map[string]any)["events"].([]HistoryItem)
	if len(items) != 2 || items[0].Client != "Pixel 8" {
		t.Errorf("journal (archives) : %+v", items)
	}

	// Mauvais mot de passe : archives illisibles, rien n'est inventé.
	wrong, _ := newBackupService(t, srv, t.TempDir(), clock, d)
	_, _ = callJSON(t, wrong, "backupEnable", `{"password":"un tout autre mot de passe"}`)
	_, _ = callJSON(t, wrong, "backupConnect", `{}`)
	waitBackupGitHub(t, wrong)
	if view, err := wrong.metricsRange(d.ID, from, to, 1000); err != nil || view.Minutes != 0 || view.Archive {
		t.Errorf("autre mot de passe : %+v, %v", view, err)
	}
}

func TestDownsample(t *testing.T) {
	from := minute("2026-10-03", 0)
	rows := []protocol.MetricsRow{
		{T: from, N: 6, CPUTemp: fp(50), CPUTempMax: fp(55)},
		{T: from + 60, N: 2, CPUTemp: fp(70), CPUTempMax: fp(80)},
		{T: from + 600, N: 6, CPULoad: fp(10)},
	}
	step, points, summary, minutes := downsample(rows, from, from+3600, 6)
	if step != 600 || len(points) != 2 || minutes != 3 {
		t.Fatalf("regroupement : pas %d, %d points, %d minutes", step, len(points), minutes)
	}
	// Moyenne pondérée par le nombre de relevés : (50×6 + 70×2) / 8 = 55 ; maximum des maximums : 80.
	if *points[0].CPUTemp != 55 || *points[0].CPUTempMax != 80 || points[0].CPULoad != nil {
		t.Errorf("premier point : %+v", points[0])
	}
	if *summary.CPUTempMax != 80 || *summary.CPULoad != 10 {
		t.Errorf("résumé : %+v", summary)
	}
}

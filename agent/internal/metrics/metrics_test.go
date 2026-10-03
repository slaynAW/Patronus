package metrics

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol"
)

func f(v float64) *float64 { return &v }

func value(p *float64) float64 {
	if p == nil {
		return -1
	}
	return *p
}

func TestMinuteSummary(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 3, 12, 0, 5, 0, time.UTC)
	r := New(dir, nil, func() time.Time { return now })
	// Minute 12:00 : trois relevés ; la carte graphique n'est lue qu'une fois.
	r.Add(now, protocol.Temperatures{CPU: f(50), CPULoad: f(10)})
	r.Add(now.Add(10*time.Second), protocol.Temperatures{CPU: f(54), CPULoad: f(30), GPU: f(61), GPULoad: f(80)})
	r.Add(now.Add(20*time.Second), protocol.Temperatures{CPU: f(55), CPULoad: f(20)})
	// Rien encore : la minute n'est pas finie.
	if rows, _ := r.Day("2026-10-03"); len(rows) != 0 {
		t.Fatalf("minute enregistrée trop tôt : %+v", rows)
	}
	// Minute suivante : la précédente est enregistrée.
	r.Add(now.Add(time.Minute), protocol.Temperatures{CPU: f(56)})
	rows, err := r.Day("2026-10-03")
	if err != nil || len(rows) != 1 {
		t.Fatalf("lignes : %+v, %v", rows, err)
	}
	row := rows[0]
	if row.T != time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC).Unix() || row.N != 3 {
		t.Errorf("minute : %+v", row)
	}
	if value(row.CPUTemp) != 53 || value(row.CPUTempMax) != 55 || value(row.CPULoad) != 20 || value(row.CPULoadMax) != 30 {
		t.Errorf("processeur : %v %v %v %v", value(row.CPUTemp), value(row.CPUTempMax), value(row.CPULoad), value(row.CPULoadMax))
	}
	if value(row.GPUTemp) != 61 || value(row.GPUTempMax) != 61 || value(row.GPULoad) != 80 {
		t.Errorf("carte graphique : %v %v %v", value(row.GPUTemp), value(row.GPUTempMax), value(row.GPULoad))
	}
	// Arrêt de l'agent : la minute en cours est enregistrée.
	r.Flush()
	if rows, _ := r.Day("2026-10-03"); len(rows) != 2 || value(rows[1].CPUTemp) != 56 {
		t.Errorf("après arrêt : %+v", rows)
	}
	// Minute sans aucune mesure lisible : pas de ligne.
	r.Add(now.Add(2*time.Minute), protocol.Temperatures{})
	r.Flush()
	if rows, _ := r.Day("2026-10-03"); len(rows) != 2 {
		t.Errorf("minute vide enregistrée : %d lignes", len(rows))
	}
}

func TestDaysCompressionAndRetention(t *testing.T) {
	dir := t.TempDir()
	start := time.Date(2026, 10, 1, 23, 59, 30, 0, time.UTC)
	now := start
	r := New(dir, nil, func() time.Time { return now })
	r.Add(now, protocol.Temperatures{CPU: f(40)})
	// Passage au jour suivant : le jour terminé est compressé.
	now = start.Add(time.Minute)
	r.Add(now, protocol.Temperatures{CPU: f(41)})
	now = now.Add(time.Minute)
	r.Add(now, protocol.Temperatures{CPU: f(42)})
	if _, err := os.Stat(filepath.Join(dir, "2026-10-01.jsonl.gz")); err != nil {
		t.Fatalf("jour terminé non compressé : %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "2026-10-01.jsonl")); !os.IsNotExist(err) {
		t.Error("jour terminé laissé en clair")
	}
	if rows, err := r.Day("2026-10-01"); err != nil || len(rows) != 1 || value(rows[0].CPUTemp) != 40 {
		t.Errorf("jour compressé : %+v, %v", rows, err)
	}
	if days := r.Days(); !slices.Equal(days, []string{"2026-10-01", "2026-10-02"}) {
		t.Errorf("jours : %v", days)
	}
	// Jour trop ancien : supprimé au démarrage suivant.
	old := filepath.Join(dir, "2026-06-01.jsonl.gz")
	if err := os.WriteFile(old, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	r.tidy(now)
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Error("jour de plus de 90 jours gardé")
	}
	// Jour inconnu ou invalide.
	if rows, err := r.Day("2026-09-01"); err != nil || len(rows) != 0 {
		t.Errorf("jour absent : %+v, %v", rows, err)
	}
	if _, err := r.Day("../config"); err == nil {
		t.Error("jour invalide accepté")
	}
}

func TestDamagedAndDuplicateLines(t *testing.T) {
	dir := t.TempDir()
	content := `{"t":1759492800,"n":2,"ct":50}
{"t":1759492860,"n":1,"ct":51}
{"t":1759492800,"n":3,"ct":52}
{"t":17594929`
	if err := os.WriteFile(filepath.Join(dir, "2026-10-03.jsonl"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	r := New(dir, nil, time.Now)
	rows, err := r.Day("2026-10-03")
	if err != nil || len(rows) != 2 || value(rows[0].CPUTemp) != 52 || rows[1].T != 1759492860 {
		t.Errorf("lignes : %+v, %v", rows, err)
	}
}

func TestLatestAndInterval(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	r := New(t.TempDir(), nil, func() time.Time { return now })
	if r.Latest() != nil {
		t.Error("relevé avant le premier relevé")
	}
	r.Add(now, protocol.Temperatures{CPU: f(50)})
	if got := r.Latest(); got == nil || value(got.CPU) != 50 {
		t.Errorf("dernier relevé : %+v", got)
	}
	// Une application regarde : relevés plus fréquents, puis retour au rythme normal.
	if r.interval() != ActiveInterval {
		t.Error("rythme rapide attendu")
	}
	now = now.Add(time.Minute)
	if r.interval() != Interval {
		t.Error("rythme normal attendu")
	}
	// Relevé trop ancien : plus servi.
	if r.Latest() != nil {
		t.Error("relevé périmé servi")
	}
}

func TestValidDay(t *testing.T) {
	for day, want := range map[string]bool{"2026-10-03": true, "2026-13-01": false, "2026-1-3": false, "": false, "../x": false} {
		if ValidDay(day) != want {
			t.Errorf("%q", day)
		}
	}
}

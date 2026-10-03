package archive

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/slaynaw/wakeonlan/agent/protocol"
)

type vectors struct {
	Password      string            `json:"password"`
	WrongPassword string            `json:"wrongPassword"`
	Manifest      json.RawMessage   `json:"manifest"`
	Files         map[string]string `json:"files"`
	Day           json.RawMessage   `json:"day"`
	Journal       json.RawMessage   `json:"journal"`
}

func loadVectors(t *testing.T) vectors {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "protocol", "archive-vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var v vectors
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// sameJSON compare deux contenus après relecture (indépendamment de la mise en forme des nombres).
func sameJSON(t *testing.T, got any, want json.RawMessage) {
	t.Helper()
	raw, _ := json.Marshal(got)
	var a, b any
	_ = json.Unmarshal(raw, &a)
	_ = json.Unmarshal(want, &b)
	if !reflect.DeepEqual(a, b) {
		t.Errorf("contenu :\n%s\nattendu :\n%s", raw, want)
	}
}

// Archives produites par une autre implémentation (Node.js) : relues à l'identique.
func TestReadReferenceArchive(t *testing.T) {
	v := loadVectors(t)
	m, err := ParseManifest(string(v.Manifest))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Unlock(v.WrongPassword); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("mauvais mot de passe : %v", err)
	}
	key, err := m.Unlock(v.Password)
	if err != nil {
		t.Fatal(err)
	}
	var day Day
	if err := key.Open(DayFile("2026-10-03"), v.Files["mesures-2026-10-03.txt"], &day); err != nil {
		t.Fatal(err)
	}
	sameJSON(t, day, v.Day)
	var journal Journal
	if err := key.Open(JournalFile("2026-10"), v.Files["journal-2026-10.txt"], &journal); err != nil {
		t.Fatal(err)
	}
	sameJSON(t, journal, v.Journal)
	// Fichier présenté sous un autre nom (remplacé par un autre jour) : refusé.
	if err := key.Open(DayFile("2026-10-04"), v.Files["mesures-2026-10-03.txt"], &day); err == nil {
		t.Error("fichier renommé accepté")
	}
	// Contenu modifié : refusé.
	tampered := []byte(v.Files["journal-2026-10.txt"])
	tampered[40] ^= 1
	if err := key.Open(JournalFile("2026-10"), string(tampered), &journal); err == nil {
		t.Error("fichier modifié accepté")
	}
}

func TestRoundTrip(t *testing.T) {
	m, key, err := NewManifest("2026-11", "mot de passe très sûr")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseManifest(m.JSON())
	if err != nil || parsed != m {
		t.Fatalf("manifeste : %+v, %v", parsed, err)
	}
	again, err := parsed.Unlock("mot de passe très sûr")
	if err != nil {
		t.Fatal(err)
	}
	v := 50.5
	day := Day{Day: "2026-11-02", PCs: []DayPC{{MAC: "AA:BB:CC:DD:EE:01", Name: "PC", Rows: []protocol.MetricsRow{{T: 1793577600, N: 6, CPUTemp: &v}}}}}
	text, err := key.Seal(DayFile(day.Day), day)
	if err != nil {
		t.Fatal(err)
	}
	var back Day
	if err := again.Open(DayFile(day.Day), text, &back); err != nil || *back.PCs[0].Rows[0].CPUTemp != 50.5 {
		t.Fatalf("relu : %+v, %v", back, err)
	}
	if _, _, err := NewManifest("octobre", "x"); err == nil {
		t.Error("mois invalide accepté")
	}
}

func TestParseManifestRejects(t *testing.T) {
	for _, text := range []string{
		`{"format":"autre","version":1,"month":"2026-10","kdf":"PBKDF2WithHmacSHA256","iterations":600000}`,
		`{"format":"patronus-archive","version":2,"month":"2026-10","kdf":"PBKDF2WithHmacSHA256","iterations":600000}`,
		`{"format":"patronus-archive","version":1,"month":"2026-10","kdf":"PBKDF2WithHmacSHA256","iterations":10}`,
		`{"format":"patronus-archive","version":1,"month":"x","kdf":"PBKDF2WithHmacSHA256","iterations":600000}`,
		`pas du JSON`,
	} {
		if _, err := ParseManifest(text); err == nil {
			t.Errorf("manifeste accepté : %s", text)
		}
	}
}

func f(v float64) *float64 { return &v }

func TestAddRows(t *testing.T) {
	day := Day{Day: "2026-10-03"}
	start, _ := DayStart("2026-10-03")
	rows := []protocol.MetricsRow{
		{T: start + 60, N: 6, CPUTemp: f(50)},
		{T: start, N: 6, CPUTemp: f(49)},
		{T: start - 60, N: 6, CPUTemp: f(40)},    // veille : ignorée
		{T: start + 86400, N: 6, CPUTemp: f(40)}, // lendemain : ignorée
	}
	day, changed := day.AddRows("AA:BB:CC:DD:EE:01", "PC Bureau", rows)
	if !changed || len(day.Rows("AA:BB:CC:DD:EE:01")) != 2 || day.Rows("AA:BB:CC:DD:EE:01")[0].T != start {
		t.Fatalf("ajout : %+v", day)
	}
	// Mêmes mesures envoyées par un autre appareil : rien ne change.
	if _, changed := day.AddRows("AA:BB:CC:DD:EE:01", "PC Bureau", rows[:2]); changed {
		t.Error("doublon compté comme changement")
	}
	// Minute complétée (agent relu plus tard) : remplacée ; autre PC : ajouté à part.
	day, changed = day.AddRows("AA:BB:CC:DD:EE:01", "PC Bureau", []protocol.MetricsRow{{T: start + 60, N: 6, CPUTemp: f(50), GPUTemp: f(45)}})
	if !changed || day.Rows("AA:BB:CC:DD:EE:01")[1].GPUTemp == nil {
		t.Error("minute complétée non remplacée")
	}
	day, _ = day.AddRows("AA:BB:CC:DD:EE:02", "PC Salon", []protocol.MetricsRow{{T: start, N: 1, GPULoad: f(90)}})
	if len(day.PCs) != 2 || len(day.Rows("AA:BB:CC:DD:EE:02")) != 1 {
		t.Errorf("deux PC : %+v", day.PCs)
	}
}

func TestAddEvents(t *testing.T) {
	j := Journal{Month: "2026-10"}
	events := []protocol.HistoryEvent{
		{T: 1791025260, K: "shutdown"},
		{T: 1790982000, K: "boot"},
		{T: 1790640000, K: "boot"}, // septembre : ignoré
	}
	j, changed := j.AddEvents("AA:BB:CC:DD:EE:01", "PC Bureau", events)
	got := j.Events("AA:BB:CC:DD:EE:01")
	if !changed || len(got) != 2 || got[0].K != "boot" {
		t.Fatalf("journal : %+v", got)
	}
	if _, changed := j.AddEvents("AA:BB:CC:DD:EE:01", "PC Bureau", events); changed {
		t.Error("doublons comptés comme changement")
	}
}

func TestNames(t *testing.T) {
	if MonthOf(Description("2026-10")) != "2026-10" || MonthOf("Patronus – sauvegardes chiffrées") != "" {
		t.Error("description")
	}
	if DayOf(DayFile("2026-10-03")) != "2026-10-03" || DayOf(JournalFile("2026-10")) != "" || DayOf(ManifestFile) != "" {
		t.Error("nom de fichier")
	}
}

// Changement de mot de passe : même contenu, nouveau sel ; l'ancien mot de passe n'ouvre plus rien.
func TestReencrypt(t *testing.T) {
	m, key, err := NewManifest("2026-10", "ancien-mot-de-passe")
	if err != nil {
		t.Fatal(err)
	}
	day := Day{Day: "2026-10-03", PCs: []DayPC{{MAC: "AA:BB:CC:DD:EE:FF", Name: "Bureau", Rows: []protocol.MetricsRow{{T: 1790985600, N: 6}}}}}
	journal := Journal{Month: "2026-10"}
	dayText, _ := key.Seal(DayFile(day.Day), day)
	journalText, _ := key.Seal(JournalFile("2026-10"), journal)
	files := map[string]string{ManifestFile: m.JSON(), ReadmeFile: Readme, DayFile(day.Day): dayText,
		JournalFile("2026-10"): journalText, "mesures-2026-10-04.txt": "abîmé"}

	if _, _, err := Reencrypt(files, "mauvais-mot-de-passe", "nouveau-mot-de-passe"); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("mauvais mot de passe accepté : %v", err)
	}
	out, unreadable, err := Reencrypt(files, "ancien-mot-de-passe", "nouveau-mot-de-passe")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != len(files) || !reflect.DeepEqual(unreadable, []string{"mesures-2026-10-04.txt"}) || out["mesures-2026-10-04.txt"] != "abîmé" {
		t.Fatalf("fichiers : %d %v", len(out), unreadable)
	}
	next, err := ParseManifest(out[ManifestFile])
	if err != nil || next.Month != "2026-10" || next.Salt == m.Salt {
		t.Fatalf("manifeste : %+v %v", next, err)
	}
	if _, err := next.Unlock("ancien-mot-de-passe"); !errors.Is(err, ErrWrongPassword) {
		t.Error("l'ancien mot de passe ouvre encore le mois")
	}
	newKey, err := next.Unlock("nouveau-mot-de-passe")
	if err != nil {
		t.Fatal(err)
	}
	var gotDay Day
	if err := newKey.Open(DayFile(day.Day), out[DayFile(day.Day)], &gotDay); err != nil || !reflect.DeepEqual(gotDay, day) {
		t.Errorf("mesures : %+v %v", gotDay, err)
	}
	var gotJournal Journal
	if err := newKey.Open(JournalFile("2026-10"), out[JournalFile("2026-10")], &gotJournal); err != nil || gotJournal.Month != "2026-10" {
		t.Errorf("journal : %+v %v", gotJournal, err)
	}
}

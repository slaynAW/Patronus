package diag

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixedClock() func() time.Time {
	t := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	return func() time.Time { return t }
}

func TestWriteAndRecords(t *testing.T) {
	dir := t.TempDir()
	j, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	j.Now = fixedClock()
	var mirror bytes.Buffer
	j.Mirror = &mirror
	j.Write(LevelInfo, "partage", "publication : 2 fichiers")
	j.Write(LevelError, "interface", "erreur\navec trace")

	got := j.Records()
	want := []string{
		"2026-09-29T10:00:00.000Z I partage : publication : 2 fichiers",
		"2026-09-29T10:00:00.000Z E interface : erreur\navec trace",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("évènements : %q", got)
	}
	if !strings.Contains(mirror.String(), "publication : 2 fichiers") {
		t.Errorf("copie en clair (développement) : %q", mirror.String())
	}

	// Rien en clair sur le disque.
	raw, _ := os.ReadFile(filepath.Join(dir, fileCurrent))
	if bytes.Contains(raw, []byte("publication")) || bytes.Contains(raw, []byte("partage")) {
		t.Error("évènement écrit en clair")
	}

	// Relu après réouverture (même clé).
	j2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := j2.Records(); len(got) != 2 || got[0] != want[0] {
		t.Errorf("après réouverture : %q", got)
	}

	// Ligne abîmée : signalée, les autres restent lisibles.
	f, _ := os.OpenFile(filepath.Join(dir, fileCurrent), os.O_APPEND|os.O_WRONLY, 0o600)
	_, _ = f.WriteString("AAAA\n")
	_ = f.Close()
	if got := j2.Records(); len(got) != 3 || !strings.HasPrefix(got[2], "[ligne illisible") {
		t.Errorf("ligne abîmée : %q", got)
	}
}

func TestLostKeyStartsOver(t *testing.T) {
	dir := t.TempDir()
	j, _ := Open(dir)
	j.Write(LevelInfo, "appli", "avant")
	_ = os.WriteFile(filepath.Join(dir, keyFile), []byte("abîmée"), 0o600)
	j2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := j2.Records(); len(got) != 0 {
		t.Errorf("anciens évènements conservés sans leur clé : %q", got)
	}
	j2.Write(LevelInfo, "appli", "après")
	if got := j2.Records(); len(got) != 1 || !strings.HasSuffix(got[0], "après") {
		t.Errorf("nouveau journal : %q", got)
	}
}

func TestRotationAndLimits(t *testing.T) {
	dir := t.TempDir()
	j, _ := Open(dir)
	long := strings.Repeat("é", maxRecordBytes)
	j.Write(LevelWarn, "test", long)
	got := j.Records()
	if len(got) != 1 || len(got[0]) > maxRecordBytes+len("…") || !strings.HasSuffix(got[0], "…") || !strings.Contains(got[0], "éé") {
		t.Fatalf("évènement trop long mal coupé (%d octets)", len(got[0]))
	}
	for !fileExists(filepath.Join(dir, filePrevious)) {
		j.Write(LevelInfo, "test", strings.Repeat("x", 10_000))
	}
	for i := 0; i < 5; i++ {
		j.Write(LevelInfo, "test", "après rotation")
	}
	for _, name := range []string{fileCurrent, filePrevious} {
		if info, err := os.Stat(filepath.Join(dir, name)); err == nil && info.Size() > maxFileBytes+maxRecordBytes*2 {
			t.Errorf("%s : %d octets", name, info.Size())
		}
	}
	got = j.Records()
	if !strings.HasSuffix(got[len(got)-1], "après rotation") {
		t.Errorf("dernier évènement : %q", got[len(got)-1])
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestCrashCapture(t *testing.T) {
	dir := t.TempDir()
	j, _ := Open(dir)
	j.Now = fixedClock()
	_ = os.WriteFile(filepath.Join(dir, crashFile), []byte("panic: boum\n\ngoroutine 1 [running]:\nmain.main()\n"), 0o600)
	if err := j.CaptureCrashes(); err != nil {
		t.Fatal(err)
	}
	got := j.Records()
	if len(got) != 1 || !strings.Contains(got[0], " E plantage : arrêt brutal") || !strings.Contains(got[0], "panic: boum") {
		t.Fatalf("plantage précédent : %q", got)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, crashFile)); len(data) != 0 {
		t.Errorf("rapport de plantage non vidé : %q", data)
	}
	// Lancement suivant sans plantage : rien de plus.
	if err := j.CaptureCrashes(); err != nil {
		t.Fatal(err)
	}
	if got := j.Records(); len(got) != 1 {
		t.Errorf("plantage compté deux fois : %q", got)
	}
}

func TestDefaultAndLogWriter(t *testing.T) {
	SetDefault(nil)
	Info("test", "ignoré %d", 1) // aucun journal : sans effet
	j, _ := Open(t.TempDir())
	SetDefault(j)
	t.Cleanup(func() { SetDefault(nil) })
	Warn("réseau", "adresse %s injoignable", "192.168.1.20")
	logger := log.New(Writer(LevelInfo, "go"), "", 0)
	logger.Printf("message du paquet log")
	got := Records()
	if len(got) != 2 || !strings.HasSuffix(got[0], " W réseau : adresse 192.168.1.20 injoignable") ||
		!strings.HasSuffix(got[1], " I go : message du paquet log") {
		t.Errorf("évènements : %q", got)
	}
}

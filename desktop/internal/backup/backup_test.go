package backup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNamesAndVersions(t *testing.T) {
	if got := Slug("PC Bureau d'Hélène – Étage"); got != "pc-bureau-d-helene-etage" {
		t.Errorf("slug : %q", got)
	}
	// Identifiant anonyme : type de l'appareil et 8 caractères aléatoires, jamais son nom.
	id := DeviceID("windows")
	if !strings.HasPrefix(id, "windows-") || len(id) != len("windows-")+8 || !Anonymous(id) || id == DeviceID("windows") {
		t.Errorf("identifiant : %q", id)
	}
	for _, legacy := range []string{"windows-bureau-3fa2", "android-pixel-8-3fa2", "windows-3fa2", "windows-a1b2c3d4-3fa2"} {
		if Anonymous(legacy) {
			t.Errorf("ancien identifiant pris pour anonyme : %q", legacy)
		}
	}
	if got, ok := Renamed("patronus-windows-bureau-3fa2-2026-09-30.json", "windows-bureau-3fa2", "windows-0a1b2c3d"); !ok || got != "patronus-windows-0a1b2c3d-2026-09-30.json" {
		t.Errorf("renommage : %q %v", got, ok)
	}
	if _, ok := Renamed("patronus-android-pixel-1111-2026-09-30.json", "windows-bureau-3fa2", "windows-0a1b2c3d"); ok {
		t.Error("fichier d'un autre appareil renommé")
	}
	day := time.Date(2026, 9, 30, 15, 0, 0, 0, time.Local)
	name := FileName("windows-bureau-3fa2", day)
	if name != "patronus-windows-bureau-3fa2-2026-09-30.json" {
		t.Fatalf("nom : %q", name)
	}
	e, ok := Parse(name)
	if !ok || e.Device != "windows-bureau-3fa2" || e.Date != "2026-09-30" {
		t.Errorf("lecture du nom : %+v %v", e, ok)
	}
	for _, bad := range []string{"LISEZMOI.md", "patronus-x-2026-13-40.json", "patronus--2026-09-30.json", "acces-abc.json"} {
		if _, ok := Parse(bad); ok {
			t.Errorf("nom accepté : %q", bad)
		}
	}

	var names []string
	for i := 0; i < 10; i++ {
		names = append(names, FileName("a-1111", day.AddDate(0, 0, -i)))
	}
	names = append(names, FileName("b-2222", day.AddDate(0, 0, -30)), "LISEZMOI.md")
	old := Outdated(names, "a-1111", Keep)
	if len(old) != 3 || old[0] != FileName("a-1111", day.AddDate(0, 0, -7)) {
		t.Errorf("versions à supprimer : %v", old)
	}
	sorted := Sorted(names)
	if len(sorted) != 11 || sorted[0].Date != "2026-09-30" || sorted[10].Device != "b-2222" {
		t.Errorf("tri : %+v", sorted)
	}
}

func TestStoreAndFolder(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	st, err := s.Load()
	if err != nil || st.Enabled {
		t.Fatalf("réglages initiaux : %+v %v", st, err)
	}
	st = Settings{Enabled: true, Password: "motdepasse", Device: "windows-pc-0001", GitHub: &GitHub{Token: "gho_x", User: "hugo", Gist: "abc"}, Folder: dir}
	if err := s.Save(st); err != nil {
		t.Fatal(err)
	}
	back, err := s.Load()
	if err != nil || back.Password != "motdepasse" || back.GitHub.User != "hugo" || back.Folder != dir {
		t.Errorf("relecture : %+v %v", back, err)
	}
	_ = os.WriteFile(filepath.Join(dir, "backup.dat"), []byte("abîmé"), 0o600)
	if _, err := s.Load(); err == nil || !strings.Contains(err.Error(), "mis de côté") {
		t.Errorf("fichier abîmé : %v", err)
	}

	folder := t.TempDir()
	day := time.Date(2026, 9, 30, 12, 0, 0, 0, time.Local)
	for i := 0; i < Keep+2; i++ {
		name := FileName("windows-pc-0001", day.AddDate(0, 0, -(Keep+1)+i))
		if _, err := WriteFolder(folder, "windows-pc-0001", name, []byte("{}")); err != nil {
			t.Fatal(err)
		}
	}
	_ = os.WriteFile(filepath.Join(folder, FileName("android-pixel-0002", day.AddDate(0, 0, -60))), []byte("{}"), 0o600)
	entries, _ := os.ReadDir(folder)
	if len(entries) != Keep+1 {
		t.Errorf("fichiers conservés : %d (%d attendus : %d de ce PC + celui du téléphone)", len(entries), Keep+1, Keep)
	}
	if _, err := WriteFolder(filepath.Join(folder, "absent"), "x-1", "patronus-x-1-2026-09-30.json", nil); err == nil {
		t.Error("dossier absent accepté")
	}
}

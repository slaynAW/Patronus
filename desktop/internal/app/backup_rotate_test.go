package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol"
	"github.com/slaynaw/wakeonlan/desktop/internal/archive"
	"github.com/slaynaw/wakeonlan/desktop/internal/backup"
	"github.com/slaynaw/wakeonlan/desktop/internal/config"
)

const newBackupPassword = "nouveau mot de passe des sauvegardes"

// backupGistOf renvoie le Gist des sauvegardes d'un service.
func backupGistOf(s *Service) string {
	s.backups.mu.Lock()
	defer s.backups.mu.Unlock()
	if s.backups.st.GitHub == nil {
		return ""
	}
	return s.backups.st.GitHub.Gist
}

// gistContent copie les fichiers d'un Gist du faux GitHub (nil s'il n'existe pas).
func gistContent(g *gistServer, id string) map[string]string {
	g.mu.Lock()
	defer g.mu.Unlock()
	files, ok := g.files[id]
	if !ok {
		return nil
	}
	out := map[string]string{}
	for k, v := range files {
		out[k] = v
	}
	return out
}

// gistsDescribed renvoie les Gists du faux GitHub portant cette description.
func gistsDescribed(g *gistServer, description string) []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	var ids []string
	for id := range g.files {
		if g.descriptions[id] == description {
			ids = append(ids, id)
		}
	}
	return ids
}

// waitRotation attend la fin du changement de mot de passe.
func waitRotation(t *testing.T, s *Service) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		st := s.State().Backup
		if st.Rotation == nil {
			return
		}
		if st.Rotation.Error != "" && !st.Rotation.Running {
			t.Fatalf("changement du mot de passe en échec : %s", st.Rotation.Error)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("changement du mot de passe non terminé : %+v", s.State().Backup.Rotation)
}

func TestBackupDeviceRenameAndPasswordChange(t *testing.T) {
	gists, srv := newGistServer(t)
	clock := &testClock{now: time.Date(2026, 10, 3, 10, 0, 0, 0, time.Local)}
	key := "kY5jjMI6cQpU1gqHSiok1wAPlr6OUVgey66jYkJw5DI"
	pc := testDevice(t, "dev-1", "PC streaming", "192.168.1.20", key)
	a, platformA := newBackupService(t, srv, t.TempDir(), clock, pc)
	other, _ := newBackupService(t, srv, t.TempDir(), clock, pc)
	gh := a.backups.opts.GitHub
	ctx := context.Background()

	// A : sauvegardes d'avant la 1.9.0 (identifiant avec le nom de l'appareil), sur GitHub et dans un dossier.
	call(t, a, "backupEnable", map[string]any{"password": backupPassword})
	call(t, a, "backupConnect", nil)
	waitBackupGitHub(t, a)
	folder := t.TempDir()
	platformA.folder = folder
	call(t, a, "backupFolder", nil)
	const legacy = "windows-bureau-3fa2"
	yesterday := clock.Now().AddDate(0, 0, -1)
	legacyName := backup.FileName(legacy, yesterday)
	text, _, err := a.exportText(backupPassword, yesterday)
	if err != nil {
		t.Fatal(err)
	}
	firstGist := backupGistOf(a)
	gists.mu.Lock()
	gists.files[firstGist][legacyName] = string(text)
	gists.mu.Unlock()
	if err := os.WriteFile(filepath.Join(folder, legacyName), text, 0o600); err != nil {
		t.Fatal(err)
	}
	a.backups.mu.Lock()
	a.backups.st.Device, a.backups.st.Uploaded = legacy, []string{legacyName}
	a.backups.mu.Unlock()

	// Autre appareil, même compte et même mot de passe : sa sauvegarde est dans le même Gist.
	call(t, other, "backupEnable", map[string]any{"password": backupPassword})
	call(t, other, "backupConnect", nil)
	waitBackupGitHub(t, other)
	if backupGistOf(other) != firstGist {
		t.Fatalf("Gist des sauvegardes différent : %s / %s", backupGistOf(other), firstGist)
	}
	if _, err := other.backupNow(ctx, true); err != nil {
		t.Fatal(err)
	}

	// Archives : un mois chiffré par le mot de passe des sauvegardes, un autre par un autre mot de passe.
	m, akey, err := archive.NewManifest("2026-10", backupPassword)
	if err != nil {
		t.Fatal(err)
	}
	day := archive.Day{Day: "2026-10-02", PCs: []archive.DayPC{{MAC: "AA:BB:CC:DD:EE:01", Name: "PC streaming",
		Rows: []protocol.MetricsRow{{T: 1790899200, N: 6}}}}}
	dayText, _ := akey.Seal(archive.DayFile(day.Day), day)
	archiveGist, err := gh.CreateGist(ctx, "tok", archive.Description("2026-10"), map[string]string{
		archive.ManifestFile: m.JSON(), archive.ReadmeFile: archive.Readme, archive.DayFile(day.Day): dayText})
	if err != nil {
		t.Fatal(err)
	}
	m2, _, _ := archive.NewManifest("2026-09", "un autre mot de passe")
	foreignGist, err := gh.CreateGist(ctx, "tok", archive.Description("2026-09"), map[string]string{
		archive.ManifestFile: m2.JSON(), archive.ReadmeFile: archive.Readme})
	if err != nil {
		t.Fatal(err)
	}

	// 1. Première sauvegarde de la 1.9.0 : identifiant anonyme, fichiers renommés, Gist recopié
	// (l'ancien, et l'ancien nom dans son historique, disparaissent).
	res, err := a.backupNow(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	a.backups.mu.Lock()
	device, legacyLeft := a.backups.st.Device, a.backups.st.LegacyDevice
	a.backups.mu.Unlock()
	if !backup.Anonymous(device) || legacyLeft != "" || !strings.Contains(res.GitHub, device) {
		t.Fatalf("identifiant : %q (ancien %q), sauvegarde %q", device, legacyLeft, res.GitHub)
	}
	renamedGist := backupGistOf(a)
	if renamedGist == firstGist || gistContent(gists, firstGist) != nil {
		t.Fatalf("Gist non recopié : %s", renamedGist)
	}
	files := gistContent(gists, renamedGist)
	renamed := backup.FileName(device, yesterday)
	if files[renamed] != string(text) || files[legacyName] != "" || len(files) != 4 { // LISEZMOI, 2 de A, 1 de l'autre
		t.Fatalf("fichiers après renommage : %v", keys(files))
	}
	for name := range files {
		if strings.Contains(name, "bureau") {
			t.Errorf("nom de l'appareil encore visible : %s", name)
		}
	}
	if _, err := os.Stat(filepath.Join(folder, renamed)); err != nil {
		t.Errorf("dossier non renommé : %v", err)
	}
	// L'autre appareil retrouve le Gist recopié.
	if _, err := other.backupNow(ctx, true); err != nil || backupGistOf(other) != renamedGist {
		t.Fatalf("autre appareil : %v (Gist %s)", err, backupGistOf(other))
	}

	// 2. Changement du mot de passe, interrompu une fois (GitHub indisponible), puis repris.
	if err := a.backupChangePassword("faux", newBackupPassword); err == nil {
		t.Error("mot de passe actuel faux accepté")
	}
	if err := a.backupChangePassword(backupPassword, "court"); err == nil {
		t.Error("nouveau mot de passe trop court accepté")
	}
	gists.mu.Lock()
	gists.failDelete = 1
	gists.mu.Unlock()
	a.backups.mu.Lock()
	if err := a.startRotationLocked(backupPassword, newBackupPassword); err != nil {
		t.Fatal(err)
	}
	a.backups.mu.Unlock()
	if err := a.runRotation(ctx); err == nil {
		t.Fatal("interruption non signalée")
	}
	if r := a.State().Backup.Rotation; r == nil || r.Error == "" {
		t.Fatalf("état après interruption : %+v", r)
	}
	if _, err := a.backupNow(ctx, true); err == nil {
		t.Error("sauvegarde pendant le changement du mot de passe")
	}
	if err := a.runRotation(ctx); err != nil {
		t.Fatalf("reprise : %v", err)
	}
	waitRotation(t, a)
	if ids := gistsDescribed(gists, backup.GistDescription); len(ids) != 1 {
		t.Fatalf("Gists des sauvegardes après reprise (un seul attendu) : %v", ids)
	}

	// Sauvegardes : toutes rechiffrées (celle de l'autre appareil comprise), dans un nouveau Gist.
	rotatedGist := backupGistOf(a)
	if rotatedGist == renamedGist || gistContent(gists, renamedGist) != nil {
		t.Fatalf("Gist des sauvegardes non remplacé : %s", rotatedGist)
	}
	files = gistContent(gists, rotatedGist)
	count := 0
	for name, content := range files {
		if _, ok := backup.Parse(name); !ok {
			continue
		}
		count++
		if config.CheckPassword([]byte(content), newBackupPassword) != nil || config.CheckPassword([]byte(content), backupPassword) == nil {
			t.Errorf("%s non rechiffrée", name)
		}
	}
	if count != 3 {
		t.Errorf("sauvegardes après changement : %v", keys(files))
	}
	// Dossier rechiffré.
	names, _ := backup.FolderBackups(folder)
	for _, name := range names {
		content, _ := os.ReadFile(filepath.Join(folder, name))
		if config.CheckPassword(content, newBackupPassword) != nil {
			t.Errorf("dossier : %s non rechiffrée", name)
		}
	}
	// Archives : le mois du mot de passe des sauvegardes est rechiffré (même contenu, nouveau Gist),
	// celui d'un autre mot de passe n'est pas touché.
	if gistContent(gists, archiveGist) != nil {
		t.Error("ancien Gist d'archives toujours là")
	}
	months := gistsDescribed(gists, archive.Description("2026-10"))
	if len(months) != 1 {
		t.Fatalf("Gists d'archives d'octobre : %v", months)
	}
	afiles := gistContent(gists, months[0])
	next, err := archive.ParseManifest(afiles[archive.ManifestFile])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := next.Unlock(backupPassword); !errors.Is(err, archive.ErrWrongPassword) {
		t.Error("l'ancien mot de passe ouvre encore les archives")
	}
	nkey, err := next.Unlock(newBackupPassword)
	if err != nil {
		t.Fatal(err)
	}
	var gotDay archive.Day
	if err := nkey.Open(archive.DayFile(day.Day), afiles[archive.DayFile(day.Day)], &gotDay); err != nil || !reflect.DeepEqual(gotDay, day) {
		t.Errorf("mesures après changement : %+v %v", gotDay, err)
	}
	if gistContent(gists, foreignGist) == nil || gistContent(gists, foreignGist)[archive.ManifestFile] != m2.JSON() {
		t.Error("archives d'un autre mot de passe modifiées")
	}
	if _, err := a.backupNow(ctx, true); err != nil {
		t.Fatalf("sauvegarde après changement : %v", err)
	}

	// 3. L'autre appareil s'en aperçoit : tout s'arrête jusqu'à la saisie du nouveau mot de passe.
	if _, err := other.backupNow(ctx, true); !errors.Is(err, errStalePassword) {
		t.Fatalf("mot de passe périmé non détecté : %v", err)
	}
	if !other.State().Backup.Stale || other.backupDue() {
		t.Error("état « nouveau mot de passe à saisir » absent")
	}
	if err := other.backupUpdatePassword(backupPassword); err == nil {
		t.Error("ancien mot de passe accepté comme nouveau")
	}
	if err := other.backupUpdatePassword(newBackupPassword); err != nil {
		t.Fatal(err)
	}
	waitRotation(t, other)
	if other.State().Backup.Stale {
		t.Error("toujours en attente du nouveau mot de passe")
	}
	res, err = other.backupNow(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if c := gistContent(gists, backupGistOf(other))[res.GitHub]; config.CheckPassword([]byte(c), newBackupPassword) != nil {
		t.Error("sauvegarde de l'autre appareil non chiffrée par le nouveau mot de passe")
	}
}

func keys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

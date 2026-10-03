package app

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/desktop/internal/backup"
	"github.com/slaynaw/wakeonlan/desktop/internal/config"
	"github.com/slaynaw/wakeonlan/desktop/internal/model"
	"github.com/slaynaw/wakeonlan/desktop/internal/netstate"
	"github.com/slaynaw/wakeonlan/desktop/internal/share"
	"github.com/slaynaw/wakeonlan/desktop/internal/status"
)

const backupPassword = "mot de passe des sauvegardes"

// testClock est une horloge réglable.
type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) Add(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

// newBackupService crée un service avec partage et sauvegardes (données dans dir).
func newBackupService(t *testing.T, srv *httptest.Server, dir string, clock *testClock, devices ...model.Device) (*Service, *fakePlatform) {
	t.Helper()
	store, err := config.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) > 0 {
		if err := store.Save(model.AppConfig{SchemaVersion: model.CurrentSchemaVersion, Devices: devices, Settings: model.DefaultSettings()}); err != nil {
			t.Fatal(err)
		}
	}
	shareStore, _ := share.NewStore(dir)
	backupStore, _ := backup.NewStore(dir)
	gh := &share.GitHub{API: srv.URL, Web: srv.URL, ClientID: "client", Client: srv.Client(), PollUnit: time.Millisecond}
	platform := &fakePlatform{}
	s := New(Options{
		Version: "test", Store: store, Platform: platform, Now: clock.Now,
		Prober:   status.ProberFunc(func(context.Context, model.Device) status.ProbeResult { return status.ProbeResult{} }),
		NetState: func() (netstate.State, error) { return netstate.State{}, nil },
		Share:    &ShareOptions{Store: shareStore, GitHub: gh},
		Backup:   &BackupOptions{Store: backupStore, GitHub: gh, Kind: "windows", Name: "Bureau"},
	})
	return s, platform
}

// waitBackupGitHub attend la fin de la connexion GitHub des sauvegardes.
func waitBackupGitHub(t *testing.T, s *Service) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && s.State().Backup.GitHub == nil {
		time.Sleep(10 * time.Millisecond)
	}
	if s.State().Backup.GitHub == nil {
		t.Fatalf("GitHub non connecté : %+v", s.State().Backup)
	}
}

func TestBackupToGitHubAndFolder(t *testing.T) {
	gists, srv := newGistServer(t)
	clock := &testClock{now: time.Date(2026, 9, 30, 10, 0, 0, 0, time.Local)}
	key := "kY5jjMI6cQpU1gqHSiok1wAPlr6OUVgey66jYkJw5DI"
	s, platform := newBackupService(t, srv, t.TempDir(), clock, testDevice(t, "dev-1", "PC streaming", "192.168.1.20", key))

	if st := s.State().Backup; !st.Available || st.Enabled || !st.CanLogin {
		t.Fatalf("état initial : %+v", st)
	}
	if _, err := callJSON(t, s, "backupEnable", `{"password":"court"}`); err == nil {
		t.Error("mot de passe trop court accepté")
	}
	if _, err := callJSON(t, s, "backupEnable", `{"password":"`+backupPassword+`"}`); err != nil {
		t.Fatal(err)
	}
	if _, err := callJSON(t, s, "backupNow", `{}`); err == nil || !strings.Contains(err.Error(), "où sauvegarder") {
		t.Errorf("sauvegarde sans destination : %v", err)
	}

	// GitHub : connexion par code (le partage n'est pas connecté), Gist secret des sauvegardes créé.
	login, err := callJSON(t, s, "backupConnect", `{}`)
	if err != nil || login["code"] != "WXYZ-1234" {
		t.Fatalf("connexion : %v %v", login, err)
	}
	waitBackupGitHub(t, s)
	if st := s.State().Backup; st.GitHub.Label != "@hugo" || st.Login != nil {
		t.Errorf("compte : %+v", st)
	}
	var gistID string
	gists.mu.Lock()
	for id, d := range gists.descriptions {
		if d == backup.GistDescription {
			gistID = id
		}
	}
	gists.mu.Unlock()
	if gistID == "" {
		t.Fatal("Gist des sauvegardes non créé")
	}

	// Dossier.
	folder := t.TempDir()
	platform.folder = folder
	if r, err := callJSON(t, s, "backupFolder", `{}`); err != nil || r["path"] != folder {
		t.Fatalf("dossier : %v %v", r, err)
	}

	res, err := callJSON(t, s, "backupNow", `{}`)
	if err != nil || res["github"] == nil || res["folder"] == nil {
		t.Fatalf("sauvegarde : %v %v", res, err)
	}
	name := res["github"].(string)
	// Nom en clair sur GitHub : type de l'appareil et identifiant aléatoire, jamais son nom (« Bureau »).
	if e, ok := backup.Parse(name); !ok || !backup.Anonymous(e.Device) || !strings.HasPrefix(name, "patronus-windows-") ||
		strings.Contains(name, "bureau") || !strings.HasSuffix(name, "-2026-09-30.json") {
		t.Errorf("nom : %s", name)
	}
	gists.mu.Lock()
	content := gists.files[gistID][name]
	gists.mu.Unlock()
	cfg, extra, err := config.ImportWithExtra([]byte(content), backupPassword)
	if err != nil || len(cfg.Devices) != 1 || cfg.Devices[0].Agent == nil || cfg.Devices[0].Agent.Key != key || extra["history"] == nil {
		t.Fatalf("sauvegarde illisible ou incomplète : %+v %v", cfg, err)
	}
	if strings.Contains(content, "PC streaming") || strings.Contains(content, key) {
		t.Error("sauvegarde en clair")
	}
	if data, err := os.ReadFile(filepath.Join(folder, name)); err != nil || string(data) != content {
		t.Errorf("fichier du dossier : %v", err)
	}
	if st := s.State().Backup; st.GitHub.Last == 0 || st.Folder.Last == 0 || st.Running {
		t.Errorf("dernières sauvegardes : %+v", st)
	}

	// Déclenchement automatique : après un changement (délai), puis une fois par jour.
	if s.backupDue() {
		t.Error("sauvegarde due juste après une sauvegarde")
	}
	if _, err := callJSON(t, s, "updateSettings", `{"pollIntervalSeconds":7}`); err != nil {
		t.Fatal(err)
	}
	if s.backupDue() {
		t.Error("sauvegarde lancée avant la fin du délai")
	}
	clock.Add(backupDelay)
	if !s.backupDue() {
		t.Error("sauvegarde non déclenchée après un changement")
	}

	// Une semaine et demie de sauvegardes quotidiennes : Keep versions gardées de chaque côté.
	for day := 1; day <= 10; day++ {
		clock.Add(backupDaily)
		if !s.backupDue() {
			t.Fatalf("jour %d : sauvegarde quotidienne non due", day)
		}
		if _, err := s.backupNow(context.Background(), false); err != nil {
			t.Fatal(err)
		}
	}
	gists.mu.Lock()
	inGist := len(gists.files[gistID])
	gists.mu.Unlock()
	if inGist != backup.Keep+1 { // + LISEZMOI.md
		t.Errorf("fichiers du Gist : %d (%d attendus)", inGist, backup.Keep+1)
	}
	if entries, _ := os.ReadDir(folder); len(entries) != backup.Keep {
		t.Errorf("fichiers du dossier : %d (%d attendus)", len(entries), backup.Keep)
	}

	// Données perdues : la sauvegarde automatique ne remplace pas la dernière bonne sauvegarde.
	if _, err := callJSON(t, s, "deleteDevice", `{"id":"dev-1"}`); err != nil {
		t.Fatal(err)
	}
	clock.Add(backupDelay)
	if r, err := s.backupNow(context.Background(), false); err != nil || r.Skipped == "" {
		t.Errorf("sauvegarde vide non ignorée : %+v %v", r, err)
	}

	// Désactivation : mot de passe oublié, plus de sauvegarde.
	if _, err := callJSON(t, s, "backupDisable", `{}`); err != nil {
		t.Fatal(err)
	}
	clock.Add(2 * backupDaily)
	if s.backupDue() {
		t.Error("sauvegarde due après désactivation")
	}
}

func TestBackupRestoreOnNewDevice(t *testing.T) {
	_, srv := newGistServer(t)
	clock := &testClock{now: time.Date(2026, 9, 30, 10, 0, 0, 0, time.Local)}
	key := "kY5jjMI6cQpU1gqHSiok1wAPlr6OUVgey66jYkJw5DI"
	pc, _ := newBackupService(t, srv, t.TempDir(), clock,
		testDevice(t, "dev-1", "PC streaming", "192.168.1.20", key), testDevice(t, "dev-2", "Bureau", "192.168.1.21", key))

	// Le partage est connecté : son compte GitHub sert aussi aux sauvegardes, sans nouvelle connexion.
	call(t, pc, "shareLogin", map[string]any{"name": "Hugo"})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && (pc.State().Share.Owner == nil || !pc.State().Share.Owner.Connected) {
		time.Sleep(10 * time.Millisecond)
	}
	if r, err := callJSON(t, pc, "backupConnect", `{}`); err != nil || r["connected"] != true || r["user"] != "hugo" {
		t.Fatalf("compte du partage non repris : %v %v", r, err)
	}
	call(t, pc, "backupEnable", map[string]any{"password": backupPassword})
	if _, err := pc.backupNow(context.Background(), true); err != nil {
		t.Fatal(err)
	}

	// Nouvel appareil (téléphone perdu, PC réinstallé…) : connexion GitHub, liste, restauration.
	fresh, _ := newBackupService(t, srv, t.TempDir(), clock)
	if r, err := callJSON(t, fresh, "backupList", `{}`); err != nil || r["connected"] != false {
		t.Errorf("liste sans compte : %v %v", r, err)
	}
	call(t, fresh, "backupConnect", nil)
	waitBackupGitHub(t, fresh)
	list, err := callJSON(t, fresh, "backupList", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	entries, _ := list["entries"].([]any)
	if len(entries) != 1 {
		t.Fatalf("sauvegardes listées : %v", list)
	}
	entry := entries[0].(map[string]any)
	if entry["mine"] != false || !backup.Anonymous(entry["device"].(string)) || !strings.HasPrefix(entry["device"].(string), "windows-") ||
		strings.Contains(entry["name"].(string), "bureau") || entry["date"] != "2026-09-30" {
		t.Errorf("sauvegarde : %v", entry)
	}
	step, err := callJSON(t, fresh, "backupRestore", `{"name":"`+entry["name"].(string)+`"}`)
	if err != nil || step["step"] != "password" {
		t.Fatalf("restauration : %v %v", step, err)
	}
	confirm := call(t, fresh, "importPassword", map[string]any{"password": backupPassword})
	if confirm["step"] != "confirm" || confirm["count"] != float64(2) || confirm["sharing"] == nil {
		t.Fatalf("confirmation : %v", confirm)
	}
	done := call(t, fresh, "importConfirm", map[string]any{"replace": true})
	if done["ok"] != true || done["sharing"] != true || len(fresh.State().Devices) != 2 {
		t.Errorf("restauration : %v, %d PC", done, len(fresh.State().Devices))
	}
	if _, err := callJSON(t, fresh, "backupRestore", `{"name":"inconnu.json"}`); err == nil {
		t.Error("fichier hors liste restauré")
	}
}

func TestBackupsPausedAfterDataLoss(t *testing.T) {
	_, srv := newGistServer(t)
	clock := &testClock{now: time.Date(2026, 9, 30, 10, 0, 0, 0, time.Local)}
	dir := t.TempDir()
	name := "share.json" // en clair hors Windows (développement et tests)
	if runtime.GOOS == "windows" {
		name = "share.dat"
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte("abîmé"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, platform := newBackupService(t, srv, dir, clock, testDevice(t, "dev-1", "PC", "192.168.1.20", ""))
	platform.folder = t.TempDir()
	call(t, s, "backupEnable", map[string]any{"password": backupPassword})
	call(t, s, "backupFolder", nil)
	if st := s.State().Backup; !strings.Contains(st.Paused, "partage illisible") {
		t.Fatalf("pause : %+v", st)
	}
	clock.Add(backupDaily)
	if s.backupDue() {
		t.Error("sauvegarde automatique malgré des données illisibles")
	}
	// Sauvegarde demandée : la pause est levée.
	if _, err := callJSON(t, s, "backupNow", `{}`); err != nil {
		t.Fatal(err)
	}
	if st := s.State().Backup; st.Paused != "" {
		t.Errorf("pause maintenue : %+v", st)
	}
}

// Jeton du partage révoqué (ou expiré) sur GitHub alors que le partage se croit connecté : la
// connexion des sauvegardes ne doit pas rester bloquée sur « expirée ou révoquée ». Elle passe par
// un code, et le nouveau jeton répare aussi le partage (même compte).
func TestBackupConnectFallsBackWhenShareTokenRevoked(t *testing.T) {
	_, srv := newGistServer(t)
	clock := &testClock{now: time.Date(2026, 9, 30, 10, 0, 0, 0, time.Local)}
	s, _ := newBackupService(t, srv, t.TempDir(), clock)

	call(t, s, "shareLogin", map[string]any{"name": "Hugo"})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && (s.State().Share.Owner == nil || !s.State().Share.Owner.Connected) {
		time.Sleep(10 * time.Millisecond)
	}
	// GitHub a révoqué le jeton du partage ; rien n'a encore été publié, le partage l'ignore.
	s.sharing.mu.Lock()
	s.sharing.state.Owner.Token = "revoque"
	s.sharing.mu.Unlock()
	if o := s.State().Share.Owner; o == nil || !o.Connected {
		t.Fatalf("partage : %+v", o)
	}

	r, err := callJSON(t, s, "backupConnect", `{}`)
	if err != nil {
		t.Fatalf("connexion bloquée : %v", err)
	}
	if r["code"] != "WXYZ-1234" || r["shareUser"] != "hugo" || r["connected"] != nil {
		t.Fatalf("connexion par code attendue : %v", r)
	}
	if o := s.State().Share.Owner; o.Connected || o.Error == "" {
		t.Errorf("partage toujours affiché connecté : %+v", o)
	}

	waitBackupGitHub(t, s)
	if st := s.State().Backup; st.GitHub.Label != "@hugo" || st.Login != nil {
		t.Errorf("sauvegardes : %+v", st)
	}
	// Le partage reprend la nouvelle connexion (même compte).
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && !s.State().Share.Owner.Connected {
		time.Sleep(10 * time.Millisecond)
	}
	s.sharing.mu.Lock()
	token := s.sharing.state.Owner.Token
	s.sharing.mu.Unlock()
	if o := s.State().Share.Owner; !o.Connected || o.Error != "" || token != "tok" {
		t.Errorf("partage non reconnecté : %+v (jeton %q)", o, token)
	}

	// Ensuite, le compte du partage (valide) est repris directement.
	call(t, s, "backupDisconnect", nil)
	if r, err := callJSON(t, s, "backupConnect", `{}`); err != nil || r["connected"] != true {
		t.Errorf("compte du partage non repris : %v %v", r, err)
	}
}

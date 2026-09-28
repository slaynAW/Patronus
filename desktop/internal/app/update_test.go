package app

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/desktop/internal/config"
	"github.com/slaynaw/wakeonlan/desktop/internal/model"
	"github.com/slaynaw/wakeonlan/desktop/internal/netstate"
	"github.com/slaynaw/wakeonlan/desktop/internal/status"
	"github.com/slaynaw/wakeonlan/desktop/internal/update"
)

// releaseServer imite GitHub Releases : manifeste signé avec une clé de test et fichier de la version.
func releaseServer(t *testing.T, key *rsa.PrivateKey, code int64, payload []byte) *httptest.Server {
	t.Helper()
	sum := sha256.Sum256(payload)
	manifest, _ := json.Marshal(update.Manifest{
		Format: 1, Version: "9.9.0", Code: code, Date: "2026-10-04", Notes: "- **Nouveau** : test",
		Files: []update.File{{Platform: "windows-amd64", Name: "WakeOnLan-Windows-9.9.0-x64.exe", Size: int64(len(payload)), SHA256: hex.EncodeToString(sum[:])}},
	})
	digest := sha256.Sum256(manifest)
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /releases/latest/download/update.json", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(manifest) })
	mux.HandleFunc("GET /releases/latest/download/update.json.sig", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(base64.StdEncoding.EncodeToString(sig)))
	})
	mux.HandleFunc("GET /releases/download/v9.9.0/WakeOnLan-Windows-9.9.0-x64.exe", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(payload) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func newUpdateService(t *testing.T, srv *httptest.Server, key *rsa.PrivateKey, exe string) (*Service, *fakePlatform) {
	t.Helper()
	dir := t.TempDir()
	store, err := config.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	platform := &fakePlatform{}
	s := New(Options{
		Version: "1.2.0", Store: store, Platform: platform,
		Prober:   status.ProberFunc(func(context.Context, model.Device) status.ProbeResult { return status.ProbeResult{} }),
		NetState: func() (netstate.State, error) { return netstate.State{}, nil },
		Updates: &UpdateOptions{
			Source:   update.Source{Base: srv.URL + "/releases", Key: &key.PublicKey},
			Code:     40,
			Platform: "windows-amd64",
			Exe:      exe,
		},
	})
	return s, platform
}

func TestUpdateCheckPostponeAndInstall(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	srv := releaseServer(t, key, 45, []byte("nouvelle version"))
	exeDir := t.TempDir()
	exe := filepath.Join(exeDir, "WakeOnLan.exe")
	_ = os.WriteFile(exe, []byte("ancienne version"), 0o755)
	s, platform := newUpdateService(t, srv, key, exe)

	if v := s.State().Update; !v.Enabled || !v.Auto || v.Available != nil {
		t.Fatalf("état initial : %+v", v)
	}
	u := call(t, s, "checkUpdate", nil)
	available, _ := u["available"].(map[string]any)
	if available["version"] != "9.9.0" || available["notes"] != "- **Nouveau** : test" || u["lastCheck"] == nil {
		t.Fatalf("recherche : %v", u)
	}

	// « Plus tard » : plus proposée d'elle-même, mais toujours visible.
	call(t, s, "postponeUpdate", nil)
	if v := s.State().Update; !v.Postponed || v.Available == nil {
		t.Errorf("après « Plus tard » : %+v", v)
	}
	call(t, s, "setAutoUpdate", map[string]any{"enabled": false})
	if s.State().Update.Auto {
		t.Error("recherche automatique non désactivée")
	}
	// Réglages conservés d'un lancement à l'autre.
	reloaded := newUpdater(nil, filepath.Dir(s.updates.path), time.Now)
	if reloaded.prefs.Auto || reloaded.prefs.PostponedVersion != "9.9.0" || reloaded.prefs.LastCheck == 0 {
		t.Errorf("préférences enregistrées : %+v", reloaded.prefs)
	}

	// Installation : téléchargement vérifié, remplacement de l'exécutable, relance.
	call(t, s, "installUpdate", nil)
	deadline := time.Now().Add(5 * time.Second)
	for platform.relaunched == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if platform.relaunched != 1 {
		t.Fatalf("application non relancée : %+v", s.State().Update)
	}
	if data, _ := os.ReadFile(exe); string(data) != "nouvelle version" {
		t.Errorf("exécutable : %q", data)
	}
	if data, _ := os.ReadFile(exe + ".old"); string(data) != "ancienne version" {
		t.Errorf("ancienne version gardée pour le redémarrage : %q", data)
	}
}

func TestUpdateNotNewerOrTampered(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	other, _ := rsa.GenerateKey(rand.Reader, 2048)

	// Même code de build : rien à proposer.
	s, _ := newUpdateService(t, releaseServer(t, key, 40, []byte("x")), key, "")
	if u := call(t, s, "checkUpdate", nil); u["available"] != nil {
		t.Errorf("version identique proposée : %v", u)
	}

	// Manifeste signé par une autre clé : refusé, rien n'est proposé.
	s2, platform := newUpdateService(t, releaseServer(t, other, 99, []byte("x")), key, "")
	if _, err := s2.Call("checkUpdate", nil); err == nil {
		t.Error("manifeste d'une autre clé accepté")
	}
	if v := s2.State().Update; v.Available != nil || v.Error == "" {
		t.Errorf("état après refus : %+v", v)
	}
	if _, err := s2.Call("installUpdate", nil); err == nil || platform.relaunched != 0 {
		t.Error("installation sans mise à jour valide")
	}
}

func TestUpdatesDisabled(t *testing.T) {
	s, _ := newService(t)
	if s.State().Update.Enabled {
		t.Error("mises à jour actives sans source")
	}
	if _, err := s.Call("checkUpdate", nil); err == nil {
		t.Error("recherche possible sans source")
	}
}

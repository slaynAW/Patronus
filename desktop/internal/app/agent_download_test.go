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
	"strings"
	"testing"

	"github.com/slaynaw/wakeonlan/agent/update"
	"github.com/slaynaw/wakeonlan/desktop/internal/config"
	"github.com/slaynaw/wakeonlan/desktop/internal/model"
	"github.com/slaynaw/wakeonlan/desktop/internal/netstate"
	"github.com/slaynaw/wakeonlan/desktop/internal/status"
)

var (
	agentX64 = []byte("agent 9.9.1 pour x64")
	agentARM = []byte("agent 9.9.1 pour ARM64")
)

// agentReleaseServer publie une version 9.9.0 de l'application contenant l'agent 9.9.1 (x64 et ARM64).
// served remplace le contenu envoyé pour un fichier (fichier altéré en chemin).
func agentReleaseServer(t *testing.T, key *rsa.PrivateKey, served map[string][]byte) *httptest.Server {
	t.Helper()
	file := func(platform, name string, content []byte) update.File {
		sum := sha256.Sum256(content)
		return update.File{Platform: platform, Name: name, Size: int64(len(content)), SHA256: hex.EncodeToString(sum[:])}
	}
	manifest, _ := json.Marshal(update.Manifest{
		Format: 1, Version: "9.9.0", Code: 90, Date: "2026-10-04", Notes: "- Application",
		Files: []update.File{},
		Agent: &update.Agent{Version: "9.9.1", Notes: "- **Températures** du PC", Files: []update.File{
			file("windows-amd64", "wol-agent-windows-amd64.exe", agentX64),
			file("windows-arm64", "wol-agent-windows-arm64.exe", agentARM),
		}},
	})
	digest := sha256.Sum256(manifest)
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	content := map[string][]byte{"wol-agent-windows-amd64.exe": agentX64, "wol-agent-windows-arm64.exe": agentARM}
	for name, data := range served {
		content[name] = data
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /releases/latest/download/update.json", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(manifest) })
	mux.HandleFunc("GET /releases/latest/download/update.json.sig", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(base64.StdEncoding.EncodeToString(sig)))
	})
	mux.HandleFunc("GET /releases/download/v9.9.0/{name}", func(w http.ResponseWriter, r *http.Request) {
		data, ok := content[r.PathValue("name")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(data)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func newAgentService(t *testing.T, srv *httptest.Server, key *rsa.PrivateKey, dir string, withUpdates bool) (*Service, *fakePlatform) {
	t.Helper()
	store, err := config.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	platform := &fakePlatform{}
	src := update.Source{Base: srv.URL + "/releases", Key: &key.PublicKey}
	opts := Options{
		Version: "1.5.3", Store: store, Platform: platform,
		Prober:   status.ProberFunc(func(context.Context, model.Device) status.ProbeResult { return status.ProbeResult{} }),
		NetState: func() (netstate.State, error) { return netstate.State{}, nil },
		Agent:    &AgentOptions{Source: src, Platform: "windows-amd64", Dir: dir},
	}
	if withUpdates {
		opts.Updates = &UpdateOptions{Source: src, Code: 80, Platform: "windows-amd64"}
	}
	return New(opts), platform
}

func testKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func callJSON(t *testing.T, s *Service, method, params string) (map[string]any, error) {
	t.Helper()
	res, err := s.Call(method, json.RawMessage(params))
	if err != nil {
		return nil, err
	}
	var out map[string]any
	data, _ := json.Marshal(res)
	_ = json.Unmarshal(data, &out)
	return out, nil
}

func TestAgentInfoAndSave(t *testing.T) {
	key := testKey(t)
	srv := agentReleaseServer(t, key, nil)
	dir := filepath.Join(t.TempDir(), "Agent")
	_ = os.MkdirAll(dir, 0o700)
	_ = os.WriteFile(filepath.Join(dir, "reste.exe"), []byte("ancien"), 0o600)
	s, platform := newAgentService(t, srv, key, dir, false)
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("dossier de travail non vidé au démarrage : %v", err)
	}
	if st := s.State().Agent; !st.Enabled || st.Latest != "" {
		t.Errorf("état initial : %+v", st)
	}

	info, err := callJSON(t, s, "agentInfo", `{}`)
	if err != nil || info["version"] != "9.9.1" || info["platform"] != "windows-amd64" || !strings.Contains(info["notes"].(string), "Températures") {
		t.Fatalf("agentInfo : %v %v", info, err)
	}
	if sizes := info["sizes"].(map[string]any); sizes["windows-amd64"] != float64(len(agentX64)) || sizes["windows-arm64"] != float64(len(agentARM)) {
		t.Errorf("tailles : %v", sizes)
	}
	if got := s.State().Agent.Latest; got != "9.9.1" {
		t.Errorf("dernière version connue : %q", got)
	}

	// Enregistrement de l'agent ARM64 pour un autre PC.
	res, err := callJSON(t, s, "agentDownload", `{"platform":"windows-arm64"}`)
	if err != nil || res["path"] == nil || res["version"] != "9.9.1" {
		t.Fatalf("enregistrement : %v %v", res, err)
	}
	if string(platform.saved) != string(agentARM) || platform.savedName != "wol-agent-windows-arm64.exe" {
		t.Errorf("fichier enregistré : %q (%s)", platform.saved, platform.savedName)
	}
	if platform.installer != nil {
		t.Error("installation lancée pour un simple enregistrement")
	}
	if left, _ := os.ReadDir(dir); len(left) != 0 {
		t.Errorf("fichiers de travail restants : %v", left)
	}
	if st := s.State().Agent; st.Busy {
		t.Errorf("téléchargement toujours signalé : %+v", st)
	}

	// Annulation de la boîte d'enregistrement.
	platform.cancel = true
	if res, err := callJSON(t, s, "agentDownload", `{"platform":"windows-amd64"}`); err != nil || res["cancelled"] != true {
		t.Errorf("annulation : %v %v", res, err)
	}
}

func TestAgentInstall(t *testing.T) {
	key := testKey(t)
	srv := agentReleaseServer(t, key, nil)
	dir := filepath.Join(t.TempDir(), "Agent")
	s, platform := newAgentService(t, srv, key, dir, false)

	res, err := callJSON(t, s, "agentDownload", `{"platform":"windows-amd64","install":true}`)
	if err != nil || res["installing"] != true {
		t.Fatalf("installation : %v %v", res, err)
	}
	sum := sha256.Sum256(agentX64)
	if len(platform.installer) != 2 || filepath.Dir(platform.installer[0]) != dir ||
		filepath.Base(platform.installer[0]) != "wol-agent-windows-amd64.exe" || platform.installer[1] != hex.EncodeToString(sum[:]) {
		t.Errorf("installateur : %v", platform.installer)
	}
	if string(platform.installed) != string(agentX64) {
		t.Errorf("fichier lancé : %q", platform.installed)
	}

	// L'agent d'un autre processeur ne s'installe pas sur ce PC ; plateforme inconnue refusée.
	if _, err := callJSON(t, s, "agentDownload", `{"platform":"windows-arm64","install":true}`); err == nil {
		t.Error("agent ARM64 installé sur un PC x64")
	}
	if _, err := callJSON(t, s, "agentDownload", `{"platform":"linux-amd64"}`); err == nil {
		t.Error("plateforme inconnue acceptée")
	}

	// Invite administrateur refusée : rien d'installé, fichier effacé.
	platform.cancel = true
	res, err = callJSON(t, s, "agentDownload", `{"platform":"windows-amd64","install":true}`)
	if err != nil || res["cancelled"] != true {
		t.Errorf("invite refusée : %v %v", res, err)
	}
	if left, _ := os.ReadDir(dir); len(left) != 0 {
		t.Errorf("fichier conservé après refus : %v", left)
	}
}

func TestAgentDownloadRejectsTamperedFile(t *testing.T) {
	key := testKey(t)
	srv := agentReleaseServer(t, key, map[string][]byte{"wol-agent-windows-amd64.exe": []byte("agent piégé !!!!!!!!")})
	dir := filepath.Join(t.TempDir(), "Agent")
	s, platform := newAgentService(t, srv, key, dir, false)
	_, err := callJSON(t, s, "agentDownload", `{"platform":"windows-amd64","install":true}`)
	if err == nil || !strings.Contains(err.Error(), "ne correspond pas") {
		t.Fatalf("fichier altéré accepté : %v", err)
	}
	if platform.installer != nil || platform.saved != nil {
		t.Error("fichier altéré lancé ou enregistré")
	}
	if left, _ := os.ReadDir(dir); len(left) != 0 {
		t.Errorf("fichier altéré conservé : %v", left)
	}

	// Manifeste signé par une autre clé : refusé avant tout téléchargement.
	other := testKey(t)
	s2, _ := newAgentService(t, srv, other, filepath.Join(t.TempDir(), "Agent"), false)
	if _, err := callJSON(t, s2, "agentInfo", `{}`); err == nil {
		t.Error("manifeste à la signature invalide accepté")
	}
}

func TestUpdateCheckNotesLatestAgent(t *testing.T) {
	key := testKey(t)
	srv := agentReleaseServer(t, key, nil)
	s, _ := newAgentService(t, srv, key, filepath.Join(t.TempDir(), "Agent"), true)
	if err := s.checkUpdate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := s.State().Agent.Latest; got != "9.9.1" {
		t.Errorf("dernière version de l'agent après la recherche de mise à jour : %q", got)
	}
}

func TestAgentDownloadUnavailable(t *testing.T) {
	s, _ := newService(t)
	if st := s.State().Agent; st.Enabled {
		t.Errorf("téléchargement annoncé sans source : %+v", st)
	}
	if _, err := callJSON(t, s, "agentInfo", `{}`); err == nil {
		t.Error("agentInfo sans source")
	}
}

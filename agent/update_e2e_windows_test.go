//go:build e2e

package main

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/internal/config"
	"github.com/slaynaw/wakeonlan/agent/internal/selfupdate"
	"github.com/slaynaw/wakeonlan/agent/internal/service"
	"github.com/slaynaw/wakeonlan/agent/internal/terminal"
	"github.com/slaynaw/wakeonlan/agent/update"
)

// TestUpdateEndToEnd (CI Windows, droits administrateur) : un agent 1.0.0 est installé comme service,
// une version 9.9.9 est publiée sur un faux serveur de versions (manifeste signé par une clé de test
// intégrée aux deux compilations), puis le service doit la trouver, l'installer et redémarrer avec.
//
//	go test -tags e2e -run TestUpdateEndToEnd -v .
func TestUpdateEndToEnd(t *testing.T) {
	if !terminal.IsAdmin() {
		t.Skip("droits administrateur nécessaires")
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)

	platform := "windows-" + runtime.GOARCH
	name := "wol-agent-" + platform + ".exe"
	var manifest, signature, payload []byte
	mux := http.NewServeMux()
	mux.HandleFunc("GET /releases/latest/download/update.json", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(manifest) })
	mux.HandleFunc("GET /releases/latest/download/update.json.sig", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(signature) })
	mux.HandleFunc("GET /releases/download/v9.9.9/"+name, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(payload) })
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	build := func(v string) string {
		out := filepath.Join(dir, "wol-agent-"+v+".exe")
		flags := strings.Join([]string{
			"-X main.version=" + v,
			"-X github.com/slaynaw/wakeonlan/agent/update.testBase=" + srv.URL + "/releases",
			"-X github.com/slaynaw/wakeonlan/agent/update.testKey=" + base64.StdEncoding.EncodeToString(der),
			"-X github.com/slaynaw/wakeonlan/agent/internal/selfupdate.testAccept=1",
		}, " ")
		cmd := exec.Command("go", "build", "-trimpath", "-ldflags", flags, "-o", out, ".")
		if msg, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("compilation %s : %v\n%s", v, err, msg)
		}
		return out
	}
	oldExe, newExe := build("1.0.0"), build("9.9.9")

	payload, err = os.ReadFile(newExe)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	manifest, _ = json.Marshal(update.Manifest{
		Format: 1, Version: "9.9.9", Code: 1, Files: []update.File{},
		Agent: &update.Agent{Version: "9.9.9", Notes: "- Test de bout en bout", Files: []update.File{
			{Platform: platform, Name: name, Size: int64(len(payload)), SHA256: hex.EncodeToString(sum[:])},
		}},
	})
	digest := sha256.Sum256(manifest)
	sig, _ := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	signature = []byte(base64.StdEncoding.EncodeToString(sig))

	installed := service.DefaultBinary()
	t.Cleanup(func() {
		_ = exec.Command(installed, "uninstall", "--purge").Run()
		_ = os.RemoveAll(filepath.Dir(installed))
	})
	// L'installation affiche aussi le QR code d'appairage : seul le service importe ici.
	out, err := exec.Command(oldExe, "install", "--no-firewall").CombinedOutput()
	t.Logf("installation : %v\n%s", err, out)
	if v, err := selfupdate.ProbeVersion(context.Background(), installed); err != nil || v != "1.0.0" {
		t.Fatalf("agent installé : %q, %v", v, err)
	}

	// Le service vérifie 2 s après son démarrage (mode test), installe puis redémarre.
	deadline := time.Now().Add(3 * time.Minute)
	for {
		v, _ := selfupdate.ProbeVersion(context.Background(), installed)
		_, oldErr := os.Stat(installed + ".old")
		if v == "9.9.9" && service.IsRunning() && os.IsNotExist(oldErr) && serviceHealthy() {
			break
		}
		if time.Now().After(deadline) {
			logData, _ := os.ReadFile(filepath.Join(config.Dir(), "agent.log"))
			t.Fatalf("mise à jour non terminée : version %q, service démarré %v, ancienne version présente %v\njournal :\n%s",
				v, service.IsRunning(), oldErr == nil, logData)
		}
		time.Sleep(time.Second)
	}
	logData, _ := os.ReadFile(filepath.Join(config.Dir(), "agent.log"))
	if !strings.Contains(string(logData), "agent 9.9.9 installé") {
		t.Errorf("journal sans la fin de la mise à jour :\n%s", logData)
	}
	// La nouvelle version, à jour, ne se propose pas elle-même.
	out, err = exec.Command(installed, "update", "--check").CombinedOutput()
	if err != nil || !strings.Contains(string(out), "à jour (version 9.9.9)") {
		t.Errorf("update --check : %v\n%s", err, out)
	}
}

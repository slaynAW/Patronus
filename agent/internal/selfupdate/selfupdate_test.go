package selfupdate

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/update"
)

// Le binaire de test sert aussi de « nouvelle version » téléchargée : lancé avec cette variable, il
// se comporte comme « wol-agent version ».
const fakeAgentEnv = "WOL_SELFUPDATE_FAKE_VERSION"

func TestMain(m *testing.M) {
	if v := os.Getenv(fakeAgentEnv); v != "" && len(os.Args) > 1 && os.Args[len(os.Args)-1] == "version" {
		fmt.Println("wol-agent " + v)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type release struct {
	key      *rsa.PrivateKey
	payload  []byte
	manifest update.Manifest
	hits     atomic.Int32
}

// newRelease simule GitHub Releases : manifeste signé (clé de test) et binaire de l'agent.
func newRelease(t *testing.T, agentVersion string) (*release, update.Source) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	r := &release{key: key, payload: payload}
	r.manifest = update.Manifest{Format: 1, Version: "1.4.0", Code: 60, Files: []update.File{}}
	if agentVersion != "" {
		r.manifest.Agent = &update.Agent{Version: agentVersion, Notes: "- **Mises à jour** guidées", Files: []update.File{{
			Platform: Platform(), Name: "wol-agent-" + Platform() + ".exe", Size: int64(len(payload)), SHA256: hex.EncodeToString(sum[:]),
		}}}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /releases/latest/download/update.json", func(w http.ResponseWriter, _ *http.Request) {
		r.hits.Add(1)
		_, _ = w.Write(r.signed(t))
	})
	mux.HandleFunc("GET /releases/latest/download/update.json.sig", func(w http.ResponseWriter, _ *http.Request) {
		data := r.signed(t)
		digest := sha256.Sum256(data)
		sig, _ := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
		_, _ = w.Write([]byte(base64.StdEncoding.EncodeToString(sig)))
	})
	mux.HandleFunc("GET /releases/download/v1.4.0/{name}", func(w http.ResponseWriter, req *http.Request) {
		if r.manifest.Agent == nil || req.PathValue("name") != r.manifest.Agent.Files[0].Name {
			http.NotFound(w, req)
			return
		}
		_, _ = w.Write(r.payload)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return r, update.Source{Base: srv.URL + "/releases", Key: &key.PublicKey}
}

func (r *release) signed(t *testing.T) []byte {
	data, err := json.Marshal(r.manifest)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestCheck(t *testing.T) {
	ctx := context.Background()
	_, src := newRelease(t, "1.4.0")
	offer, err := Check(ctx, src, "1.3.1")
	if err != nil || offer == nil || offer.Version != "1.4.0" || offer.File.Platform != Platform() {
		t.Fatalf("mise à jour attendue : %+v, %v", offer, err)
	}
	for _, current := range []string{"1.4.0", "1.5.0", "dev"} {
		if offer, err := Check(ctx, src, current); err != nil || offer != nil {
			t.Errorf("agent %s : %+v, %v", current, offer, err)
		}
	}
	// Version publiée sans agent (manifestes antérieurs).
	_, bare := newRelease(t, "")
	if offer, err := Check(ctx, bare, "1.2.0"); err != nil || offer != nil {
		t.Errorf("sans agent : %+v, %v", offer, err)
	}
	// Manifeste signé par une autre clé : refusé.
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	forged := src
	forged.Key = &other.PublicKey
	if _, err := Check(ctx, forged, "1.3.1"); !errors.Is(err, update.ErrSignature) {
		t.Errorf("signature d'une autre clé : %v", err)
	}
}

func TestDownload(t *testing.T) {
	ctx := context.Background()
	r, src := newRelease(t, "1.4.0")
	dir := t.TempDir()
	exe := filepath.Join(dir, "wol-agent.exe")
	offer, err := Check(ctx, src, "1.3.1")
	if err != nil || offer == nil {
		t.Fatal(offer, err)
	}

	t.Setenv(fakeAgentEnv, "1.4.0")
	next, err := Download(ctx, src, *offer, exe)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(next) != dir {
		t.Errorf("téléchargé hors du dossier de l'agent : %s", next)
	}
	if data, _ := os.ReadFile(next); len(data) != len(r.payload) {
		t.Errorf("fichier téléchargé : %d octets", len(data))
	}

	// La version téléchargée annonce un autre numéro : refusée et supprimée.
	t.Setenv(fakeAgentEnv, "1.3.9")
	if _, err := Download(ctx, src, *offer, exe); err == nil || !strings.Contains(err.Error(), "1.3.9") {
		t.Errorf("mauvaise version acceptée : %v", err)
	}
	// Empreinte différente de celle du manifeste : refusée.
	bad := *offer
	bad.File.SHA256 = strings.Repeat("0", 64)
	if _, err := Download(ctx, src, bad, exe); err == nil {
		t.Error("fichier modifié accepté")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("fichiers restants : %v", entries)
	}
}

func TestAutoOnce(t *testing.T) {
	ctx := context.Background()
	r, src := newRelease(t, "1.4.0")
	now := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	var asked, applied int
	answer := Later
	a := &Auto{
		Source: src, Current: "1.3.1", StatePath: filepath.Join(t.TempDir(), "update-state.json"),
		Ask: func(_ context.Context, o Offer) (Answer, error) {
			asked++
			if o.Version != "1.4.0" {
				t.Errorf("version proposée : %s", o.Version)
			}
			return answer, nil
		},
		Apply: func(context.Context, Offer) error { applied++; return nil },
		Now:   func() time.Time { return now },
	}

	// « Non » : reproposée le lendemain, sans nouvelle question ni requête d'ici là.
	if wait := a.Once(ctx); wait != RemindAfter || asked != 1 || applied != 0 {
		t.Fatalf("refus : attente %v, %d question(s), %d installation(s)", wait, asked, applied)
	}
	now = now.Add(2 * time.Hour)
	hits := r.hits.Load()
	if wait := a.Once(ctx); wait != RemindAfter-2*time.Hour || asked != 1 || r.hits.Load() != hits {
		t.Fatalf("report : attente %v, %d question(s)", wait, asked)
	}

	// Personne n'est connecté : nouvel essai dans l'heure.
	now = now.Add(RemindAfter)
	answer = NoUser
	if wait := a.Once(ctx); wait != NoUserRetry || applied != 0 {
		t.Fatalf("sans utilisateur : attente %v", wait)
	}

	// « Oui » : installation.
	answer = Accept
	if wait := a.Once(ctx); wait != Interval || applied != 1 {
		t.Fatalf("accord : attente %v, %d installation(s)", wait, applied)
	}

	// À jour : pas de question.
	a.Current = "1.4.0"
	asked = 0
	if wait := a.Once(ctx); wait != Interval || asked != 0 {
		t.Fatalf("à jour : attente %v, %d question(s)", wait, asked)
	}

	// Échec de l'installation : reproposée le lendemain.
	a.Current, answer = "1.3.1", Accept
	a.Apply = func(context.Context, Offer) error { return errors.New("disque plein") }
	if wait := a.Once(ctx); wait != Interval {
		t.Fatalf("échec : attente %v", wait)
	}
	if wait := a.Once(ctx); wait != RemindAfter {
		t.Fatalf("après un échec : attente %v", wait)
	}
}

func TestFinish(t *testing.T) {
	setup := func(t *testing.T) string {
		dir := t.TempDir()
		exe := filepath.Join(dir, "wol-agent.exe")
		_ = os.WriteFile(exe, []byte("nouvelle"), 0o755)
		_ = os.WriteFile(exe+".old", []byte("ancienne"), 0o755)
		return exe
	}
	read := func(path string) string { data, _ := os.ReadFile(path); return string(data) }

	exe := setup(t)
	restarts := 0
	restart := func() error { restarts++; return nil }
	if err := Finish(exe, restart, func() bool { return true }); err != nil || restarts != 1 || read(exe) != "nouvelle" {
		t.Fatalf("succès : %v, %d redémarrage(s), %q", err, restarts, read(exe))
	}

	// La nouvelle version ne répond pas : l'ancienne est remise en place et redémarrée.
	exe = setup(t)
	restarts = 0
	err := Finish(exe, restart, func() bool { return false })
	if err == nil || restarts != 2 || read(exe) != "ancienne" || read(exe+".failed") != "nouvelle" {
		t.Fatalf("retour arrière : %v, %d redémarrage(s), %q", err, restarts, read(exe))
	}

	// Le service ne redémarre pas du tout.
	exe = setup(t)
	calls := 0
	err = Finish(exe, func() error {
		calls++
		if calls == 1 {
			return errors.New("accès refusé")
		}
		return nil
	}, func() bool { return true })
	if err == nil || !strings.Contains(err.Error(), "accès refusé") || read(exe) != "ancienne" {
		t.Fatalf("redémarrage impossible : %v, %q", err, read(exe))
	}
}

func TestMessage(t *testing.T) {
	msg := Message("1.2.0", Offer{Version: "1.4.0", Notes: "### Agent\n- **Mises à jour** guidées\n\n- Journal plus précis"})
	for _, want := range []string{"1.2.0 → 1.4.0", "• Mises à jour guidées", "• Journal plus précis", "Agent\n", "Non : me le rappeler demain"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message sans %q :\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "**") || strings.Contains(msg, "###") {
		t.Errorf("Markdown restant :\n%s", msg)
	}
}

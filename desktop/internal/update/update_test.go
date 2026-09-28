package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type vectors struct {
	PublicKey        string `json:"publicKey"`
	Manifest         string `json:"manifest"`
	Signature        string `json:"signature"`
	TamperedManifest string `json:"tamperedManifest"`
	Expected         struct {
		Version   string   `json:"version"`
		Code      int64    `json:"code"`
		Date      string   `json:"date"`
		Platforms []string `json:"platforms"`
		Android   File     `json:"android"`
		URL       string   `json:"url"`
	} `json:"expected"`
	Invalid []struct {
		Reason   string `json:"reason"`
		Manifest string `json:"manifest"`
	} `json:"invalid"`
}

func load(t *testing.T) vectors {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "protocol", "update-vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var v vectors
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestSharedVectors(t *testing.T) {
	v := load(t)
	key, err := ParsePublicKey(v.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(key, []byte(v.Manifest), v.Signature); err != nil {
		t.Fatalf("signature valide refusée : %v", err)
	}
	if Verify(key, []byte(v.TamperedManifest), v.Signature) == nil {
		t.Error("manifeste modifié accepté")
	}
	if Verify(key, []byte(v.Manifest), "pas du base64") == nil {
		t.Error("signature illisible acceptée")
	}
	m, err := Parse([]byte(v.Manifest))
	if err != nil {
		t.Fatal(err)
	}
	if m.Version != v.Expected.Version || m.Code != v.Expected.Code || m.Date != v.Expected.Date {
		t.Errorf("manifeste lu : %+v", m)
	}
	var platforms []string
	for _, f := range m.Files {
		platforms = append(platforms, f.Platform)
	}
	if strings.Join(platforms, ",") != strings.Join(v.Expected.Platforms, ",") {
		t.Errorf("plateformes : %v", platforms)
	}
	android, ok := m.File("android")
	if !ok || android != v.Expected.Android {
		t.Errorf("fichier Android : %+v", android)
	}
	if !strings.Contains(m.Notes, "Latence en direct") {
		t.Errorf("nouveautés : %q", m.Notes)
	}
	if got := (Source{Base: DefaultBase}).FileURL(m, android); got != v.Expected.URL {
		t.Errorf("adresse : %s", got)
	}
	for _, c := range v.Invalid {
		if _, err := Parse([]byte(c.Manifest)); err == nil {
			t.Errorf("manifeste invalide accepté (%s)", c.Reason)
		}
	}
}

func TestReleaseCertificate(t *testing.T) {
	// Certificat de la clé de signature de l'APK (vérifié par la CI à chaque publication).
	key, err := ReleaseKey()
	if err != nil {
		t.Fatalf("certificat de mise à jour : %v", err)
	}
	if key.N.BitLen() < 2048 {
		t.Errorf("clé trop courte : %d bits", key.N.BitLen())
	}
	src, err := Official()
	if err != nil || src.Base != DefaultBase || src.Key == nil {
		t.Errorf("source officielle : %+v (%v)", src, err)
	}
}

// server simule GitHub Releases : manifeste signé (vecteurs partagés) et fichier de la version.
func server(t *testing.T, v vectors, payload []byte, manifest string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /releases/latest/download/update.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(manifest))
	})
	mux.HandleFunc("GET /releases/latest/download/update.json.sig", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(v.Signature + "\n"))
	})
	mux.HandleFunc("GET /releases/download/v1.3.0/{name}", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("name") != "WakeOnLan-Windows-1.3.0-x64.exe" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(payload)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestLatestAndDownload(t *testing.T) {
	v := load(t)
	key, _ := ParsePublicKey(v.PublicKey)
	srv := server(t, v, []byte("contenu"), v.Manifest)
	src := Source{Base: srv.URL + "/releases", Key: key}
	m, err := src.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if m.Version != "1.3.0" {
		t.Fatalf("version : %s", m.Version)
	}

	// Fichier conforme au manifeste (taille et empreinte recalculées pour ce test).
	dir := t.TempDir()
	f, _ := m.File("windows-amd64")
	sum := sha256.Sum256([]byte("contenu"))
	f.Size, f.SHA256 = 7, hex.EncodeToString(sum[:])
	dest := filepath.Join(dir, "next.exe")
	var last int64
	if err := src.Download(context.Background(), m, f, dest, func(done, _ int64) { last = done }); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(dest); string(data) != "contenu" || last != 7 {
		t.Errorf("fichier téléchargé : %q (%d)", data, last)
	}

	// Empreinte différente : rien n'est écrit.
	f.SHA256 = strings.Repeat("0", 64)
	dest2 := filepath.Join(dir, "autre.exe")
	if err := src.Download(context.Background(), m, f, dest2, nil); err == nil {
		t.Error("fichier corrompu accepté")
	}
	if _, err := os.Stat(dest2); err == nil {
		t.Error("fichier corrompu écrit")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("fichiers temporaires restants : %d", len(entries))
	}
}

func TestLatestRejectsTampered(t *testing.T) {
	v := load(t)
	key, _ := ParsePublicKey(v.PublicKey)
	srv := server(t, v, nil, v.TamperedManifest)
	if _, err := (Source{Base: srv.URL + "/releases", Key: key}).Latest(context.Background()); err != ErrSignature {
		t.Errorf("manifeste modifié : %v", err)
	}
	// Pas encore de manifeste publié (versions antérieures) : erreur distincte.
	empty := httptest.NewServer(http.NotFoundHandler())
	defer empty.Close()
	if _, err := (Source{Base: empty.URL, Key: key}).Latest(context.Background()); err != ErrNotPublished {
		t.Errorf("sans manifeste : %v", err)
	}
}

func TestReplace(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "WakeOnLan.exe")
	next := filepath.Join(dir, "next.exe")
	_ = os.WriteFile(exe, []byte("v1"), 0o755)
	_ = os.WriteFile(next, []byte("v2"), 0o755)
	if err := Replace(exe, next); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(exe); string(data) != "v2" {
		t.Errorf("nouvelle version : %q", data)
	}
	if data, _ := os.ReadFile(exe + ".old"); string(data) != "v1" {
		t.Errorf("ancienne version conservée : %q", data)
	}
	CleanupOld(exe)
	if _, err := os.Stat(exe + ".old"); err == nil {
		t.Error("ancienne version non supprimée")
	}
	// Échec (nouvelle version absente) : l'exécutable d'origine est remis en place.
	if err := Replace(exe, filepath.Join(dir, "absent.exe")); err == nil {
		t.Error("remplacement par un fichier absent")
	}
	if data, _ := os.ReadFile(exe); string(data) != "v2" {
		t.Errorf("exécutable restauré : %q", data)
	}
}

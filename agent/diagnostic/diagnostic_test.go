package diagnostic

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

// Rapport de référence produit indépendamment (Node.js), aussi ouvert par les tests Kotlin.
func TestVector(t *testing.T) {
	raw, err := os.ReadFile("../../protocol/diagnostic-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct{ Password, Report, File string }
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	env, text, err := Open([]byte(v.File), v.Password)
	if err != nil || text != v.Report || env.App != "Patronus Android 9.9.9" {
		t.Fatalf("vecteur : %q %v", text, err)
	}
	if _, _, err := Open([]byte(v.File), v.Password+"!"); !errors.Is(err, ErrPassword) {
		t.Errorf("mauvais mot de passe : %v", err)
	}
	tampered := strings.Replace(v.File, `"data": "`, `"data": "AA`, 1)
	if _, _, err := Open([]byte(tampered), v.Password); err == nil {
		t.Error("rapport modifié accepté")
	}
}

func TestSealOpen(t *testing.T) {
	report := "ligne 1\nétat : ✓\n"
	data, err := SealWith(report, "motdepasse", "Patronus Windows 1.5.4", "2026-09-29T17:00:00Z", MinIterations, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("ligne 1")) {
		t.Error("texte du rapport en clair dans le fichier")
	}
	env, text, err := Open(data, "motdepasse")
	if err != nil || text != report || env.App != "Patronus Windows 1.5.4" || env.CreatedAt != "2026-09-29T17:00:00Z" {
		t.Fatalf("aller-retour : %q %+v %v", text, env, err)
	}
	if _, err := Seal(report, "court", "x", "y"); err == nil {
		t.Error("mot de passe trop court accepté")
	}
	if _, _, err := Open([]byte(`{"format":"wakeonlan-config"}`), "motdepasse"); err == nil {
		t.Error("sauvegarde prise pour un rapport")
	}
	weak := strings.Replace(string(data), `"iterations": 100000`, `"iterations": 1000`, 1)
	if _, _, err := Open([]byte(weak), "motdepasse"); err == nil {
		t.Error("itérations trop faibles acceptées")
	}
}

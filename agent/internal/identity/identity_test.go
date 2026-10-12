package identity

import (
	"errors"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol2"
)

func TestIdentityLifecycle(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 12, 15, 0, 0, 0, time.UTC)
	if _, err := Load(dir, now); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("identité absente : %v", err)
	}
	first, created, err := LoadOrCreate(dir, now)
	if err != nil || !created {
		t.Fatalf("création : %v %v", created, err)
	}
	if pin, err := protocol2.PinFromCertificate(first.Cert.Certificate[0]); err != nil || pin != first.Pin {
		t.Fatalf("certificat : %v", err)
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(Path(dir)); err != nil || info.Mode().Perm() != 0o600 {
			t.Errorf("droits de la clé : %v %v", info.Mode(), err)
		}
	}
	// Relue plus tard : même clé, même empreinte, nouveau certificat.
	again, created, err := LoadOrCreate(dir, now.Add(48*time.Hour))
	if err != nil || created || again.Pin != first.Pin || again.Key.Equal(first.Key) == false {
		t.Fatalf("relecture : %v %v", created, err)
	}
	rotated, err := Rotate(dir, now)
	if err != nil || rotated.Pin == first.Pin {
		t.Fatalf("remplacement : %v", err)
	}
	if loaded, err := Load(dir, now); err != nil || loaded.Pin != rotated.Pin {
		t.Fatalf("relecture après remplacement : %v", err)
	}
	if err := os.WriteFile(Path(dir), []byte("abîmée"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadOrCreate(dir, now); err == nil || errors.Is(err, os.ErrNotExist) {
		t.Errorf("clé abîmée remplacée en silence : %v", err)
	}
}

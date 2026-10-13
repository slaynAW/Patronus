package identity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol2"
)

func TestCreatedOnceThenReused(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir, nil, nil)
	if s.Existing() != nil {
		t.Fatal("identité avant sa création")
	}
	if _, err := os.Stat(s.Path()); !os.IsNotExist(err) {
		t.Fatal("Existing ne doit rien créer")
	}
	id, err := s.Get()
	if err != nil {
		t.Fatal(err)
	}
	pin, err := protocol2.PinFromCertificate(id.Cert.Certificate[0])
	if err != nil || pin != id.Pin {
		t.Fatalf("empreinte du certificat : %v", err)
	}
	if info, err := os.Stat(s.Path()); err != nil || (info.Mode().Perm()&0o077 != 0 && os.PathSeparator == '/') {
		t.Fatalf("fichier de la clé : %v %v", info, err)
	}
	// Relue par une nouvelle instance (nouveau lancement) : même empreinte.
	again, err := NewStore(dir, nil, nil).Get()
	if err != nil || again.Pin != id.Pin {
		t.Fatalf("relecture : %v", err)
	}
	if got := NewStore(dir, nil, nil).Existing(); got == nil || got.Pin != id.Pin {
		t.Fatal("Existing après création")
	}
}

func TestUnreadableFileSetAside(t *testing.T) {
	dir := t.TempDir()
	first, err := NewStore(dir, nil, nil).Get()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, FileName)
	if err := os.WriteFile(path, []byte("abîmé"), 0o600); err != nil {
		t.Fatal(err)
	}
	if NewStore(dir, nil, nil).Existing() != nil {
		t.Fatal("fichier abîmé lu")
	}
	var warned string
	now := time.Date(2026, 10, 13, 8, 30, 0, 0, time.Local)
	s := NewStore(dir, func() time.Time { return now }, func(f string, a ...any) { warned = f })
	id, err := s.Get()
	if err != nil || id.Pin == first.Pin {
		t.Fatalf("nouvelle identité attendue : %v", err)
	}
	if !strings.Contains(warned, "illisible") {
		t.Errorf("incident non signalé : %q", warned)
	}
	kept, err := os.ReadFile(path + ".illisible-20261013-083000")
	if err != nil || string(kept) != "abîmé" {
		t.Fatalf("fichier illisible non conservé : %v", err)
	}
}

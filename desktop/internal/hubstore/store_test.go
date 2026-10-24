package hubstore

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestStore(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.Load(); err != nil || got != nil {
		t.Fatalf("aucun hub : %v %v", got, err)
	}
	want := &Hub{Name: "Hub NAS", Host: "192.168.1.20", Port: 9771, Pin: "pin", Role: "admin", Paired: 1_792_845_451}
	if err := s.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load()
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("relu : %+v %v", got, err)
	}
	// Hub oublié : fichier retiré, deux fois sans erreur.
	if err := s.Save(nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(nil); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Load(); err != nil || got != nil {
		t.Fatalf("hub oublié : %v %v", got, err)
	}
	// Fichier abîmé : mis de côté, aucun hub.
	if err := os.WriteFile(s.path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Load(); err == nil || got != nil || !strings.Contains(err.Error(), "mis de côté") {
		t.Fatalf("fichier abîmé : %v %v", got, err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || !strings.HasPrefix(entries[0].Name(), filepath.Base(s.path)+".illisible-") {
		t.Errorf("fichier mis de côté : %v", entries)
	}
	mem := Memory()
	if err := mem.Save(want); err != nil {
		t.Fatal(err)
	}
	if got, err := mem.Load(); err != nil || got != want {
		t.Errorf("magasin en mémoire : %v %v", got, err)
	}
	var none *Store
	if got, err := none.Load(); err != nil || got != nil || none.Save(want) != nil {
		t.Error("magasin nul")
	}
}

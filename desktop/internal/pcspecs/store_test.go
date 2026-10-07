package pcspecs

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/slaynaw/wakeonlan/agent/protocol"
)

func TestStore(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.Load(); err != nil || len(got) != 0 {
		t.Fatalf("magasin vide : %v %v", got, err)
	}
	want := map[string]Entry{"pc": {Specs: protocol.Specs{Model: "Dell XPS 15", CPU: &protocol.SpecsCPU{Name: "Intel Core i7", Cores: 14}}, Fetched: 1_790_000_000_000, Agent: "1.10.0"}}
	if err := s.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load()
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("relu : %+v %v", got, err)
	}
	// Fichier abîmé : mis de côté, liste vide.
	if err := os.WriteFile(s.path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Load(); err == nil || len(got) != 0 || !strings.Contains(err.Error(), "mises de côté") {
		t.Fatalf("fichier abîmé : %v %v", got, err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || !strings.HasPrefix(entries[0].Name(), filepath.Base(s.path)+".illisible-") {
		t.Errorf("fichier mis de côté : %v", entries)
	}
	var none *Store
	if got, err := none.Load(); err != nil || got == nil || none.Save(want) != nil {
		t.Error("magasin nul")
	}
}

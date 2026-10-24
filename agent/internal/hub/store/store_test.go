package store

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

func testKey(seed byte) Key {
	var k Key
	for i := range k {
		k[i] = seed + byte(i)
	}
	return k
}

func TestKeyLifecycle(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "keys")
	k1, created, err := LoadOrCreateKey(dir)
	if err != nil || !created {
		t.Fatalf("création : %v, %v", created, err)
	}
	// Droits Unix : le hub tourne sous Linux (sous Windows, Go ne renvoie pas les droits du fichier).
	if info, err := os.Stat(KeyPath(dir)); err != nil || runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("droits de la clé : %v, %v", info, err)
	}
	k2, created, err := LoadOrCreateKey(dir)
	if err != nil || created || k1 != k2 {
		t.Fatalf("relecture : %v, %v, même clé %v", created, err, k1 == k2)
	}
	if k1 == (Key{}) {
		t.Fatal("clé nulle")
	}
	// Une clé illisible n'est jamais remplacée.
	if err := os.WriteFile(KeyPath(dir), []byte("abîmée"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadOrCreateKey(dir); err == nil {
		t.Fatal("clé illisible acceptée")
	}
	if raw, _ := os.ReadFile(KeyPath(dir)); string(raw) != "abîmée" {
		t.Fatal("clé illisible remplacée")
	}
}

func TestValidPath(t *testing.T) {
	for _, p := range []string{"hub.db", "pc/AbC-_9/2026-10.bin", "a", "a.b.c"} {
		if !ValidPath(p) {
			t.Errorf("%q refusé", p)
		}
	}
	for _, p := range []string{"", "/hub.db", "../hub.db", "pc/../hub.db", "pc//x", ".hub", "pc/.x", "pc/", "a\\b",
		"hub.db.tmp", "é", "a b", string(make([]byte, 201))} {
		if ValidPath(p) {
			t.Errorf("%q accepté", p)
		}
	}
}

func TestFile(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, testKey(1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadFile("hub.db"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("fichier absent : %v", err)
	}
	for _, content := range []string{`{"pcs":[]}`, `{"pcs":[{"name":"PC Bureau"}]}`, ""} {
		if err := s.WriteFile("hub.db", []byte(content)); err != nil {
			t.Fatal(err)
		}
		got, err := s.ReadFile("hub.db")
		if err != nil || string(got) != content {
			t.Fatalf("relecture : %q, %v", got, err)
		}
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "hub.db"))
	if bytes.Contains(raw, []byte("pcs")) {
		t.Fatal("contenu en clair")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Fatalf("fichiers laissés : %v", entries)
	}

	// Déplacé, autre clé, modifié : illisible.
	if err := os.WriteFile(filepath.Join(dir, "other.db"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadFile("other.db"); !errors.Is(err, ErrCorrupted) {
		t.Errorf("fichier déplacé : %v", err)
	}
	other, _ := Open(dir, testKey(2))
	if _, err := other.ReadFile("hub.db"); !errors.Is(err, ErrCorrupted) {
		t.Errorf("autre clé : %v", err)
	}
	for i := range raw {
		changed := bytes.Clone(raw)
		changed[i] ^= 0x01
		if _, err := testKey(1).OpenFile("hub.db", changed); !errors.Is(err, ErrCorrupted) {
			t.Fatalf("octet %d modifié : %v", i, err)
		}
	}
	if _, err := testKey(1).OpenFile("hub.db", raw[:len(raw)-1]); !errors.Is(err, ErrCorrupted) {
		t.Errorf("fichier tronqué : %v", err)
	}

	if err := s.WriteFile("../evil", nil); !errors.Is(err, ErrBadPath) {
		t.Errorf("chemin hors du dossier : %v", err)
	}
	if err := s.WriteFile("big", make([]byte, MaxFileBytes+1)); !errors.Is(err, ErrTooLarge) {
		t.Errorf("fichier trop grand : %v", err)
	}
}

func records(n, from int) [][]byte {
	var out [][]byte
	for i := range n {
		out = append(out, []byte(fmt.Sprintf(`{"t":%d}`, from+i)))
	}
	return out
}

func TestJournal(t *testing.T) {
	dir := t.TempDir()
	const rel = "pc/AAAA/2026-10.bin"
	s, _ := Open(dir, testKey(1))
	if got, err := s.Records(rel); err != nil || got != nil {
		t.Fatalf("journal absent : %v, %v", got, err)
	}
	if err := s.Append(rel, records(2, 0)...); err != nil {
		t.Fatal(err)
	}
	if err := s.Append(rel, records(1, 2)...); err != nil {
		t.Fatal(err)
	}
	// Un autre processus (nouveau Store) reprend la numérotation.
	s2, _ := Open(dir, testKey(1))
	if err := s2.Append(rel, records(2, 3)...); err != nil {
		t.Fatal(err)
	}
	got, err := s.Records(rel)
	if err != nil || !slices.EqualFunc(got, records(5, 0), bytes.Equal) {
		t.Fatalf("relecture : %q, %v", got, err)
	}
	p := filepath.Join(dir, filepath.FromSlash(rel))
	raw, _ := os.ReadFile(p)
	if bytes.Contains(raw, []byte(`"t"`)) {
		t.Fatal("contenu en clair")
	}

	// Coupure pendant une écriture : l'enregistrement incomplet est ignoré, puis remplacé.
	for cut := 1; cut < 4+NonceSize+TagSize+8; cut += 7 {
		torn := append(bytes.Clone(raw), testKey(1).SealRecord(rel, 5, []byte(`{"t":5}`), make([]byte, NonceSize))[:cut]...)
		if err := os.WriteFile(p, torn, 0o600); err != nil {
			t.Fatal(err)
		}
		if got, err := s.Records(rel); err != nil || len(got) != 5 {
			t.Fatalf("coupure à %d : %d enregistrements, %v", cut, len(got), err)
		}
		s3, _ := Open(dir, testKey(1))
		if err := s3.Append(rel, []byte(`{"t":5}`)); err != nil {
			t.Fatal(err)
		}
		if got, err := s.Records(rel); err != nil || !slices.EqualFunc(got, records(6, 0), bytes.Equal) {
			t.Fatalf("après la coupure à %d : %q, %v", cut, got, err)
		}
	}

	// Enregistrement modifié, retiré ou déplacé : journal illisible.
	key := testKey(1)
	rec := func(i int) []byte {
		return key.SealRecord(rel, uint64(i), records(1, i)[0], bytes.Repeat([]byte{byte(i)}, NonceSize))
	}
	join := func(parts ...[]byte) []byte { return bytes.Join(append([][]byte{JournalMagic}, parts...), nil) }
	if got, _, err := key.OpenJournal(rel, join(rec(0), rec(1), rec(2))); err != nil || len(got) != 3 {
		t.Fatalf("journal de référence : %d, %v", len(got), err)
	}
	for name, data := range map[string][]byte{
		"intervertis": join(rec(1), rec(0), rec(2)),
		"retiré":      join(rec(0), rec(2)),
		"autre chemin": func() []byte {
			r := key.SealRecord("pc/BBBB/2026-10.bin", 0, []byte("x"), make([]byte, NonceSize))
			return join(r)
		}(),
		"en-tête":   append([]byte("PHF1"), rec(0)...),
		"longueur":  join([]byte{0, 0, 0, 3, 1, 2, 3}),
		"trop long": join(binary32(NonceSize + MaxRecordBytes + TagSize + 1)),
		"court":     []byte("XY"),
	} {
		if _, _, err := key.OpenJournal(rel, data); !errors.Is(err, ErrCorrupted) {
			t.Errorf("%s : %v", name, err)
		}
	}
	good := join(rec(0), rec(1))
	for i := len(JournalMagic); i < len(good); i++ {
		changed := bytes.Clone(good)
		changed[i] ^= 0x80
		if got, _, err := key.OpenJournal(rel, changed); err == nil && len(got) == 2 {
			t.Fatalf("octet %d modifié accepté", i)
		}
	}

	// Un journal illisible n'est jamais complété.
	if err := os.WriteFile(p, join(rec(1)), 0o600); err != nil {
		t.Fatal(err)
	}
	s4, _ := Open(dir, testKey(1))
	if err := s4.Append(rel, []byte("x")); !errors.Is(err, ErrCorrupted) {
		t.Errorf("ajout à un journal illisible : %v", err)
	}
	if err := s4.Append(rel, make([]byte, MaxRecordBytes+1)); !errors.Is(err, ErrTooLarge) {
		t.Errorf("enregistrement trop grand : %v", err)
	}
}

func binary32(n int) []byte { return []byte{byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)} }

func TestListAndRemove(t *testing.T) {
	s, _ := Open(t.TempDir(), testKey(1))
	for _, rel := range []string{"pc/B/2026-09.bin", "pc/B/2026-10.bin", "pc/A/2026-10.bin"} {
		if err := s.Append(rel, []byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.WriteFile("hub.db", []byte("{}")); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Dirs("pc"); !slices.Equal(got, []string{"A", "B"}) {
		t.Errorf("dossiers : %v", got)
	}
	if got, _ := s.List("pc/B"); !slices.Equal(got, []string{"2026-09.bin", "2026-10.bin"}) {
		t.Errorf("fichiers : %v", got)
	}
	if got, _ := s.List(""); !slices.Equal(got, []string{"hub.db"}) {
		t.Errorf("racine : %v", got)
	}
	if got, err := s.List("absent"); err != nil || got != nil {
		t.Errorf("dossier absent : %v, %v", got, err)
	}
	if err := s.Remove("pc/B/2026-09.bin"); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove("pc/B/2026-09.bin"); err != nil {
		t.Errorf("suppression d'un absent : %v", err)
	}
	if got, _ := s.List("pc/B"); !slices.Equal(got, []string{"2026-10.bin"}) {
		t.Errorf("après suppression : %v", got)
	}
	// Recréé après suppression : la numérotation repart de zéro.
	if err := s.Append("pc/B/2026-09.bin", []byte("y")); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Records("pc/B/2026-09.bin"); err != nil || len(got) != 1 || string(got[0]) != "y" {
		t.Errorf("journal recréé : %q, %v", got, err)
	}
}

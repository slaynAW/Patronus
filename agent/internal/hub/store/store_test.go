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

// TestSymlinksStayInside : un lien symbolique posé dans le dossier des données (par qui peut y écrire,
// docs/HUB.md §9.3) ne fait jamais lire, écrire, tronquer ni supprimer par le hub un fichier situé
// ailleurs, comme la clé des données.
func TestSymlinksStayInside(t *testing.T) {
	base := t.TempDir()
	keys := filepath.Join(base, "keys")
	if _, _, err := LoadOrCreateKey(keys); err != nil {
		t.Fatal(err)
	}
	secret := KeyPath(keys)
	want, err := os.ReadFile(secret)
	if err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(base, "data")
	s, err := Open(data, testKey(1))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	link := func(target, name string) {
		t.Helper()
		p := filepath.Join(data, filepath.FromSlash(name))
		_ = os.Remove(p)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, p); err != nil {
			t.Skipf("liens symboliques indisponibles : %v", err)
		}
	}
	intact := func(step string) {
		t.Helper()
		got, err := os.ReadFile(secret)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("%s : clé des données touchée (%v)", step, err)
		}
	}

	// Fichier temporaire remplacé par un lien vers la clé : retiré, puis un fichier neuf est écrit.
	link(secret, "hub.db.tmp")
	if err := s.WriteFile("hub.db", []byte("{}")); err != nil {
		t.Fatal(err)
	}
	intact("fichier temporaire")
	if got, err := s.ReadFile("hub.db"); err != nil || string(got) != "{}" {
		t.Fatalf("hub.db : %q, %v", got, err)
	}

	// Fichier entier remplacé par un lien : lecture refusée ; l'écriture remplace le lien, pas sa cible.
	link(secret, "hub.db")
	if _, err := s.ReadFile("hub.db"); err == nil {
		t.Error("lien suivi en lecture")
	}
	if err := s.WriteFile("hub.db", []byte("[]")); err != nil {
		t.Fatal(err)
	}
	intact("fichier entier")
	if info, err := os.Lstat(filepath.Join(data, "hub.db")); err != nil || !info.Mode().IsRegular() {
		t.Errorf("hub.db n'est pas redevenu un fichier : %v", err)
	}

	// Journal remplacé par un lien : ni lecture, ni ajout.
	link(secret, "events-2026-11.bin")
	if _, err := s.Records("events-2026-11.bin"); err == nil {
		t.Error("journal lu à travers un lien")
	}
	if err := s.Append("events-2026-11.bin", []byte("x")); err == nil {
		t.Error("ajout à travers un lien")
	}
	intact("journal")

	// Dossier d'un PC remplacé par un lien vers le dossier des clés (« Retirer du hub » supprime les
	// fichiers de ce dossier) : ni liste, ni suppression, ni écriture.
	link(keys, "pc/abc")
	if names, err := s.List("pc/abc"); err == nil {
		t.Errorf("dossier des clés listé : %v", names)
	}
	if err := s.Remove("pc/abc/" + KeyFileName); err == nil {
		t.Error("suppression à travers un lien")
	}
	if err := s.Append("pc/abc/history-2026-11.bin", []byte("x")); err == nil {
		t.Error("journal créé à travers un lien")
	}
	if err := s.WriteFile("pc/abc/hub.db", []byte("{}")); err == nil {
		t.Error("fichier créé à travers un lien")
	}
	if got, _ := s.Dirs("pc"); len(got) != 0 {
		t.Errorf("lien compté comme dossier : %v", got)
	}
	intact("dossier d'un PC")
	if names, err := os.ReadDir(keys); err != nil || len(names) != 1 {
		t.Errorf("dossier des clés modifié : %v, %v", names, err)
	}
}

// TestJournalTornCreation : un journal coupé pendant sa création (en-tête incomplet) est recréé au
// prochain ajout, y compris sous Windows, qui refuse de remplacer un fichier encore ouvert.
func TestJournalTornCreation(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, testKey(1))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const rel = "events-2026-11.bin"
	for _, torn := range [][]byte{{}, []byte("P"), []byte("PHJ")} {
		if err := os.WriteFile(filepath.Join(dir, rel), torn, 0o600); err != nil {
			t.Fatal(err)
		}
		fresh, _ := Open(dir, testKey(1)) // journal pas encore ouvert en écriture
		if err := fresh.Append(rel, []byte("x")); err != nil {
			t.Fatalf("après %q : %v", torn, err)
		}
		if got, err := fresh.Records(rel); err != nil || len(got) != 1 || string(got[0]) != "x" {
			t.Fatalf("après %q : %q, %v", torn, got, err)
		}
		_ = fresh.Close()
	}
}

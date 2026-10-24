package store

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "régénère protocol/hub-store-vectors.json à partir de ce paquet")

var vectorsPath = filepath.Join("..", "..", "..", "..", "protocol", "hub-store-vectors.json")

type vectorFile struct {
	Description string          `json:"description"`
	Key         string          `json:"key"` // clé des données, en hexadécimal
	FileKeys    []fileKeyVector `json:"fileKeys"`
	Files       []fileVector    `json:"files"`
	Journals    []journalVector `json:"journals"`
	Invalid     []invalidVector `json:"invalid"`
}

type fileKeyVector struct {
	Path string `json:"path"`
	Key  string `json:"key"`
}

type fileVector struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Nonce     string `json:"nonce"`
	Plaintext string `json:"plaintext"`
	Sealed    string `json:"sealed"`
}

type journalVector struct {
	Name    string         `json:"name"`
	Path    string         `json:"path"`
	Records []recordVector `json:"records"`
	// Sealed : le journal complet ; Tail : octets d'un enregistrement incomplet ajoutés après lui, ignorés.
	Sealed string `json:"sealed"`
	Tail   string `json:"tail,omitempty"`
}

type recordVector struct {
	Nonce     string `json:"nonce"`
	Plaintext string `json:"plaintext"`
}

type invalidVector struct {
	Name string `json:"name"`
	Kind string `json:"kind"` // "file" ou "journal"
	Path string `json:"path"`
	Data string `json:"data"`
}

func nonceOf(b byte) []byte { return bytes.Repeat([]byte{b}, NonceSize) }

func buildVectors() vectorFile {
	key := testKey(0)
	v := vectorFile{
		Description: "Stockage chiffré du hub (docs/HUB.md, §9) : clé de chaque fichier dérivée par HKDF-SHA256 (sans sel, " +
			"information « patronus-hub/1 file key », un octet nul, puis le chemin), AES-256-GCM. Fichier entier : " +
			"« PHF1 », nonce, texte chiffré et étiquette ; données authentifiées : « PHF1 » puis le chemin. Journal : " +
			"« PHJ1 », puis chaque enregistrement : longueur du reste (4 octets, gros-boutiste), nonce, texte chiffré et " +
			"étiquette ; données authentifiées : « PHJ1 », rang de l'enregistrement (8 octets, gros-boutiste) puis le " +
			"chemin. Produits par agent/internal/hub/store (go test ./internal/hub/store -run TestVectors -update), " +
			"recalculés indépendamment par .github/scripts/check-hub-store-vectors.py.",
		Key: hex.EncodeToString(key[:]),
	}
	paths := []string{"hub.db", "pc/q2Xh0bE9mGk4J1tYw7RfZA/2026-10.bin", "pc/q2Xh0bE9mGk4J1tYw7RfZA/2026-11.bin"}
	for _, p := range paths {
		v.FileKeys = append(v.FileKeys, fileKeyVector{Path: p, Key: hex.EncodeToString(key.FileKey(p))})
	}
	for i, f := range []struct{ name, path, plain string }{
		{"registre du hub", "hub.db", `{"devices":[],"invites":[],"pcs":[{"id":"q2Xh0bE9mGk4J1tYw7RfZA","name":"PC Bureau"}]}`},
		{"fichier vide", "hub.db", ""},
	} {
		nonce := nonceOf(byte(0xa0 + i))
		v.Files = append(v.Files, fileVector{
			Name: f.name, Path: f.path, Nonce: hex.EncodeToString(nonce), Plaintext: f.plain,
			Sealed: hex.EncodeToString(key.SealFile(f.path, []byte(f.plain), nonce)),
		})
	}
	journal := func(name, path string, plains []string, torn bool) journalVector {
		j := journalVector{Name: name, Path: path, Records: []recordVector{}}
		data := bytes.Clone(JournalMagic)
		for i, p := range plains {
			nonce := nonceOf(byte(0x10 + i))
			j.Records = append(j.Records, recordVector{Nonce: hex.EncodeToString(nonce), Plaintext: p})
			data = append(data, key.SealRecord(path, uint64(i), []byte(p), nonce)...)
		}
		j.Sealed = hex.EncodeToString(data)
		if torn {
			full := key.SealRecord(path, uint64(len(plains)), []byte(`{"t":1790000180,"k":"status"}`), nonceOf(0x7f))
			j.Tail = hex.EncodeToString(full[:len(full)-5])
		}
		return j
	}
	month := paths[1]
	rows := []string{`{"t":1790000000,"k":"status","s":"online"}`, `{"t":1790000060,"k":"status","s":"online"}`, ""}
	v.Journals = []journalVector{
		journal("trois enregistrements, dont un vide", month, rows, false),
		journal("journal vide", paths[2], nil, false),
		journal("enregistrement incomplet en fin de journal (coupure de courant)", month, rows[:2], true),
	}

	rec := func(path string, i int, p string) []byte {
		return key.SealRecord(path, uint64(i), []byte(p), nonceOf(byte(0x10+i)))
	}
	sealedFile := key.SealFile("hub.db", []byte(`{"devices":[]}`), nonceOf(0xa0))
	flip := func(b []byte, i int) []byte { c := bytes.Clone(b); c[i] ^= 0x01; return c }
	join := func(parts ...[]byte) []byte { return bytes.Join(append([][]byte{JournalMagic}, parts...), nil) }
	invalid := []struct {
		name, kind, path string
		data             []byte
	}{
		{"fichier lu sous un autre chemin", "file", "hub.bak", sealedFile},
		{"fichier : un octet du texte chiffré modifié", "file", "hub.db", flip(sealedFile, len(FileMagic)+NonceSize)},
		{"fichier : étiquette modifiée", "file", "hub.db", flip(sealedFile, len(sealedFile)-1)},
		{"fichier : nonce modifié", "file", "hub.db", flip(sealedFile, len(FileMagic))},
		{"fichier : en-tête de journal", "file", "hub.db", append(bytes.Clone(JournalMagic), sealedFile[len(FileMagic):]...)},
		{"fichier trop court", "file", "hub.db", sealedFile[:len(FileMagic)+NonceSize+TagSize-1]},
		{"journal : enregistrements intervertis", "journal", month, join(rec(month, 1, rows[1]), rec(month, 0, rows[0]))},
		{"journal : premier enregistrement retiré", "journal", month, join(rec(month, 1, rows[1]))},
		{"journal : enregistrement d'un autre journal", "journal", month, join(rec(paths[2], 0, rows[0]))},
		{"journal : texte chiffré modifié", "journal", month, flip(join(rec(month, 0, rows[0])), len(JournalMagic)+4+NonceSize)},
		{"journal : en-tête de fichier entier", "journal", month, append(bytes.Clone(FileMagic), rec(month, 0, rows[0])...)},
		{"journal : longueur inférieure au nonce et à l'étiquette", "journal", month, join([]byte{0, 0, 0, 27}, make([]byte, 27))},
		{"journal : longueur au-delà de la limite", "journal", month, join(binary32(NonceSize + MaxRecordBytes + TagSize + 1))},
	}
	for _, x := range invalid {
		v.Invalid = append(v.Invalid, invalidVector{Name: x.name, Kind: x.kind, Path: x.path, Data: hex.EncodeToString(x.data)})
	}
	return v
}

// TestVectors vérifie protocol/hub-store-vectors.json. Avec -update, le fichier est régénéré.
func TestVectors(t *testing.T) {
	want := buildVectors()
	raw, err := json.MarshalIndent(want, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	if *update {
		if err := os.WriteFile(vectorsPath, raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	saved, err := os.ReadFile(vectorsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(saved, raw) {
		t.Fatal("protocol/hub-store-vectors.json n'est plus à jour : go test ./internal/hub/store -run TestVectors -update")
	}

	// Relecture des vecteurs par le code de lecture.
	var v vectorFile
	if err := json.Unmarshal(saved, &v); err != nil {
		t.Fatal(err)
	}
	keyBytes, _ := hex.DecodeString(v.Key)
	var key Key
	copy(key[:], keyBytes)
	for _, f := range v.Files {
		data, _ := hex.DecodeString(f.Sealed)
		if got, err := key.OpenFile(f.Path, data); err != nil || string(got) != f.Plaintext {
			t.Errorf("%s : %q, %v", f.Name, got, err)
		}
	}
	for _, j := range v.Journals {
		data, _ := hex.DecodeString(j.Sealed + j.Tail)
		got, end, err := key.OpenJournal(j.Path, data)
		if err != nil || len(got) != len(j.Records) || end != len(j.Sealed)/2 {
			t.Errorf("%s : %d enregistrements, fin %d, %v", j.Name, len(got), end, err)
			continue
		}
		for i, r := range j.Records {
			if string(got[i]) != r.Plaintext {
				t.Errorf("%s, enregistrement %d : %q", j.Name, i, got[i])
			}
		}
	}
	for _, x := range v.Invalid {
		data, _ := hex.DecodeString(x.Data)
		if x.Kind == "file" {
			_, err = key.OpenFile(x.Path, data)
		} else {
			_, _, err = key.OpenJournal(x.Path, data)
		}
		if !errors.Is(err, ErrCorrupted) {
			t.Errorf("%s : accepté (%v)", x.Name, err)
		}
	}
}

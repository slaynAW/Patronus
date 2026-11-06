package store

import (
	"bytes"
	"errors"
	"testing"
)

// Fuzzing : go test ./internal/hub/store -run '^$' -fuzz FuzzJournalIntegrity, par exemple. Sans -fuzz,
// seuls les exemples de départ sont rejoués.

// sealJournal écrit un journal authentique des enregistrements records.
func sealJournal(k Key, rel string, records [][]byte) []byte {
	data := bytes.Clone(JournalMagic)
	for i, r := range records {
		nonce := make([]byte, NonceSize)
		nonce[0], nonce[NonceSize-1] = byte(i), 0xA5
		data = append(data, k.SealRecord(rel, uint64(i), r, nonce)...)
	}
	return data
}

// FuzzOpenJournal : un journal quelconque ne fait jamais paniquer ; un journal accepté se relit à
// l'identique une fois réduit à sa partie complète, qui n'a plus de reste.
func FuzzOpenJournal(f *testing.F) {
	k := testKey(7)
	const rel = "pc/A/history-2026-11.bin"
	valid := sealJournal(k, rel, [][]byte{[]byte("un"), []byte("deux"), {}})
	for _, seed := range [][]byte{nil, []byte("PH"), []byte("PHX"), JournalMagic, valid, valid[:len(valid)-3],
		append(bytes.Clone(valid), 0, 0, 0), append(bytes.Clone(valid), 0xff, 0xff, 0xff, 0xff)} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		records, end, err := k.OpenJournal(rel, data)
		if err != nil {
			if !errors.Is(err, ErrCorrupted) {
				t.Fatalf("erreur inattendue : %v", err)
			}
			return
		}
		if end < 0 || end > len(data) {
			t.Fatalf("partie complète de %d octets sur %d", end, len(data))
		}
		again, end2, err := k.OpenJournal(rel, data[:end])
		if err != nil || end2 != end || len(again) != len(records) {
			t.Fatalf("relecture : %d enregistrements, fin %d, %v (au lieu de %d, %d)", len(again), end2, err, len(records), end)
		}
		for i := range records {
			if !bytes.Equal(again[i], records[i]) {
				t.Fatalf("enregistrement %d relu différemment", i)
			}
		}
	})
}

// FuzzJournalIntegrity : un journal authentique, tronqué ou dont un octet est modifié, ne rend jamais un
// enregistrement altéré. Il rend une erreur, ou le début intact du journal ; une modification est
// toujours visible (erreur ou enregistrements en moins). Seule la fin d'un journal peut disparaître sans
// erreur, comme après une coupure de courant (docs/HUB.md, §9.2).
func FuzzJournalIntegrity(f *testing.F) {
	f.Add([]byte("un|deux|trois"), uint16(10), uint8(1), false)
	f.Add([]byte("un|deux|trois"), uint16(40), uint8(0), true)
	f.Add([]byte("a|b|c|d"), uint16(4), uint8(0x80), false) // longueur du premier enregistrement
	f.Add([]byte(""), uint16(0), uint8(0xff), false)
	f.Fuzz(func(t *testing.T, content []byte, pos uint16, mask uint8, cut bool) {
		k := testKey(3)
		const rel = "events-2026-11.bin"
		parts := bytes.Split(content, []byte("|"))
		if len(parts) > 32 {
			parts = parts[:32]
		}
		data := sealJournal(k, rel, parts)
		mutated := bytes.Clone(data)
		at := int(pos) % (len(data) + 1)
		switch {
		case cut:
			mutated = mutated[:at]
		case mask != 0 && at < len(data):
			mutated[at] ^= mask
		default:
			return
		}
		got, end, err := k.OpenJournal(rel, mutated)
		if err != nil {
			return // altération détectée
		}
		if len(got) > len(parts) || end > len(mutated) {
			t.Fatalf("%d enregistrements rendus sur %d", len(got), len(parts))
		}
		for i := range got {
			if !bytes.Equal(got[i], parts[i]) {
				t.Fatalf("enregistrement %d altéré : %q au lieu de %q", i, got[i], parts[i])
			}
		}
		if !cut && len(got) == len(parts) && end == len(mutated) {
			t.Fatalf("octet %d modifié sans que rien n'y paraisse", at)
		}
		// Le même journal sous un autre chemin, ou chaque enregistrement à un autre rang, est refusé.
		if len(got) > 0 {
			if moved, _, err := k.OpenJournal("pc/B/"+rel, mutated[:end]); err == nil && len(moved) > 0 {
				t.Fatal("journal accepté sous un autre chemin")
			}
		}
	})
}

// FuzzOpenFile : un fichier entier quelconque est refusé (ErrCorrupted) sans paniquer ; un fichier
// authentique dont un octet change aussi.
func FuzzOpenFile(f *testing.F) {
	k := testKey(9)
	const rel = "hub.db"
	valid := k.SealFile(rel, []byte(`{"pcs":[]}`), bytes.Repeat([]byte{7}, NonceSize))
	f.Add(valid, uint16(0), uint8(0))
	f.Add(valid, uint16(20), uint8(1))
	f.Add([]byte("PHF1"), uint16(0), uint8(0))
	f.Fuzz(func(t *testing.T, data []byte, pos uint16, mask uint8) {
		if mask != 0 && len(data) > 0 {
			data = bytes.Clone(data)
			data[int(pos)%len(data)] ^= mask
		}
		plain, err := k.OpenFile(rel, data)
		if err != nil {
			if !errors.Is(err, ErrCorrupted) {
				t.Fatalf("erreur inattendue : %v", err)
			}
			return
		}
		// Accepté : forcément un fichier authentique de ce chemin, dont le contenu se rescelle à l'identique.
		nonce := data[len(FileMagic) : len(FileMagic)+NonceSize]
		if !bytes.Equal(k.SealFile(rel, plain, nonce), data) {
			t.Fatal("fichier accepté qui n'est pas celui que le hub aurait écrit")
		}
		if _, err := k.OpenFile("settings.db", data); err == nil {
			t.Fatal("fichier accepté sous un autre chemin")
		}
	})
}

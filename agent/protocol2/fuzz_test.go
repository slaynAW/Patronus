package protocol2

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

// Fuzzing (§12) : go test ./protocol2 -run '^$' -fuzz FuzzParseLink, par exemple. Sans -fuzz, seuls les
// exemples de départ sont rejoués.

func FuzzReadFrame(f *testing.F) {
	for _, seed := range [][]byte{{0, 0, 0, 2, '{', '}'}, {0, 0, 0, 0}, {0xff, 0xff, 0xff, 0xff}, {0, 0}, {0, 0, 0, 3, 'a'}} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		const limit = 64
		r := bytes.NewReader(data)
		consumed := 0
		for {
			payload, err := ReadFrame(r, limit)
			if err != nil {
				return
			}
			if len(payload) == 0 || len(payload) > limit {
				t.Fatalf("message de %d octets", len(payload))
			}
			var frame bytes.Buffer
			if err := WriteFrame(&frame, payload, limit); err != nil || !bytes.Equal(frame.Bytes(), data[consumed:consumed+frame.Len()]) {
				t.Fatalf("trame réécrite %x (%v)", frame.Bytes(), err)
			}
			consumed += frame.Len()
		}
	})
}

func FuzzParseLink(f *testing.F) {
	raw, err := os.ReadFile(vectorsPath)
	if err != nil {
		f.Fatal(err)
	}
	var doc vectorFile
	if err := json.Unmarshal(raw, &doc); err != nil {
		f.Fatal(err)
	}
	for _, c := range doc.Links.Valid {
		f.Add(c.Link)
	}
	for _, c := range doc.Links.Invalid {
		f.Add(c.Link)
	}
	f.Fuzz(func(t *testing.T, text string) {
		l, err := ParseLink(text)
		if err != nil {
			var linkErr *LinkError
			if !errors.As(err, &linkErr) {
				t.Fatalf("erreur %v", err)
			}
			return
		}
		if l.Name == "" || !ValidHost(l.Host) || l.Port < 1 || l.Port > 65535 {
			t.Fatalf("lien accepté : %+v", l)
		}
		if code, err := NormalizeCode(l.Code); err != nil || code != l.Code {
			t.Fatalf("code %q", l.Code)
		}
		if mac, ok := NormalizeMAC(l.MAC); l.MAC != "" && (!ok || mac != l.MAC) {
			t.Fatalf("MAC %q", l.MAC)
		}
		if again, err := ParseLink(l.String()); err != nil || again != l {
			t.Fatalf("relecture de %s : %+v %v", l, again, err)
		}
	})
}

func FuzzNormalizeCode(f *testing.F) {
	for _, seed := range []string{"7KQ2-9MXP", "oil0-OIL0", "", "7KQ29MXı", " 7kq2 9mxp ", "7KQ2-9MXU"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		code, err := NormalizeCode(input)
		if err != nil {
			return
		}
		if len(code) != CodeLength || strings.Trim(code, crockford) != "" {
			t.Fatalf("%q donne %q", input, code)
		}
		for _, again := range []string{code, FormatCode(code), strings.ToLower(code)} {
			if got, err := NormalizeCode(again); err != nil || got != code {
				t.Fatalf("%q donne %q", again, got)
			}
		}
	})
}

func FuzzParsePin(f *testing.F) {
	f.Add(PinOf(nil).String())
	f.Add(strings.Repeat("A", 43))
	f.Fuzz(func(t *testing.T, s string) {
		// Une seule écriture par empreinte : celle que l'on relit est celle que l'on écrit.
		if p, err := ParsePin(s); err == nil && p.String() != s {
			t.Fatalf("%q relu comme %s", s, p)
		}
	})
}

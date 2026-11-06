package hubprotocol

import (
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/slaynaw/wakeonlan/agent/protocol2"
)

// Fuzzing : go test ./hubprotocol -run '^$' -fuzz FuzzParseLink, par exemple. Sans -fuzz, seuls les
// exemples de départ sont rejoués.

// FuzzParseLink : un lien d'appairage du hub accepté est bien formé, n'annonce qu'un rôle du hub et se
// relit à l'identique une fois réécrit.
func FuzzParseLink(f *testing.F) {
	raw, err := os.ReadFile(vectorsPath)
	if err != nil {
		f.Fatal(err)
	}
	var doc vectorFile
	if err := json.Unmarshal(raw, &doc); err != nil {
		f.Fatal(err)
	}
	for _, c := range append(doc.Links.Valid, doc.Links.Invalid...) {
		f.Add(c.Link)
	}
	f.Fuzz(func(t *testing.T, text string) {
		l, err := ParseLink(text)
		if err != nil {
			var linkErr *protocol2.LinkError
			if !errors.As(err, &linkErr) {
				t.Fatalf("erreur %v", err)
			}
			return
		}
		if l.Name == "" || !protocol2.ValidHost(l.Host) || l.Port < 1 || l.Port > 65535 || l.Role != 0 && !ValidRole(l.Role) {
			t.Fatalf("lien accepté : %+v", l)
		}
		if code, err := protocol2.NormalizeCode(l.Code); err != nil || code != l.Code {
			t.Fatalf("code %q", l.Code)
		}
		if again, err := ParseLink(l.Format(LinkFormat)); err != nil || again != l {
			t.Fatalf("relecture de %s : %+v %v", l.Format(LinkFormat), again, err)
		}
		// Un lien du hub n'est jamais pris pour celui d'un agent, ni l'inverse.
		if _, err := protocol2.ParseLink(text); err == nil {
			t.Fatalf("lien du hub accepté comme lien d'agent : %s", text)
		}
	})
}

// FuzzSlot : une programmation acceptée a une heure « HH:MM » et de 1 à 7 jours distincts, de 1 à 7 ;
// la règle reste la même après un aller-retour JSON (celui du protocole).
func FuzzSlot(f *testing.F) {
	for _, s := range []string{`{"on":true,"days":[1,2,3],"at":"07:30"}`, `{"on":false,"days":[7],"at":"23:59"}`,
		`{"on":true,"days":[1,1],"at":"07:30"}`, `{"on":true,"days":[0],"at":"24:00"}`, `{"on":true,"at":"7:30"}`} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		var s Slot
		if json.Unmarshal(raw, &s) != nil || ValidSlot(s) != nil {
			return
		}
		if len(s.At) != 5 || s.At[2] != ':' || len(s.Days) < 1 || len(s.Days) > 7 {
			t.Fatalf("programmation acceptée : %+v", s)
		}
		seen := map[int]bool{}
		for _, d := range s.Days {
			if d < 1 || d > 7 || seen[d] {
				t.Fatalf("jours acceptés : %v", s.Days)
			}
			seen[d] = true
		}
		out, _ := json.Marshal(s)
		var again Slot
		if json.Unmarshal(out, &again) != nil || ValidSlot(again) != nil {
			t.Fatalf("aller-retour refusé : %s", out)
		}
	})
}

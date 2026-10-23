package protocol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Vecteurs communs avec l'application Android : protocol/measure-vectors.json.
func TestMeasureVectors(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "protocol", "measure-vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Cases []struct {
			Name     string            `json:"name"`
			Input    []json.RawMessage `json:"input"`
			Expected []struct {
				ID    string   `json:"id"`
				Label string   `json:"label"`
				Value float64  `json:"value"`
				Unit  string   `json:"unit"`
				Min   *float64 `json:"min"`
				Max   *float64 `json:"max"`
				Warn  *float64 `json:"warn"`
				Crit  *float64 `json:"crit"`
				Low   bool     `json:"low"`
				Text  string   `json:"text"`
				Level string   `json:"level"`
			} `json:"expected"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	same := func(a, b *float64) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }
	for _, c := range file.Cases {
		got := ParseMeasures(c.Input)
		if len(got) != len(c.Expected) {
			t.Errorf("%s : %d mesures, %d attendues : %+v", c.Name, len(got), len(c.Expected), got)
			continue
		}
		for i, e := range c.Expected {
			g := got[i]
			if g.ID != e.ID || g.Label != e.Label || g.Value != e.Value || g.Unit != e.Unit || g.Low != e.Low || g.Text != e.Text ||
				!same(g.Min, e.Min) || !same(g.Max, e.Max) || !same(g.Warn, e.Warn) || !same(g.Crit, e.Crit) || g.Level() != e.Level {
				t.Errorf("%s, mesure %d : %+v (gravité %q), attendu %+v", c.Name, i, g, g.Level(), e)
			}
		}
	}
	if len(file.Cases) < 5 {
		t.Errorf("%d cas seulement", len(file.Cases))
	}
}

// Ce que l'agent écrit se relit à l'identique.
func TestMeasureRoundTrip(t *testing.T) {
	zero, hundred, warn := 0.0, 100.0, 85.0
	in := []Measure{{ID: "mem", Label: "Mémoire", Value: 41.5, Unit: "%", Min: &zero, Max: &hundred, Warn: &warn, Text: "6,4 Go sur 16 Go"}}
	data, err := json.Marshal(ResponseBody{OK: true, Code: "ok", Measures: in})
	if err != nil {
		t.Fatal(err)
	}
	var back ResponseBody
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	out := back.Measures
	if len(out) != 1 || out[0].ID != "mem" || *out[0].Max != 100 || out[0].Crit != nil || out[0].Level() != LevelNone {
		t.Errorf("%s → %+v", data, out)
	}
}

// Une liste de mesures mal formée ne rend pas la réponse illisible.
func TestMeasureListLenient(t *testing.T) {
	for _, raw := range []string{
		`{"ok":true,"code":"ok","measures":{"id":"mem"}}`,
		`{"ok":true,"code":"ok","measures":"mem"}`,
		`{"ok":true,"code":"ok","measures":[{"id":"mem","label":"Mémoire","value":"41"}, 3]}`,
	} {
		var body ResponseBody
		if err := json.Unmarshal([]byte(raw), &body); err != nil || !body.OK || len(body.Measures) != 0 {
			t.Errorf("%s : %v, %+v", raw, err, body.Measures)
		}
	}
}

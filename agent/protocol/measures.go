package protocol

import (
	"bytes"
	"encoding/json"
	"math"
	"regexp"
	"unicode"
	"unicode/utf8"
)

// Measure est une mesure décrite par l'agent lui-même (réponse à « status », agent 1.13.0) : les
// applications affichent aussi celles qu'elles ne connaissent pas (docs/PROTOCOLE.md, « Mesures »).
// Sens des seuils : une valeur au moins égale à Warn ou Crit est à surveiller ou grave ; avec Low,
// c'est une valeur au plus égale (batterie faible).
type Measure struct {
	// ID : identifiant stable (« mem », « battery »), [a-z0-9._-], 24 caractères au plus.
	ID string `json:"id"`
	// Label : nom affiché (« Mémoire »), 24 caractères au plus.
	Label string  `json:"label"`
	Value float64 `json:"value"`
	// Unit : unité affichée après la valeur (« % », « °C », « tr/min »), 8 caractères au plus.
	Unit string `json:"unit,omitempty"`
	// Min et Max : bornes d'une jauge, ensemble seulement.
	Min *float64 `json:"min,omitempty"`
	Max *float64 `json:"max,omitempty"`
	// Warn et Crit : seuils à surveiller et grave.
	Warn *float64 `json:"warn,omitempty"`
	Crit *float64 `json:"crit,omitempty"`
	Low  bool     `json:"low,omitempty"`
	// Text : précision affichée sous la mesure (« 6,4 Go sur 16 Go »), 32 caractères au plus.
	Text string `json:"text,omitempty"`
}

// Limites des mesures : la réponse à « status » doit tenir dans MaxLineBytes.
const (
	MaxMeasures     = 8
	MaxMeasureLabel = 24
	MaxMeasureUnit  = 8
	MaxMeasureText  = 32
	// maxMeasureValue écarte les valeurs absurdes (|valeur| ≤ 10^12).
	maxMeasureValue = 1e12
)

// Gravité d'une mesure (Measure.Level).
const (
	LevelNone = ""
	LevelWarn = "warn"
	LevelCrit = "crit"
)

var measureID = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,23}$`)

// Level renvoie la gravité de la mesure d'après ses seuils.
func (m Measure) Level() string {
	reached := func(t *float64) bool {
		if t == nil {
			return false
		}
		if m.Low {
			return m.Value <= *t
		}
		return m.Value >= *t
	}
	switch {
	case reached(m.Crit):
		return LevelCrit
	case reached(m.Warn):
		return LevelWarn
	}
	return LevelNone
}

// MeasureList : mesures d'une réponse. À la lecture, elles passent par ParseMeasures et ne causent jamais
// d'erreur : une liste illisible est vide, la réponse reste lisible.
type MeasureList []Measure

// UnmarshalJSON lit la liste élément par élément (ParseMeasures).
func (l *MeasureList) UnmarshalJSON(data []byte) error {
	var raw []json.RawMessage
	if json.Unmarshal(data, &raw) != nil {
		*l = nil
		return nil
	}
	*l = ParseMeasures(raw)
	return nil
}

// ParseMeasures lit les mesures d'une réponse, élément par élément : une mesure invalide est ignorée sans
// rendre la réponse illisible, et un champ facultatif invalide est ignoré (sauf l'unité : sans elle, la
// valeur serait trompeuse). Seule la première mesure d'un identifiant compte ; MaxMeasures au plus. Mêmes
// règles que l'application Android (vecteurs communs : protocol/measure-vectors.json).
func ParseMeasures(raw []json.RawMessage) []Measure {
	var out []Measure
	seen := map[string]bool{}
	for _, item := range raw {
		m, ok := parseMeasure(item)
		if !ok || seen[m.ID] {
			continue
		}
		seen[m.ID] = true
		out = append(out, m)
		if len(out) == MaxMeasures {
			break
		}
	}
	return out
}

func parseMeasure(raw json.RawMessage) (Measure, bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return Measure{}, false
	}
	var m Measure
	id, ok := jsonString(fields["id"])
	if !ok || !measureID.MatchString(id) {
		return Measure{}, false
	}
	label, ok := jsonString(fields["label"])
	if !ok || !validText(label, 1, MaxMeasureLabel) || isBlank(label) {
		return Measure{}, false
	}
	value := jsonNumber(fields["value"])
	if value == nil {
		return Measure{}, false
	}
	m.ID, m.Label, m.Value = id, label, *value
	if raw, present := fields["unit"]; present && !isNull(raw) {
		unit, ok := jsonString(raw)
		if !ok || !validText(unit, 0, MaxMeasureUnit) {
			return Measure{}, false
		}
		m.Unit = unit
	}
	m.Min, m.Max = jsonNumber(fields["min"]), jsonNumber(fields["max"])
	if m.Min == nil || m.Max == nil || *m.Min >= *m.Max {
		m.Min, m.Max = nil, nil
	}
	m.Warn, m.Crit = jsonNumber(fields["warn"]), jsonNumber(fields["crit"])
	if raw, present := fields["low"]; present {
		_ = json.Unmarshal(raw, &m.Low)
	}
	if text, ok := jsonString(fields["text"]); ok && validText(text, 0, MaxMeasureText) {
		m.Text = text
	}
	return m, true
}

func isNull(raw json.RawMessage) bool { return bytes.Equal(bytes.TrimSpace(raw), []byte("null")) }

// jsonString lit une chaîne JSON (pas un nombre, ni null).
func jsonString(raw json.RawMessage) (string, bool) {
	if raw == nil || isNull(raw) {
		return "", false
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return "", false
	}
	return s, true
}

// jsonNumber lit un nombre JSON fini, raisonnable (pas une chaîne, ni null).
func jsonNumber(raw json.RawMessage) *float64 {
	if raw == nil || isNull(raw) {
		return nil
	}
	var v float64
	if json.Unmarshal(raw, &v) != nil || math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > maxMeasureValue {
		return nil
	}
	return &v
}

// validText : entre min et max caractères, sans caractère de contrôle ou de mise en forme (inversion du sens
// de lecture…), ni caractère de remplacement (texte mal encodé).
func validText(s string, min, max int) bool {
	n := utf8.RuneCountInString(s)
	if n < min || n > max {
		return false
	}
	for _, r := range s {
		if r == utf8.RuneError || unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return false
		}
	}
	return true
}

func isBlank(s string) bool {
	for _, r := range s {
		if !unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

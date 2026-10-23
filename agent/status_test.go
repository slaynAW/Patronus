package main

import (
	"testing"

	"github.com/slaynaw/wakeonlan/agent/protocol"
)

func TestDescribeMeasures(t *testing.T) {
	warn, crit := 20.0, 10.0
	got := describeMeasures([]protocol.Measure{
		{ID: "mem", Label: "Mémoire", Value: 41.5, Unit: "%", Text: "6,4 Go sur 16 Go"},
		{ID: "battery", Label: "Batterie", Value: 15, Unit: "%", Warn: &warn, Crit: &crit, Low: true},
		{ID: "fan", Label: "Ventilateur", Value: 1180},
	})
	if want := "Mémoire 41,5 % (6,4 Go sur 16 Go) · Batterie 15 %, À SURVEILLER · Ventilateur 1180"; got != want {
		t.Errorf("%q, attendu %q", got, want)
	}
	if got := describeMeasures(nil); got != "non disponibles sur ce PC" {
		t.Errorf("sans mesure : %q", got)
	}
}

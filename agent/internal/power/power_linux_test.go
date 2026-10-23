package power

import (
	"testing"

	"github.com/slaynaw/wakeonlan/agent/internal/dsm"
)

func TestDSMUnits(t *testing.T) {
	for action, want := range map[Action]string{Shutdown: dsm.PowerOffUnit, Reboot: dsm.RebootUnit} {
		if unit, err := dsmUnit(action); err != nil || unit != want {
			t.Errorf("%s : %q, %v", action, unit, err)
		}
	}
	// Pas de veille sur un NAS : refus explicite plutôt qu'une commande au hasard.
	if unit, err := dsmUnit(Sleep); err == nil {
		t.Errorf("veille acceptée : %q", unit)
	}
}

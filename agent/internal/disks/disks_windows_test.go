package disks

import (
	"strings"
	"testing"
	"time"
)

// Lecture réelle sous Windows (CI) : le lecteur système, la santé des disques et le journal d'événements.
func TestWindowsReadsRealDisks(t *testing.T) {
	v, err := volumes()
	if err != nil || len(v) == 0 {
		t.Fatalf("lecteurs : %+v %v", v, err)
	}
	system := false
	for _, x := range v {
		t.Logf("lecteur %s « %s » %s : %d / %d octets libres", x.Mount, x.Label, x.FS, x.Free, x.Total)
		if strings.EqualFold(x.Mount, "C:") && x.Total > 0 && x.Free <= x.Total {
			system = true
		}
	}
	if !system {
		t.Error("lecteur C: absent")
	}
	d, err := drives()
	if err != nil {
		t.Fatalf("santé des disques (WMI) : %v", err)
	}
	for _, x := range d {
		t.Logf("disque %+v", x)
	}
	if _, _, err := diskErrors(time.Now().Add(-30 * 24 * time.Hour)); err != nil {
		t.Errorf("erreurs de disque (journal d'événements) : %v", err)
	}
}

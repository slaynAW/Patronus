package sensors

import (
	"testing"

	"github.com/slaynaw/wakeonlan/agent/protocol"
)

// Machine de CI Windows : ni carte NVIDIA, ni LibreHardwareMonitor. La lecture (NVML, WMI, serveur
// web) doit échouer proprement et indiquer ce qu'il manque pour le processeur.
func TestReadWithoutSensors(t *testing.T) {
	got := Read()
	t.Logf("relevé : cpu=%v gpu=%v nom=%q indice=%q", value(got.CPU), value(got.GPU), got.GPUName, got.CPUHint)
	if got.CPU == nil && got.CPUHint != protocol.CPUHintLHM {
		t.Errorf("processeur absent sans indication : %+v", got)
	}
	// Deux lectures de suite (COM initialisé puis libéré à chaque fois).
	_ = Read()
}

package sensors

import (
	"testing"
	"unsafe"

	"github.com/slaynaw/wakeonlan/agent/protocol"
)

// Machine de CI Windows : ni carte NVIDIA, ni LibreHardwareMonitor. La lecture (NVML, D3DKMT, WMI,
// serveur web) doit échouer proprement et indiquer ce qu'il manque pour le processeur.
func TestReadWithoutSensors(t *testing.T) {
	got := Read()
	t.Logf("relevé : cpu=%v gpu=%v nom=%q indice=%q lhm=%q", value(got.CPU), value(got.GPU), got.GPUName, got.CPUHint, got.LHM)
	if got.CPU == nil && (got.CPUHint != protocol.CPUHintLHM || got.LHM != protocol.LHMNotRunning) {
		t.Errorf("processeur absent sans indication : %+v", got)
	}
	// Deux lectures de suite (COM initialisé puis libéré à chaque fois).
	_ = Read()
	for _, line := range Details() {
		t.Log(line)
	}
}

// Structures D3DKMT : mêmes tailles et positions qu'en C (Windows 64 bits).
func TestD3DKMTLayout(t *testing.T) {
	var perf d3dkmtAdapterPerfData
	if unsafe.Sizeof(perf) != 64 || unsafe.Offsetof(perf.FanRPM) != 48 || unsafe.Offsetof(perf.Temperature) != 56 {
		t.Errorf("D3DKMT_ADAPTER_PERFDATA : taille %d, Temperature à %d", unsafe.Sizeof(perf), unsafe.Offsetof(perf.Temperature))
	}
	var q d3dkmtQueryAdapterInfo
	if unsafe.Sizeof(q) != 24 || unsafe.Offsetof(q.Data) != 8 || unsafe.Offsetof(q.DataSize) != 16 {
		t.Errorf("D3DKMT_QUERYADAPTERINFO : taille %d", unsafe.Sizeof(q))
	}
	if unsafe.Sizeof(d3dkmtAdapterInfo{}) != 20 || unsafe.Sizeof(d3dkmtEnumAdapters2{}) != 16 {
		t.Errorf("D3DKMT_ADAPTERINFO %d, D3DKMT_ENUMADAPTERS2 %d", unsafe.Sizeof(d3dkmtAdapterInfo{}), unsafe.Sizeof(d3dkmtEnumAdapters2{}))
	}
	if unsafe.Sizeof(d3dkmtAdapterRegistryInfo{}) != 4*260*2 {
		t.Errorf("D3DKMT_ADAPTERREGISTRYINFO : %d", unsafe.Sizeof(d3dkmtAdapterRegistryInfo{}))
	}
	// Lecture réelle : la machine de CI n'a que l'adaptateur logiciel, ignoré.
	for _, a := range adapters() {
		t.Logf("carte : %q, %.1f °C", a.name, a.temp)
	}
}

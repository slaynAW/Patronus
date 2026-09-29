package sensors

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol"
)

func value(p *float64) float64 {
	if p == nil {
		return -1
	}
	return *p
}

func TestPickCPU(t *testing.T) {
	intel := []lhmSensor{
		{"/intelcpu/0/temperature/1", "CPU Core #1", 52},
		{"/intelcpu/0/temperature/7", "Core Max", 58},
		{"/intelcpu/0/temperature/0", "CPU Package", 61.26},
		{"/gpu-nvidia/0/temperature/0", "GPU Core", 47},
		{"/lpc/nct6798d/temperature/0", "CPU", 40},
	}
	if got := value(pickCPU(intel)); got != 61.3 {
		t.Errorf("Intel : %v", got)
	}
	amd := []lhmSensor{
		{"/amdcpu/0/temperature/3", "CCD1 (Tdie)", 55},
		{"/amdcpu/0/temperature/2", "Core (Tctl/Tdie)", 63.5},
	}
	if got := value(pickCPU(amd)); got != 63.5 {
		t.Errorf("AMD : %v", got)
	}
	// Sans capteur connu : le cœur le plus chaud ; valeurs aberrantes ignorées.
	other := []lhmSensor{{"/amdcpu/0/temperature/5", "CCD2", 48}, {"/amdcpu/0/temperature/6", "CCD3", 71}, {"/amdcpu/0/temperature/7", "X", 255}}
	if got := value(pickCPU(other)); got != 71 {
		t.Errorf("secours : %v", got)
	}
	if pickCPU([]lhmSensor{{"/gpu-nvidia/0/temperature/0", "GPU Core", 47}}) != nil {
		t.Error("carte graphique prise pour le processeur")
	}
	if got := value(pickGPU(intel)); got != 47 {
		t.Errorf("GPU : %v", got)
	}
}

func TestParseLHMWeb(t *testing.T) {
	// Arbre de data.json, valeurs formatées en français.
	data := []byte(`{"id":0,"Text":"Sensor","Children":[{"id":1,"Text":"PC-SALON","Children":[
	 {"id":2,"Text":"Intel Core i7-12700K","Children":[{"id":3,"Text":"Temperatures","Children":[
	  {"id":4,"Text":"CPU Package","Value":"64,5 °C","SensorId":"/intelcpu/0/temperature/0","Type":"Temperature"},
	  {"id":5,"Text":"Core Max","Value":"66,0 °C","SensorId":"/intelcpu/0/temperature/6","Type":"Temperature"}]},
	  {"id":6,"Text":"Load","Children":[{"id":7,"Text":"CPU Total","Value":"12,3 %","SensorId":"/intelcpu/0/load/0","Type":"Load"}]}]},
	 {"id":8,"Text":"NVIDIA GeForce RTX 4070","Children":[{"id":9,"Text":"Temperatures","Children":[
	  {"id":10,"Text":"GPU Core","Value":"48.0 °C","SensorId":"/gpu-nvidia/0/temperature/0","Type":"Temperature"}]}]}]}]}`)
	sensors, err := parseLHMWeb(data)
	if err != nil || len(sensors) != 3 {
		t.Fatalf("capteurs : %+v, %v", sensors, err)
	}
	if got := value(pickCPU(sensors)); got != 64.5 {
		t.Errorf("processeur : %v", got)
	}
	if got := value(pickGPU(sensors)); got != 48 {
		t.Errorf("carte graphique : %v", got)
	}
	if _, err := parseLHMWeb([]byte("<html>")); err == nil {
		t.Error("réponse illisible acceptée")
	}
}

func TestFromHwmon(t *testing.T) {
	cpu, gpu := fromHwmon([]hwmonChip{
		{Name: "nvme", Temps: map[string]float64{"Composite": 38}},
		{Name: "coretemp", Temps: map[string]float64{"Core 0": 50, "Package id 0": 57}},
		{Name: "amdgpu", Temps: map[string]float64{"junction": 70, "edge": 62}},
	})
	if value(cpu) != 57 || value(gpu) != 62 {
		t.Errorf("Intel + AMD : %v, %v", value(cpu), value(gpu))
	}
	cpu, gpu = fromHwmon([]hwmonChip{{Name: "k10temp", Temps: map[string]float64{"Tctl": 66, "Tccd1": 60}}})
	if value(cpu) != 66 || gpu != nil {
		t.Errorf("Ryzen : %v, %v", value(cpu), gpu)
	}
	if cpu, gpu := fromHwmon(nil); cpu != nil || gpu != nil {
		t.Error("sans capteur")
	}
}

func TestParseNvidiaSMI(t *testing.T) {
	gpu, name := parseNvidiaSMI("45, NVIDIA GeForce RTX 3060\n67, NVIDIA GeForce RTX 4090\n")
	if value(gpu) != 67 || name != "NVIDIA GeForce RTX 4090" {
		t.Errorf("deux cartes : %v, %q", value(gpu), name)
	}
	if gpu, _ := parseNvidiaSMI("[N/A], NVIDIA\n"); gpu != nil {
		t.Error("valeur non disponible acceptée")
	}
}

func TestCache(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	var reads atomic.Int32
	temp := 55.0
	c := NewCache(func() protocol.Temperatures {
		reads.Add(1)
		return protocol.Temperatures{CPU: &temp}
	})
	c.now = func() time.Time { return now }
	wait := func() {
		for i := 0; i < 200; i++ {
			c.mu.Lock()
			busy := c.refreshing
			c.mu.Unlock()
			if !busy {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}

	// Première demande : rien encore, relevé lancé en arrière-plan.
	if c.Get() != nil {
		t.Fatal("relevé avant la première lecture")
	}
	wait()
	if got := c.Get(); got == nil || *got.CPU != 55 || reads.Load() != 1 {
		t.Fatalf("après lecture : %+v (%d lectures)", got, reads.Load())
	}
	// Demandes rapprochées : pas de nouvelle lecture.
	now = now.Add(2 * time.Second)
	c.Get()
	wait()
	if reads.Load() != 1 {
		t.Errorf("lectures : %d", reads.Load())
	}
	// Relevé vieilli : servi, et renouvelé en arrière-plan.
	now = now.Add(4 * time.Second)
	if c.Get() == nil {
		t.Error("relevé récent non servi")
	}
	wait()
	if reads.Load() != 2 {
		t.Errorf("renouvellement : %d lectures", reads.Load())
	}
	// Plus demandé depuis longtemps : trop ancien pour être servi.
	now = now.Add(time.Minute)
	if c.Get() != nil {
		t.Error("relevé périmé servi")
	}
	wait()
	// Aucun capteur lisible : rien n'est joint à la réponse.
	empty := NewCache(func() protocol.Temperatures { return protocol.Temperatures{} })
	empty.Get()
	for i := 0; i < 200; i++ {
		empty.mu.Lock()
		done := !empty.at.IsZero()
		empty.mu.Unlock()
		if done {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if empty.Get() != nil {
		t.Error("relevé vide servi")
	}
}

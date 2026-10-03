package sensors

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol"
)

func read() protocol.Temperatures {
	var t protocol.Temperatures
	start := time.Now()
	idle0, total0, ok0 := procStat()
	t.CPU, t.GPU = fromHwmon(readHwmon("/sys/class/hwmon"))
	if gpu, load, name := nvidiaSMI(); gpu != nil || load != nil {
		if gpu != nil {
			t.GPU = gpu
		}
		t.GPULoad, t.GPUName = load, name
	}
	if t.GPULoad == nil {
		t.GPULoad = amdgpuBusy("/sys/class/drm")
	}
	shareCPU(&t, "Intel (graphique intégré)", intelIGPU("/sys/bus/pci/devices"))
	// Utilisation du processeur sur une seconde.
	time.Sleep(usageWindow - time.Since(start))
	if idle1, total1, ok1 := procStat(); ok0 && ok1 {
		t.CPULoad = busyPercent(idle0, total0, idle1, total1)
	}
	return t
}

func procStat() (idle, total uint64, ok bool) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, 0, false
	}
	return parseProcStat(string(data))
}

// amdgpuBusy : utilisation des cartes AMD (pilote amdgpu, sous root = /sys/class/drm), la plus
// occupée s'il y en a plusieurs.
func amdgpuBusy(root string) *float64 {
	files, _ := filepath.Glob(filepath.Join(root, "card*", "device", "gpu_busy_percent"))
	var best *float64
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
		if err != nil {
			continue
		}
		if p := percent(v); p != nil && (best == nil || *p > *best) {
			best = p
		}
	}
	return best
}

// intelIGPU indique si la puce graphique intégrée d'un processeur Intel est présente (PCI 00:02.0,
// sous root = /sys/bus/pci/devices) : le noyau ne donne pas sa température.
func intelIGPU(root string) bool {
	dir := filepath.Join(root, "0000:00:02.0")
	vendor, err := os.ReadFile(filepath.Join(dir, "vendor"))
	if err != nil || strings.TrimSpace(string(vendor)) != "0x8086" {
		return false
	}
	class, err := os.ReadFile(filepath.Join(dir, "class"))
	return err == nil && strings.HasPrefix(strings.TrimSpace(string(class)), "0x03") // écran
}

// readHwmon lit les puces de capteurs du noyau sous root (/sys/class/hwmon).
func readHwmon(root string) []hwmonChip {
	dirs, _ := filepath.Glob(filepath.Join(root, "hwmon*"))
	var chips []hwmonChip
	for _, dir := range dirs {
		name, err := os.ReadFile(filepath.Join(dir, "name"))
		if err != nil {
			continue
		}
		chip := hwmonChip{Name: strings.TrimSpace(string(name)), Temps: map[string]float64{}}
		inputs, _ := filepath.Glob(filepath.Join(dir, "temp*_input"))
		for _, input := range inputs {
			raw, err := os.ReadFile(input)
			if err != nil {
				continue
			}
			milli, err := strconv.ParseFloat(strings.TrimSpace(string(raw)), 64)
			if err != nil {
				continue
			}
			base := strings.TrimSuffix(filepath.Base(input), "_input")
			label := base
			if l, err := os.ReadFile(filepath.Join(dir, base+"_label")); err == nil && strings.TrimSpace(string(l)) != "" {
				label = strings.TrimSpace(string(l))
			}
			chip.Temps[label] = milli / 1000
		}
		chips = append(chips, chip)
	}
	return chips
}

// nvidiaSMI interroge les cartes NVIDIA (pilote propriétaire, sans capteur hwmon).
func nvidiaSMI() (temp, load *float64, name string) {
	path, err := exec.LookPath("nvidia-smi")
	if err != nil {
		return nil, nil, ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--query-gpu=temperature.gpu,utilization.gpu,name", "--format=csv,noheader,nounits").Output()
	if err != nil {
		return nil, nil, ""
	}
	return parseNvidiaSMI(string(out))
}

func details() []string { return nil }

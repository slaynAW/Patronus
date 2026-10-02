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
	t.CPU, t.GPU = fromHwmon(readHwmon("/sys/class/hwmon"))
	if gpu, name := nvidiaSMI(); gpu != nil {
		t.GPU, t.GPUName = gpu, name
	}
	return t
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
func nvidiaSMI() (*float64, string) {
	path, err := exec.LookPath("nvidia-smi")
	if err != nil {
		return nil, ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--query-gpu=temperature.gpu,name", "--format=csv,noheader,nounits").Output()
	if err != nil {
		return nil, ""
	}
	return parseNvidiaSMI(string(out))
}

func details() []string { return nil }

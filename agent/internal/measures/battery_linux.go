package measures

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/slaynaw/wakeonlan/agent/protocol"
)

// powerSupplyRoot : alimentations vues par le noyau (remplaçable pour les tests).
var powerSupplyRoot = "/sys/class/power_supply"

// battery lit la première batterie du système (pas celles d'une souris ou d'un clavier sans fil).
func battery() *protocol.Measure {
	dirs, _ := filepath.Glob(filepath.Join(powerSupplyRoot, "*"))
	sort.Strings(dirs)
	for _, dir := range dirs {
		if readText(filepath.Join(dir, "type")) != "Battery" || readText(filepath.Join(dir, "scope")) == "Device" {
			continue
		}
		percent, err := strconv.Atoi(readText(filepath.Join(dir, "capacity")))
		if err != nil {
			continue
		}
		return batteryMeasure(percent, linuxBatteryState(readText(filepath.Join(dir, "status"))))
	}
	return nil
}

// linuxBatteryState traduit l'état donné par le noyau.
func linuxBatteryState(status string) string {
	switch status {
	case "Charging":
		return charging
	case "Discharging":
		return discharging
	case "Full":
		return full
	case "Not charging":
		return pluggedIn
	}
	return ""
}

func readText(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

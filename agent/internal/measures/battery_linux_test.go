package measures

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLinuxBattery(t *testing.T) {
	root := t.TempDir()
	saved := powerSupplyRoot
	t.Cleanup(func() { powerSupplyRoot = saved })
	powerSupplyRoot = root
	write := func(dev, name, value string) {
		if err := os.MkdirAll(filepath.Join(root, dev), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, dev, name), []byte(value+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if battery() != nil {
		t.Error("batterie sans alimentation")
	}
	write("AC", "type", "Mains")
	// Souris sans fil : pas la batterie du PC.
	write("hidpp_battery_0", "type", "Battery")
	write("hidpp_battery_0", "scope", "Device")
	write("hidpp_battery_0", "capacity", "5")
	if battery() != nil {
		t.Error("batterie d'une souris prise pour celle du PC")
	}
	write("BAT0", "type", "Battery")
	write("BAT0", "capacity", "64")
	write("BAT0", "status", "Charging")
	if m := battery(); m == nil || m.Value != 64 || m.Text != "en charge" {
		t.Errorf("%+v", m)
	}
}

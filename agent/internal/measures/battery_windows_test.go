package measures

import "testing"

func TestWindowsBattery(t *testing.T) {
	if windowsBattery(systemPowerStatus{BatteryFlag: 128, BatteryLifePercent: 255}) != nil {
		t.Error("PC fixe avec une batterie")
	}
	if m := windowsBattery(systemPowerStatus{ACLineStatus: 1, BatteryFlag: 8, BatteryLifePercent: 42}); m == nil || m.Value != 42 || m.Text != "en charge" {
		t.Errorf("en charge : %+v", m)
	}
	if m := windowsBattery(systemPowerStatus{ACLineStatus: 0, BatteryFlag: 2, BatteryLifePercent: 9}); m == nil || m.Text != "sur batterie" {
		t.Errorf("sur batterie : %+v", m)
	}
}

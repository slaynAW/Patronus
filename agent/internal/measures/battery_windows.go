package measures

import (
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/slaynaw/wakeonlan/agent/protocol"
)

var getSystemPowerStatus = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetSystemPowerStatus")

// systemPowerStatus : structure SYSTEM_POWER_STATUS de Windows.
type systemPowerStatus struct {
	ACLineStatus        byte
	BatteryFlag         byte
	BatteryLifePercent  byte
	SystemStatusFlag    byte
	BatteryLifeTime     uint32
	BatteryFullLifeTime uint32
}

// battery lit la batterie d'un portable ; rien sur un PC fixe.
func battery() *protocol.Measure {
	var s systemPowerStatus
	if ok, _, _ := getSystemPowerStatus.Call(uintptr(unsafe.Pointer(&s))); ok == 0 {
		return nil
	}
	return windowsBattery(s)
}

// windowsBattery traduit l'état de Windows : 128 = pas de batterie, 255 = inconnu.
func windowsBattery(s systemPowerStatus) *protocol.Measure {
	if s.BatteryFlag == 128 || s.BatteryFlag == 255 || s.BatteryLifePercent > 100 {
		return nil
	}
	state := ""
	switch {
	case s.BatteryFlag&8 != 0:
		state = charging
	case s.ACLineStatus == 1:
		state = pluggedIn
	case s.ACLineStatus == 0:
		state = discharging
	}
	return batteryMeasure(int(s.BatteryLifePercent), state)
}

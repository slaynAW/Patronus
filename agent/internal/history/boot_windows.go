package history

import (
	"strconv"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	kernel32                       = windows.NewLazySystemDLL("kernel32.dll")
	procGetTickCount64             = kernel32.NewProc("GetTickCount64")
	procQueryUnbiasedInterruptTime = kernel32.NewProc("QueryUnbiasedInterruptTime")
	prefetchParametersKey          = `SYSTEM\CurrentControlSet\Control\Session Manager\Memory Management\PrefetchParameters`
)

// BootID renvoie le compteur de démarrages de Windows (incrémenté à chaque démarrage).
func BootID() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, prefetchParametersKey, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("BootId")
	if err != nil {
		return ""
	}
	return strconv.FormatUint(v, 10)
}

// SuspendedTotal renvoie le temps passé en veille ou en veille prolongée depuis le démarrage :
// GetTickCount64 compte ce temps, QueryUnbiasedInterruptTime non.
func SuspendedTotal() (time.Duration, bool) {
	if procGetTickCount64.Find() != nil || procQueryUnbiasedInterruptTime.Find() != nil {
		return 0, false
	}
	var unbiased uint64 // unités de 100 ns
	if ok, _, _ := procQueryUnbiasedInterruptTime.Call(uintptr(unsafe.Pointer(&unbiased))); ok == 0 {
		return 0, false
	}
	ticks, _, _ := procGetTickCount64.Call()
	total := time.Duration(ticks)*time.Millisecond - time.Duration(unbiased)*100
	if total < 0 {
		total = 0
	}
	return total, true
}

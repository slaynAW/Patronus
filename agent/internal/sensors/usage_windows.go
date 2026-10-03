package sensors

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Utilisation du processeur et des cartes graphiques, lue dans les compteurs de performance de
// Windows (PDH) que le Gestionnaire des tâches affiche :
//   - processeur : « Processor Information(_Total)\% Processor Utility » (travail réellement fait,
//     selon la fréquence ; plafonné à 100 comme le Gestionnaire), à défaut le temps d'activité
//     (GetSystemTimes) ;
//   - cartes graphiques : « GPU Engine(*)\Utilization Percentage » (Windows 10 1709 ou plus).
//
// Les noms anglais des compteurs sont utilisés, quelle que soit la langue de Windows.
var (
	pdh                      = windows.NewLazySystemDLL("pdh.dll")
	procPdhOpenQuery         = pdh.NewProc("PdhOpenQueryW")
	procPdhAddEnglishCounter = pdh.NewProc("PdhAddEnglishCounterW")
	procPdhCollectQueryData  = pdh.NewProc("PdhCollectQueryData")
	procPdhGetFormattedValue = pdh.NewProc("PdhGetFormattedCounterValue")
	procPdhGetFormattedArray = pdh.NewProc("PdhGetFormattedCounterArrayW")
	procPdhCloseQuery        = pdh.NewProc("PdhCloseQuery")
	procGetSystemTimes       = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetSystemTimes")
	usageMu                  sync.Mutex
)

const (
	pdhFmtDouble   = 0x00000200
	pdhFmtNoCap100 = 0x00008000
	pdhMoreData    = 0x800007D2

	counterCPUUtility = `\Processor Information(_Total)\% Processor Utility`
	counterGPUEngine  = `\GPU Engine(*)\Utilization Percentage`
)

// PDH_FMT_COUNTERVALUE (valeur double)
type pdhCounterValue struct {
	CStatus uint32
	_       uint32
	Value   float64
}

// PDH_FMT_COUNTERVALUE_ITEM_W
type pdhCounterValueItem struct {
	Name  *uint16
	Value pdhCounterValue
}

// usageReading : utilisation mesurée ; gpus n'est renseigné que si le compteur des cartes
// graphiques existe (une carte absente de gpus est alors inactive).
type usageReading struct {
	cpu       *float64
	cpuSource string
	gpus      map[uint64]float64
	gpuErr    error
}

// measureUsage mesure l'utilisation pendant usageWindow.
func measureUsage() usageReading {
	usageMu.Lock()
	defer usageMu.Unlock()
	var u usageReading
	query, err := pdhOpenQuery()
	if err != nil {
		u.gpuErr = err
		idle0, total0, ok0 := systemTimes()
		time.Sleep(usageWindow)
		if idle1, total1, ok1 := systemTimes(); ok0 && ok1 {
			u.cpu, u.cpuSource = busyPercent(idle0, total0, idle1, total1), "temps d'activité"
		}
		return u
	}
	defer procPdhCloseQuery.Call(query)
	cpuCounter, cpuErr := pdhAddCounter(query, counterCPUUtility)
	gpuCounter, gpuErr := pdhAddCounter(query, counterGPUEngine)
	idle0, total0, ok0 := systemTimes()
	_, _, _ = procPdhCollectQueryData.Call(query)
	time.Sleep(usageWindow)
	r, _, _ := procPdhCollectQueryData.Call(query)
	idle1, total1, ok1 := systemTimes()
	if cpuErr == nil && r == 0 {
		if v, err := pdhValue(cpuCounter); err == nil {
			u.cpu, u.cpuSource = percent(v), "% Processor Utility"
		}
	}
	if u.cpu == nil && ok0 && ok1 {
		u.cpu, u.cpuSource = busyPercent(idle0, total0, idle1, total1), "temps d'activité"
	}
	switch {
	case gpuErr != nil:
		u.gpuErr = gpuErr
	case r != 0:
		u.gpuErr = fmt.Errorf("PdhCollectQueryData : 0x%08X", uint32(r))
	default:
		samples, err := pdhArray(gpuCounter)
		if err != nil {
			u.gpuErr = err
		} else {
			u.gpus = gpuEngineLoads(samples)
		}
	}
	return u
}

func pdhOpenQuery() (uintptr, error) {
	if err := procPdhOpenQuery.Find(); err != nil {
		return 0, err
	}
	var query uintptr
	if r, _, _ := procPdhOpenQuery.Call(0, 0, uintptr(unsafe.Pointer(&query))); r != 0 {
		return 0, fmt.Errorf("PdhOpenQuery : 0x%08X", uint32(r))
	}
	return query, nil
}

func pdhAddCounter(query uintptr, path string) (uintptr, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var counter uintptr
	if r, _, _ := procPdhAddEnglishCounter.Call(query, uintptr(unsafe.Pointer(p)), 0, uintptr(unsafe.Pointer(&counter))); r != 0 {
		return 0, fmt.Errorf("compteur %s absent (0x%08X)", path, uint32(r))
	}
	return counter, nil
}

func pdhValue(counter uintptr) (float64, error) {
	var v pdhCounterValue
	r, _, _ := procPdhGetFormattedValue.Call(counter, pdhFmtDouble|pdhFmtNoCap100, 0, uintptr(unsafe.Pointer(&v)))
	if r != 0 || v.CStatus > 1 { // PDH_CSTATUS_VALID_DATA, PDH_CSTATUS_NEW_DATA
		return 0, fmt.Errorf("valeur illisible (0x%08X, état %d)", uint32(r), v.CStatus)
	}
	return v.Value, nil
}

// pdhArray lit toutes les instances d'un compteur à joker (« (*) »).
func pdhArray(counter uintptr) ([]engineSample, error) {
	for range 3 { // les instances peuvent changer entre les deux appels
		var size, count uint32
		r, _, _ := procPdhGetFormattedArray.Call(counter, pdhFmtDouble|pdhFmtNoCap100, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), 0)
		if r == 0 && count == 0 {
			return nil, nil
		}
		if uint32(r) != pdhMoreData || size == 0 {
			return nil, fmt.Errorf("PdhGetFormattedCounterArray : 0x%08X", uint32(r))
		}
		buf := make([]uint64, (size+7)/8) // aligné sur 8 octets
		r, _, _ = procPdhGetFormattedArray.Call(counter, pdhFmtDouble|pdhFmtNoCap100, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&buf[0])))
		if uint32(r) == pdhMoreData {
			continue
		}
		if r != 0 {
			return nil, fmt.Errorf("PdhGetFormattedCounterArray : 0x%08X", uint32(r))
		}
		if uintptr(count)*unsafe.Sizeof(pdhCounterValueItem{}) > uintptr(len(buf))*8 {
			return nil, errors.New("réponse PDH incohérente")
		}
		items := unsafe.Slice((*pdhCounterValueItem)(unsafe.Pointer(&buf[0])), count)
		out := make([]engineSample, 0, count)
		for _, it := range items {
			if it.Value.CStatus > 1 || it.Name == nil {
				continue
			}
			out = append(out, engineSample{instance: windows.UTF16PtrToString(it.Name), value: it.Value.Value})
		}
		runtime.KeepAlive(buf)
		return out, nil
	}
	return nil, errors.New("PdhGetFormattedCounterArray : instances instables")
}

// systemTimes : temps d'inactivité et temps total (noyau, inactivité comprise, + utilisateur) de
// tous les processeurs, en centaines de nanosecondes.
func systemTimes() (idle, total uint64, ok bool) {
	var i, k, u windows.Filetime
	if r, _, _ := procGetSystemTimes.Call(uintptr(unsafe.Pointer(&i)), uintptr(unsafe.Pointer(&k)), uintptr(unsafe.Pointer(&u))); r == 0 {
		return 0, 0, false
	}
	ft := func(f windows.Filetime) uint64 { return uint64(f.HighDateTime)<<32 | uint64(f.LowDateTime) }
	return ft(i), ft(k) + ft(u), true
}

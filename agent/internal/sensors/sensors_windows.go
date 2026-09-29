package sensors

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"golang.org/x/sys/windows"

	"github.com/slaynaw/wakeonlan/agent/protocol"
)

func read() protocol.Temperatures {
	var t protocol.Temperatures
	t.GPU, t.GPUName = nvidiaGPU()
	sensors, err := lhmSensors()
	if err != nil {
		t.CPUHint = protocol.CPUHintLHM
		return t
	}
	t.CPU = pickCPU(sensors)
	if t.CPU == nil {
		t.CPUHint = protocol.CPUHintLHM
	}
	if t.GPU == nil {
		t.GPU = pickGPU(sensors)
	}
	return t
}

// --- NVML (cartes NVIDIA) ---

var nvml struct {
	once        sync.Once
	ok          bool
	deviceCount *windows.LazyProc
	handle      *windows.LazyProc
	temperature *windows.LazyProc
	name        *windows.LazyProc
}

// loadNVML charge la bibliothèque installée par le pilote NVIDIA : dans System32 (pilotes récents),
// sinon dans le dossier NVSMI des anciens pilotes. Jamais depuis un dossier modifiable par l'utilisateur.
func loadNVML() {
	candidates := []*windows.LazyDLL{windows.NewLazySystemDLL("nvml.dll")}
	if pf := os.Getenv("ProgramW6432"); pf != "" {
		candidates = append(candidates, windows.NewLazyDLL(filepath.Join(pf, "NVIDIA Corporation", "NVSMI", "nvml.dll")))
	}
	for _, dll := range candidates {
		if dll.Load() != nil {
			continue
		}
		initProc := dll.NewProc("nvmlInit_v2")
		if initProc.Find() != nil {
			continue
		}
		if r, _, _ := initProc.Call(); r != 0 {
			continue
		}
		nvml.deviceCount = dll.NewProc("nvmlDeviceGetCount_v2")
		nvml.handle = dll.NewProc("nvmlDeviceGetHandleByIndex_v2")
		nvml.temperature = dll.NewProc("nvmlDeviceGetTemperature")
		nvml.name = dll.NewProc("nvmlDeviceGetName")
		nvml.ok = nvml.deviceCount.Find() == nil && nvml.handle.Find() == nil && nvml.temperature.Find() == nil && nvml.name.Find() == nil
		return
	}
}

// nvidiaGPU renvoie la température de la carte NVIDIA la plus chaude et son nom.
func nvidiaGPU() (*float64, string) {
	nvml.once.Do(loadNVML)
	if !nvml.ok {
		return nil, ""
	}
	var count uint32
	if r, _, _ := nvml.deviceCount.Call(uintptr(unsafe.Pointer(&count))); r != 0 {
		return nil, ""
	}
	var best *float64
	var bestName string
	for i := uint32(0); i < count && i < 8; i++ {
		var device uintptr
		if r, _, _ := nvml.handle.Call(uintptr(i), uintptr(unsafe.Pointer(&device))); r != 0 {
			continue
		}
		var temp uint32
		if r, _, _ := nvml.temperature.Call(device, 0 /* NVML_TEMPERATURE_GPU */, uintptr(unsafe.Pointer(&temp))); r != 0 {
			continue
		}
		t := celsius(float64(temp))
		if t == nil || (best != nil && *t <= *best) {
			continue
		}
		var buf [96]byte
		name := ""
		if r, _, _ := nvml.name.Call(device, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf))); r == 0 {
			name = strings.TrimRight(string(buf[:]), "\x00")
		}
		best, bestName = t, name
	}
	return best, bestName
}

// --- LibreHardwareMonitor ---

// lhmSensors lit les températures publiées par LibreHardwareMonitor : par WMI (espace de noms
// root\LibreHardwareMonitor, publié tant que le programme tourne en administrateur), sinon par son
// serveur web local s'il est activé.
func lhmSensors() ([]lhmSensor, error) {
	sensors, err := lhmWMI()
	if err == nil && len(sensors) > 0 {
		return sensors, nil
	}
	if web, webErr := lhmWeb(); webErr == nil && len(web) > 0 {
		return web, nil
	}
	if err == nil {
		err = errors.New("aucun capteur de température")
	}
	return nil, err
}

func lhmWMI() (sensors []lhmSensor, err error) {
	// COM exige un fil d'exécution système fixe, initialisé pour COM.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		var oleErr *ole.OleError
		if !errors.As(err, &oleErr) || oleErr.Code() != 1 { // S_FALSE : déjà initialisé
			return nil, err
		}
	}
	defer ole.CoUninitialize()

	unknown, err := oleutil.CreateObject("WbemScripting.SWbemLocator")
	if err != nil {
		return nil, err
	}
	defer unknown.Release()
	locator, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return nil, err
	}
	defer locator.Release()
	serviceRaw, err := oleutil.CallMethod(locator, "ConnectServer", nil, `root\LibreHardwareMonitor`)
	if err != nil {
		return nil, err // espace de noms absent : LibreHardwareMonitor ne tourne pas
	}
	defer serviceRaw.Clear()
	service := serviceRaw.ToIDispatch()
	resultRaw, err := oleutil.CallMethod(service, "ExecQuery", "SELECT Identifier, Name, Value FROM Sensor WHERE SensorType = 'Temperature'")
	if err != nil {
		return nil, err
	}
	defer resultRaw.Clear()
	result := resultRaw.ToIDispatch()
	countRaw, err := oleutil.GetProperty(result, "Count")
	if err != nil {
		return nil, err
	}
	count := int(countRaw.Val)
	_ = countRaw.Clear()
	for i := 0; i < count && i < 256; i++ {
		itemRaw, err := oleutil.CallMethod(result, "ItemIndex", i)
		if err != nil {
			continue
		}
		item := itemRaw.ToIDispatch()
		s := lhmSensor{Identifier: stringProperty(item, "Identifier"), Name: stringProperty(item, "Name")}
		if v, ok := floatProperty(item, "Value"); ok {
			s.Value = v
			sensors = append(sensors, s)
		}
		_ = itemRaw.Clear()
	}
	return sensors, nil
}

func stringProperty(item *ole.IDispatch, name string) string {
	v, err := oleutil.GetProperty(item, name)
	if err != nil {
		return ""
	}
	defer v.Clear()
	s, _ := v.Value().(string)
	return s
}

func floatProperty(item *ole.IDispatch, name string) (float64, bool) {
	v, err := oleutil.GetProperty(item, name)
	if err != nil {
		return 0, false
	}
	defer v.Clear()
	switch x := v.Value().(type) {
	case float32:
		return float64(x), true
	case float64:
		return x, true
	}
	return 0, false
}

// lhmWeb lit le serveur web local de LibreHardwareMonitor (option « Remote Web Server », port 8085).
func lhmWeb() ([]lhmSensor, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:8085/data.json", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	return parseLHMWeb(data)
}

package sensors

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
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
	"golang.org/x/sys/windows/registry"

	"github.com/slaynaw/wakeonlan/agent/protocol"
)

func read() protocol.Temperatures {
	var t protocol.Temperatures
	// Utilisation mesurée pendant une seconde, en même temps que les autres lectures.
	measured := make(chan usageReading, 1)
	go func() { measured <- measureUsage() }()
	list := adapters()
	readings := nvidiaGPUs()
	integrated, hasIntegrated := "", false
	for _, a := range list {
		if a.temp > 0 {
			readings = append(readings, gpuReading{temp: a.temp, name: a.name})
		} else if a.integrated && !hasIntegrated {
			integrated, hasIntegrated = a.name, true
		}
	}
	t.GPU, t.GPUName = hottestGPU(readings)
	lhm := readLHM(nil)
	t.CPU = pickCPU(lhm.sensors)
	if t.CPU == nil {
		t.CPUHint, t.LHM = protocol.CPUHintLHM, lhm.state
	}
	if t.GPU == nil {
		t.GPU, t.GPUName = pickGPU(lhm.sensors)
	}
	shareCPU(&t, integrated, hasIntegrated)
	usage := <-measured
	t.CPULoad = usage.cpu
	cards := make([]loadCard, 0, len(list))
	for _, a := range list {
		cards = append(cards, loadCard{name: a.name, luid: a.luid})
	}
	if load, name := pickGPULoad(cards, usage.gpus, t.GPUName); load != nil {
		t.GPULoad = load
		if t.GPUName == "" {
			t.GPUName = name
		}
	}
	return t
}

func details() []string {
	var lines []string
	note := func(format string, args ...any) { lines = append(lines, fmt.Sprintf(format, args...)) }
	for _, g := range nvidiaGPUs() {
		note("NVML : %s, %.0f °C", g.name, g.temp)
	}
	list := adapters()
	for _, a := range list {
		switch {
		case a.temp > 0:
			note("Carte graphique : %s, %.0f °C", a.name, a.temp)
		case a.integrated:
			note("Carte graphique : %s, intégrée au processeur, sans sonde à part : la température de la puce (processeur) est affichée", a.name)
		default:
			note("Carte graphique : %s, température non fournie par le pilote (pilote trop ancien ?)", a.name)
		}
	}
	if len(list) == 0 {
		note("Cartes graphiques : aucune trouvée par Windows")
	}
	usage := measureUsage()
	if usage.cpu != nil {
		note("Utilisation du processeur : %.0f %% (%s, comme le Gestionnaire des tâches)", *usage.cpu, usage.cpuSource)
	} else {
		note("Utilisation du processeur : illisible")
	}
	if usage.gpuErr != nil {
		note("Utilisation des cartes graphiques : illisible (%v)", usage.gpuErr)
	} else {
		for _, a := range list {
			note("Utilisation de %s : %.0f %%", a.name, min(usage.gpus[a.luid], 100))
		}
	}
	readLHM(note)
	if v, err := pawnIOVersion(); err == nil {
		note("Pilote PawnIO (utilisé par LibreHardwareMonitor 0.9.5 ou plus) : installé, version %s", v)
	} else {
		note("Pilote PawnIO (utilisé par LibreHardwareMonitor 0.9.5 ou plus) : absent")
	}
	return lines
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

// nvidiaGPUs renvoie la température et le nom de chaque carte NVIDIA.
func nvidiaGPUs() []gpuReading {
	nvml.once.Do(loadNVML)
	if !nvml.ok {
		return nil
	}
	var count uint32
	if r, _, _ := nvml.deviceCount.Call(uintptr(unsafe.Pointer(&count))); r != 0 {
		return nil
	}
	var out []gpuReading
	for i := uint32(0); i < count && i < 8; i++ {
		var device uintptr
		if r, _, _ := nvml.handle.Call(uintptr(i), uintptr(unsafe.Pointer(&device))); r != 0 {
			continue
		}
		var temp uint32
		if r, _, _ := nvml.temperature.Call(device, 0 /* NVML_TEMPERATURE_GPU */, uintptr(unsafe.Pointer(&temp))); r != 0 {
			continue
		}
		var buf [96]byte
		name := ""
		if r, _, _ := nvml.name.Call(device, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf))); r == 0 {
			name = strings.TrimRight(string(buf[:]), "\x00")
		}
		out = append(out, gpuReading{temp: float64(temp), name: name})
	}
	return out
}

// --- Cartes graphiques : interface du noyau graphique (D3DKMT) ---

// Lecture de la température comme le Gestionnaire des tâches : D3DKMTQueryAdapterInfo,
// KMTQAITYPE_ADAPTERPERFDATA (pilotes WDDM 2.4 ou plus). Toutes marques, sans pilote ni programme
// en plus ; les puces intégrées ne la fournissent généralement pas.
var (
	gdi32               = windows.NewLazySystemDLL("gdi32.dll")
	procEnumAdapters2   = gdi32.NewProc("D3DKMTEnumAdapters2")
	procQueryAdapterInf = gdi32.NewProc("D3DKMTQueryAdapterInfo")
	procCloseAdapter    = gdi32.NewProc("D3DKMTCloseAdapter")
)

const (
	kmtqaiAdapterAddress      = 6  // KMTQAITYPE_ADAPTERADDRESS
	kmtqaiAdapterRegistryInfo = 8  // KMTQAITYPE_ADAPTERREGISTRYINFO
	kmtqaiAdapterPerfData     = 62 // KMTQAITYPE_ADAPTERPERFDATA
	maxAdapters               = 16
)

// D3DKMT_ADAPTERINFO
type d3dkmtAdapterInfo struct {
	Adapter               uint32
	LUIDLow               uint32
	LUIDHigh              int32
	NumOfSources          uint32
	PrecisePresentRegions int32
}

// D3DKMT_ENUMADAPTERS2
type d3dkmtEnumAdapters2 struct {
	NumAdapters uint32
	Adapters    *d3dkmtAdapterInfo
}

// D3DKMT_QUERYADAPTERINFO
type d3dkmtQueryAdapterInfo struct {
	Adapter  uint32
	Type     uint32
	Data     unsafe.Pointer
	DataSize uint32
}

// D3DKMT_ADAPTER_PERFDATA (Temperature : dixièmes de °C)
type d3dkmtAdapterPerfData struct {
	PhysicalAdapterIndex uint32
	MemoryFrequency      uint64
	MaxMemoryFrequency   uint64
	MaxMemoryFrequencyOC uint64
	MemoryBandwidth      uint64
	PCIEBandwidth        uint64
	FanRPM               uint32
	Power                uint32
	Temperature          uint32
	PowerStateOverride   uint8
}

// D3DKMT_ADAPTERREGISTRYINFO
type d3dkmtAdapterRegistryInfo struct {
	AdapterString [260]uint16
	BiosString    [260]uint16
	DacType       [260]uint16
	ChipType      [260]uint16
}

// adapter : carte graphique vue par Windows ; temp vaut 0 si le pilote ne la donne pas.
type adapter struct {
	name       string
	temp       float64
	integrated bool   // puce graphique intégrée au processeur
	luid       uint64 // identifiant de la carte (compteurs « GPU Engine »)
}

// adapters liste les cartes graphiques matérielles et leur température.
func adapters() []adapter {
	// Structures prévues pour Windows 64 bits (seules versions de l'agent pour Windows).
	if unsafe.Sizeof(d3dkmtAdapterPerfData{}) != 64 || unsafe.Sizeof(d3dkmtQueryAdapterInfo{}) != 24 {
		return nil
	}
	if procEnumAdapters2.Find() != nil || procQueryAdapterInf.Find() != nil || procCloseAdapter.Find() != nil {
		return nil
	}
	var enum d3dkmtEnumAdapters2
	if r, _, _ := procEnumAdapters2.Call(uintptr(unsafe.Pointer(&enum))); r != 0 || enum.NumAdapters == 0 {
		return nil
	}
	list := make([]d3dkmtAdapterInfo, min(enum.NumAdapters, maxAdapters))
	enum = d3dkmtEnumAdapters2{NumAdapters: uint32(len(list)), Adapters: &list[0]}
	r, _, _ := procEnumAdapters2.Call(uintptr(unsafe.Pointer(&enum)))
	runtime.KeepAlive(list)
	if r != 0 {
		return nil
	}
	list = list[:min(int(enum.NumAdapters), len(list))]
	defer func() {
		for _, a := range list {
			closeAdapter := struct{ Adapter uint32 }{a.Adapter}
			_, _, _ = procCloseAdapter.Call(uintptr(unsafe.Pointer(&closeAdapter)))
		}
	}()
	var out []adapter
	for _, a := range list {
		var reg d3dkmtAdapterRegistryInfo
		name := ""
		if queryAdapter(a.Adapter, kmtqaiAdapterRegistryInfo, unsafe.Pointer(&reg), unsafe.Sizeof(reg)) {
			name = strings.TrimSpace(windows.UTF16ToString(reg.AdapterString[:]))
		}
		if strings.Contains(name, "Basic Render") || strings.Contains(name, "Basic Display") {
			continue // adaptateur logiciel de Windows
		}
		var perf d3dkmtAdapterPerfData
		temp := 0.0
		if queryAdapter(a.Adapter, kmtqaiAdapterPerfData, unsafe.Pointer(&perf), unsafe.Sizeof(perf)) && perf.Temperature > 0 && perf.Temperature < 1500 {
			temp = float64(perf.Temperature) / 10
		}
		if name == "" && temp == 0 {
			continue
		}
		// D3DKMT_ADAPTERADDRESS : emplacement PCI (bus, périphérique, fonction).
		var addr pciAddress
		where := &addr
		if !queryAdapter(a.Adapter, kmtqaiAdapterAddress, unsafe.Pointer(&addr), unsafe.Sizeof(addr)) {
			where = nil
		}
		out = append(out, adapter{name: name, temp: temp, integrated: integratedGPU(name, where),
			luid: uint64(uint32(a.LUIDHigh))<<32 | uint64(a.LUIDLow)})
	}
	return out
}

func queryAdapter(handle, kind uint32, data unsafe.Pointer, size uintptr) bool {
	q := d3dkmtQueryAdapterInfo{Adapter: handle, Type: kind, Data: data, DataSize: uint32(size)}
	r, _, _ := procQueryAdapterInf.Call(uintptr(unsafe.Pointer(&q)))
	return r == 0 // STATUS_SUCCESS
}

// lhmReading : ce que LibreHardwareMonitor a fourni ; state (protocol.LHM…) explique l'absence
// de température du processeur.
type lhmReading struct {
	sensors []lhmSensor
	state   string
}

// readLHM lit les températures publiées par LibreHardwareMonitor : par WMI (espace de noms
// root\LibreHardwareMonitor, versions 0.9.4 et plus anciennes), sinon par son serveur web
// (Options → Remote Web Server → Run), seul moyen depuis la version 0.9.5. note (facultatif)
// reçoit le détail de chaque étape.
func readLHM(note func(format string, args ...any)) lhmReading {
	if note == nil {
		note = func(string, ...any) {}
	}
	if sensors, err := lhmWMI(); err == nil && len(sensors) > 0 {
		note("LibreHardwareMonitor : lu par WMI (%d capteurs de température)", len(sensors))
		return lhmReading{sensors: sensors, state: protocol.LHMNoSensor}
	}
	exe, running := lhmProcess()
	cfg := lhmConfig{Port: lhmDefaultPort}
	switch {
	case !running:
		note("LibreHardwareMonitor : ne tourne pas")
	case exe == "":
		note("LibreHardwareMonitor : en cours d'exécution (emplacement inconnu)")
	default:
		note("LibreHardwareMonitor : en cours d'exécution (%s)", exe)
		if c, err := readLHMConfig(exe); err != nil {
			note("Réglages de LibreHardwareMonitor illisibles : %v", err)
		} else {
			cfg = c
			ip := cfg.IP
			if ip == "" || ip == "?" {
				ip = "toutes les adresses"
			}
			note("Réglages enregistrés : serveur web %s, port %d, adresse %s, mot de passe %s",
				onOff(cfg.WebServer), cfg.Port, ip, onOff(cfg.Auth))
		}
	}
	auth := false
	for _, addr := range lhmAddresses(cfg, localAddrs()) {
		sensors, status, err := lhmWeb(addr)
		switch {
		case err == nil:
			note("Serveur web de LibreHardwareMonitor (%s) : %d capteurs de température", addr, len(sensors))
			return lhmReading{sensors: sensors, state: lhmState(running, auth, true)}
		case status == http.StatusUnauthorized:
			auth = true
			note("Serveur web de LibreHardwareMonitor (%s) : mot de passe demandé", addr)
		default:
			note("Serveur web de LibreHardwareMonitor (%s) : %v", addr, err)
		}
	}
	return lhmReading{state: lhmState(running, auth, false)}
}

func onOff(b bool) string {
	if b {
		return "activé"
	}
	return "désactivé"
}

// lhmProcess cherche LibreHardwareMonitor parmi les programmes en cours (toutes sessions) et
// renvoie l'emplacement de son exécutable s'il est lisible.
func lhmProcess() (exe string, running bool) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return "", false
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if !strings.EqualFold(windows.UTF16ToString(e.ExeFile[:]), "LibreHardwareMonitor.exe") {
			continue
		}
		running = true
		if path := processPath(e.ProcessID); path != "" {
			return path, true
		}
	}
	return "", running
}

func processPath(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	size := uint32(len(buf))
	if windows.QueryFullProcessImageName(h, 0, &buf[0], &size) != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:size])
}

// readLHMConfig lit LibreHardwareMonitor.config, à côté de l'exécutable.
func readLHMConfig(exe string) (lhmConfig, error) {
	path := strings.TrimSuffix(exe, filepath.Ext(exe)) + ".config"
	f, err := os.Open(path)
	if err != nil {
		return lhmConfig{Port: lhmDefaultPort}, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 1<<20))
	if err != nil {
		return lhmConfig{Port: lhmDefaultPort}, err
	}
	return parseLHMConfig(data)
}

// localAddrs : adresses IP de ce PC.
func localAddrs() []netip.Addr {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	var out []netip.Addr
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok {
			if ip, ok := netip.AddrFromSlice(n.IP); ok {
				out = append(out, ip.Unmap())
			}
		}
	}
	return out
}

// pawnIOVersion renvoie la version du pilote PawnIO installé (LibreHardwareMonitor 0.9.5 ou plus
// en a besoin pour lire le processeur).
func pawnIOVersion() (string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\PawnIO`, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return "", err
	}
	defer k.Close()
	v, _, err := k.GetStringValue("DisplayVersion")
	return v, err
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

// lhmClient interroge le serveur web de LibreHardwareMonitor : jamais de proxy ni de redirection.
var lhmClient = &http.Client{
	Timeout:       time.Second,
	Transport:     &http.Transport{Proxy: nil, DisableKeepAlives: true},
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// lhmWeb lit data.json sur le serveur web de LibreHardwareMonitor (addr : « 127.0.0.1:8085 »).
func lhmWeb(addr string) ([]lhmSensor, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/data.json", nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := lhmClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, errors.New(resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	sensors, err := parseLHMWeb(data)
	return sensors, resp.StatusCode, err
}

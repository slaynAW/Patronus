package specs

import (
	"fmt"
	"runtime"
	"strings"
	"unsafe"

	"github.com/go-ole/go-ole"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/slaynaw/wakeonlan/agent/internal/sensors"
	"github.com/slaynaw/wakeonlan/agent/internal/wmi"
	"github.com/slaynaw/wakeonlan/agent/protocol"
)

var (
	kernel32                      = windows.NewLazySystemDLL("kernel32.dll")
	procGetSystemFirmwareTable    = kernel32.NewProc("GetSystemFirmwareTable")
	procPhysicallyInstalledMemory = kernel32.NewProc("GetPhysicallyInstalledSystemMemory")
	rsmb                          = uintptr('R')<<24 | uintptr('S')<<16 | uintptr('M')<<8 | uintptr('B')
)

func read() protocol.Specs {
	var s protocol.Specs
	s.CPU = registryCPU()
	s.OS = registryOS()
	if table, err := firmwareTable(); err == nil {
		if structures, _ := parseSMBIOS(table); len(structures) > 0 {
			fromFirmware(&s, readFirmware(structures))
		}
	}
	if s.CPU != nil && s.CPU.Threads == 0 {
		s.CPU.Threads = runtime.NumCPU()
	}
	if s.Memory == nil {
		var kb uint64
		if r, _, _ := procPhysicallyInstalledMemory.Call(uintptr(unsafe.Pointer(&kb))); r != 0 && kb > 0 {
			s.Memory = &protocol.SpecsMemory{Total: kb << 10}
		}
	}
	s.GPUs = gpus()
	return s
}

// firmwareTable lit la table SMBIOS brute (en-tête RawSMBIOSData de 8 octets retiré).
func firmwareTable() ([]byte, error) {
	if err := procGetSystemFirmwareTable.Find(); err != nil {
		return nil, err
	}
	size, _, err := procGetSystemFirmwareTable.Call(rsmb, 0, 0, 0)
	if size == 0 || size > 1<<20 {
		return nil, fmt.Errorf("table SMBIOS : %v", err)
	}
	buf := make([]byte, size)
	n, _, err := procGetSystemFirmwareTable.Call(rsmb, 0, uintptr(unsafe.Pointer(&buf[0])), size)
	if n == 0 || n > size || n < 8 {
		return nil, fmt.Errorf("table SMBIOS : %v", err)
	}
	length := int(uint32(buf[4]) | uint32(buf[5])<<8 | uint32(buf[6])<<16 | uint32(buf[7])<<24)
	if length > int(n)-8 {
		length = int(n) - 8
	}
	return buf[8 : 8+length], nil
}

// registryCPU lit le nom et la fréquence du processeur.
func registryCPU() *protocol.SpecsCPU {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\CentralProcessor\0`, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer k.Close()
	name, _, _ := k.GetStringValue("ProcessorNameString")
	c := &protocol.SpecsCPU{Name: cpuName(name)}
	if mhz, _, err := k.GetIntegerValue("~MHz"); err == nil && mhz > 0 && mhz < 20000 {
		c.MHz = int(mhz)
	}
	if c.Name == "" {
		return nil
	}
	return c
}

// registryOS lit la version de Windows (« Windows 11 Pro », « 24H2, build 26100.2314 »).
func registryOS() *protocol.SpecsOS {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer k.Close()
	name, _, _ := k.GetStringValue("ProductName")
	build, _, _ := k.GetStringValue("CurrentBuild")
	if build == "" {
		build, _, _ = k.GetStringValue("CurrentBuildNumber")
	}
	return windowsVersion(name, build, stringValue(k, "DisplayVersion", "ReleaseId"), integerValue(k, "UBR"))
}

func stringValue(k registry.Key, names ...string) string {
	for _, n := range names {
		if v, _, err := k.GetStringValue(n); err == nil && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func integerValue(k registry.Key, name string) uint64 {
	v, _, err := k.GetIntegerValue(name)
	if err != nil {
		return 0
	}
	return v
}

// gpus liste les cartes graphiques présentes (WMI), avec leur mémoire dédiée (registre du pilote).
func gpus() []protocol.SpecsGPU {
	var out []protocol.SpecsGPU
	_ = wmi.WithCOM(func() error {
		service, release, err := wmi.Connect(`root\cimv2`)
		if err != nil {
			return err
		}
		defer release()
		return wmi.Each(service, "SELECT Name, PNPDeviceID, DriverVersion, AdapterRAM FROM Win32_VideoController", func(item *ole.IDispatch) error {
			name, pnp := clean(wmi.String(item, "Name")), wmi.String(item, "PNPDeviceID")
			if name == "" || virtualAdapter(name, pnp) {
				return nil
			}
			g := protocol.SpecsGPU{Name: name, Driver: clean(wmi.String(item, "DriverVersion"))}
			vram, bus, device, function := adapterRegistry(pnp)
			// AdapterRAM tient sur 32 bits : plafonnée à 4 Go, elle n'est qu'un repli.
			if ram := wmi.Int(item, "AdapterRAM"); vram == 0 && ram > 0 && ram < 0xFFF00000 {
				vram = uint64(ram)
			}
			g.Integrated = sensors.IntegratedGPU(name, bus, device, function)
			if !g.Integrated {
				g.VRAM = vram
			}
			out = append(out, g)
			return nil
		})
	})
	return out
}

// adapterRegistry lit la mémoire dédiée et l'emplacement PCI d'une carte graphique (bus négatif :
// inconnu).
func adapterRegistry(pnp string) (vram uint64, bus, device, function int) {
	bus, device, function = -1, -1, -1
	if pnp == "" || strings.ContainsAny(pnp, "\x00") {
		return
	}
	enum, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Enum\`+pnp, registry.QUERY_VALUE)
	if err != nil {
		return
	}
	driver, _, _ := enum.GetStringValue("Driver")
	location, _, _ := enum.GetStringValue("LocationInformation")
	enum.Close()
	bus, device, function = pciLocation(location)
	if driver == "" || strings.Contains(driver, "..") {
		return
	}
	class, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Class\`+driver, registry.QUERY_VALUE)
	if err != nil {
		return
	}
	defer class.Close()
	if v, _, err := class.GetIntegerValue("HardwareInformation.qwMemorySize"); err == nil && v > 0 {
		return v, bus, device, function
	}
	if v, kind, err := class.GetIntegerValue("HardwareInformation.MemorySize"); err == nil && v > 0 && kind == registry.DWORD {
		return v, bus, device, function
	}
	if raw, _, err := class.GetBinaryValue("HardwareInformation.MemorySize"); err == nil && len(raw) >= 4 {
		v := uint64(raw[0]) | uint64(raw[1])<<8 | uint64(raw[2])<<16 | uint64(raw[3])<<24
		return v, bus, device, function
	}
	return 0, bus, device, function
}

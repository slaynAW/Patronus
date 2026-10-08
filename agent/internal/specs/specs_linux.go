package specs

import (
	"os"
	"path/filepath"
	"runtime"

	"golang.org/x/sys/unix"

	"github.com/slaynaw/wakeonlan/agent/protocol"
)

// Emplacements lus (remplaçables pour les tests).
var (
	procRoot = "/proc"
	sysRoot  = "/sys"
	etcRoot  = "/etc"
	pciIDs   = []string{"/usr/share/hwdata/pci.ids", "/usr/share/misc/pci.ids", "/usr/share/pci.ids"}
)

func read() protocol.Specs {
	var s protocol.Specs
	cpuinfo, _ := os.ReadFile(filepath.Join(procRoot, "cpuinfo"))
	s.CPU, s.Model = linuxCPU(string(cpuinfo))
	if s.CPU != nil && s.CPU.MHz == 0 {
		s.CPU.MHz = linuxBaseMHz(sysRoot)
	}
	if meminfo, err := os.ReadFile(filepath.Join(procRoot, "meminfo")); err == nil {
		if total := linuxMemTotal(string(meminfo)); total > 0 {
			s.Memory = &protocol.SpecsMemory{Total: total}
		}
	}
	// Table SMBIOS complète (lisible par root, comme l'agent) ; sinon les champs publics de DMI.
	if table, err := os.ReadFile(filepath.Join(sysRoot, "firmware/dmi/tables/DMI")); err == nil {
		if structures, _ := parseSMBIOS(table); len(structures) > 0 {
			fromFirmware(&s, readFirmware(structures))
		}
	}
	if s.Board == nil {
		s.Board = linuxDMI(sysRoot)
		if s.Model == "" {
			s.Model = linuxDMIModel(sysRoot)
		}
	}
	if s.Model == "" {
		s.Model = deviceTreeModel(sysRoot)
	}
	if s.CPU != nil && s.CPU.Threads == 0 {
		s.CPU.Threads = runtime.NumCPU()
	}
	s.GPUs = linuxGPUs(sysRoot, pciIDs)
	release, _ := os.ReadFile(filepath.Join(etcRoot, "os-release"))
	var uts unix.Utsname
	kernel := ""
	if unix.Uname(&uts) == nil {
		kernel = unix.ByteSliceToString(uts.Release[:])
	}
	s.OS = linuxOS(string(release), kernel)
	return s
}

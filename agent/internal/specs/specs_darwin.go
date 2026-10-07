package specs

import (
	"runtime"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/slaynaw/wakeonlan/agent/protocol"
)

func read() protocol.Specs {
	var s protocol.Specs
	if name, err := unix.Sysctl("machdep.cpu.brand_string"); err == nil && clean(name) != "" {
		c := &protocol.SpecsCPU{Name: cpuName(name), Threads: runtime.NumCPU()}
		if n, err := unix.SysctlUint32("hw.physicalcpu"); err == nil {
			c.Cores = int(n)
		}
		if n, err := unix.SysctlUint32("hw.logicalcpu"); err == nil {
			c.Threads = int(n)
		}
		if hz, err := unix.SysctlUint64("hw.cpufrequency"); err == nil && hz > 0 {
			c.MHz = int(hz / 1_000_000)
		}
		s.CPU = c
	}
	if total, err := unix.SysctlUint64("hw.memsize"); err == nil && total > 0 {
		s.Memory = &protocol.SpecsMemory{Total: total}
	}
	if model, err := unix.Sysctl("hw.model"); err == nil {
		s.Model = clean(model)
	}
	if version, err := unix.Sysctl("kern.osproductversion"); err == nil && strings.TrimSpace(version) != "" {
		s.OS = &protocol.SpecsOS{Name: "macOS " + strings.TrimSpace(version)}
		if build, err := unix.Sysctl("kern.osversion"); err == nil && strings.TrimSpace(build) != "" {
			s.OS.Version = "build " + strings.TrimSpace(build)
		}
	}
	// Puces Apple : la carte graphique est intégrée à la puce, du même nom.
	if s.CPU != nil && strings.HasPrefix(s.CPU.Name, "Apple") {
		s.GPUs = []protocol.SpecsGPU{{Name: s.CPU.Name, Integrated: true}}
	}
	return s
}

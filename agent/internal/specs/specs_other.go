//go:build !windows && !linux && !darwin

package specs

import (
	"runtime"

	"github.com/slaynaw/wakeonlan/agent/protocol"
)

func read() protocol.Specs {
	return protocol.Specs{CPU: &protocol.SpecsCPU{Threads: runtime.NumCPU()}, OS: &protocol.SpecsOS{Name: runtime.GOOS}}
}

package measures

import (
	"os"

	"github.com/slaynaw/wakeonlan/agent/protocol"
)

// memory lit /proc/meminfo : MemTotal et MemAvailable (le cache des fichiers est disponible).
func memory() *protocol.Measure {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return nil
	}
	total, available, ok := parseMeminfo(string(data))
	if !ok {
		return nil
	}
	return memoryMeasure(total, available)
}

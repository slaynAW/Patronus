package measures

import (
	"strconv"
	"strings"
)

// parseMeminfo lit MemTotal et MemAvailable de /proc/meminfo (en kio), en octets.
func parseMeminfo(text string) (total, available uint64, ok bool) {
	var haveTotal, haveAvailable bool
	for _, line := range strings.Split(text, "\n") {
		key, rest, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		kib, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			continue
		}
		switch key {
		case "MemTotal":
			total, haveTotal = kib*1024, true
		case "MemAvailable":
			available, haveAvailable = kib*1024, true
		}
	}
	return total, available, haveTotal && haveAvailable
}

package specs

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/slaynaw/wakeonlan/agent/protocol"
)

// windowsVersion compose la version de Windows à partir du registre. ProductName dit encore
// « Windows 10 » sous Windows 11 : le numéro de build tranche (22000 et plus : Windows 11).
func windowsVersion(name, build, display string, ubr uint64) *protocol.SpecsOS {
	name = clean(name)
	if name == "" {
		return nil
	}
	if n, err := strconv.Atoi(strings.TrimSpace(build)); err == nil && n >= 22000 {
		name = strings.Replace(name, "Windows 10", "Windows 11", 1)
	}
	var parts []string
	if display != "" {
		parts = append(parts, display)
	}
	if build = strings.TrimSpace(build); build != "" {
		if ubr > 0 {
			build = fmt.Sprintf("%s.%d", build, ubr)
		}
		parts = append(parts, "build "+build)
	}
	return &protocol.SpecsOS{Name: name, Version: strings.Join(parts, ", ")}
}

// virtualAdapter : adaptateur logiciel (affichage de base, bureau à distance, écran virtuel).
func virtualAdapter(name, pnp string) bool {
	lower, id := strings.ToLower(name), strings.ToUpper(pnp)
	return strings.Contains(lower, "basic display") || strings.Contains(lower, "basic render") ||
		strings.Contains(lower, "remote display") || strings.HasPrefix(id, `ROOT\`) || strings.HasPrefix(id, `SWD\`)
}

var (
	locationTuple  = regexp.MustCompile(`\((\d+),(\d+),(\d+)\)\s*$`)
	locationDigits = regexp.MustCompile(`\d+`)
)

// pciLocation lit l'emplacement PCI d'un périphérique : « @System32\drivers\pci.sys,#65536;PCI bus
// %1, device %2, function %3;(1,0,0) » ou « PCI bus 1, device 0, function 0 ».
func pciLocation(s string) (bus, device, function int) {
	parts := locationTuple.FindStringSubmatch(s)
	if parts == nil {
		if !strings.Contains(strings.ToLower(s), "pci") {
			return -1, -1, -1
		}
		digits := locationDigits.FindAllString(s, -1)
		if len(digits) != 3 {
			return -1, -1, -1
		}
		parts = append([]string{""}, digits...)
	}
	b, _ := strconv.Atoi(parts[1])
	d, _ := strconv.Atoi(parts[2])
	f, _ := strconv.Atoi(parts[3])
	return b, d, f
}

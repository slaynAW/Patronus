package specs

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/slaynaw/wakeonlan/agent/internal/sensors"
	"github.com/slaynaw/wakeonlan/agent/protocol"
)

// Lecture des fichiers de Linux (/proc, /sys, /etc), sans dépendance au système : testée partout.

// linuxCPU lit /proc/cpuinfo : nom, cœurs, processeurs logiques et nombre de processeurs ; model
// est le modèle de la carte (Raspberry Pi…) quand cpuinfo le donne.
func linuxCPU(cpuinfo string) (cpu *protocol.SpecsCPU, model string) {
	name := ""
	threads := 0
	cores := map[string]bool{}
	sockets := map[string]bool{}
	physical, core := "", ""
	flush := func() {
		if core != "" {
			cores[physical+"/"+core] = true
		}
		if physical != "" {
			sockets[physical] = true
		}
		physical, core = "", ""
	}
	for _, line := range strings.Split(cpuinfo, "\n") {
		key, value, ok := strings.Cut(line, ":")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if !ok {
			if strings.TrimSpace(line) == "" {
				flush()
			}
			continue
		}
		switch key {
		case "processor":
			threads++
		case "model name", "cpu model", "Processor":
			if name == "" && clean(value) != "" {
				name = value
			}
		case "physical id":
			physical = value
		case "core id":
			core = value
		case "Model": // Raspberry Pi : « Raspberry Pi 4 Model B Rev 1.4 »
			model = clean(value)
		}
	}
	flush()
	if name == "" && threads == 0 {
		return nil, model
	}
	c := &protocol.SpecsCPU{Name: cpuName(name), Threads: threads, Cores: len(cores)}
	if len(sockets) > 1 {
		c.Count = len(sockets)
	}
	return c, model
}

// linuxBaseMHz : fréquence de base du processeur (pilote intel_pstate ou amd-pstate), 0 si inconnue.
func linuxBaseMHz(sys string) int {
	for _, f := range []string{"base_frequency", "amd_pstate_nominal_freq"} {
		raw, err := os.ReadFile(filepath.Join(sys, "devices/system/cpu/cpu0/cpufreq", f))
		if err != nil {
			continue
		}
		if khz, err := strconv.Atoi(strings.TrimSpace(string(raw))); err == nil && khz > 0 {
			return khz / 1000
		}
	}
	return 0
}

// linuxMemTotal lit MemTotal dans /proc/meminfo (octets).
func linuxMemTotal(meminfo string) uint64 {
	for _, line := range strings.Split(meminfo, "\n") {
		if !strings.HasPrefix(line, "MemTotal:") {
			continue
		}
		fields := strings.Fields(strings.TrimPrefix(line, "MemTotal:"))
		if len(fields) == 0 {
			return 0
		}
		kb, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			return 0
		}
		return kb << 10
	}
	return 0
}

func readText(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.TrimRight(string(raw), "\x00\n"))
}

// linuxDMI lit la carte mère et le BIOS dans /sys/class/dmi/id (lisibles sans droits).
func linuxDMI(sys string) *protocol.SpecsBoard {
	dir := filepath.Join(sys, "class/dmi/id")
	b := protocol.SpecsBoard{
		Maker: shortMaker(readText(filepath.Join(dir, "board_vendor"))), Model: clean(readText(filepath.Join(dir, "board_name"))),
		BIOS: clean(readText(filepath.Join(dir, "bios_version"))), BIOSDate: biosDate(readText(filepath.Join(dir, "bios_date"))),
	}
	if b == (protocol.SpecsBoard{}) {
		return nil
	}
	return &b
}

// linuxDMIModel : modèle du PC (PC de marque, portable) dans /sys/class/dmi/id.
func linuxDMIModel(sys string) string {
	dir := filepath.Join(sys, "class/dmi/id")
	model := clean(readText(filepath.Join(dir, "product_name")))
	if model == "" || strings.EqualFold(model, clean(readText(filepath.Join(dir, "board_name")))) {
		return ""
	}
	return joinMaker(readText(filepath.Join(dir, "sys_vendor")), model)
}

// deviceTreeModel : modèle d'une carte ARM (Raspberry Pi…) décrit par l'arbre des périphériques.
func deviceTreeModel(sys string) string {
	return clean(readText(filepath.Join(sys, "firmware/devicetree/base/model")))
}

var (
	cardName = regexp.MustCompile(`^card\d+$`)
	pciSlot  = regexp.MustCompile(`^[0-9a-fA-F]{4}:([0-9a-fA-F]{2}):([0-9a-fA-F]{2})\.([0-7])$`)
)

// gpuVendors : fabricants de cartes graphiques (identifiant PCI), pour un nom absent de pci.ids.
var gpuVendors = map[string]string{"10de": "NVIDIA", "1002": "AMD", "8086": "Intel", "1af4": "Virtio", "15ad": "VMware", "1234": "QEMU"}

// linuxGPUs liste les cartes graphiques (/sys/class/drm) ; leur nom vient de pci.ids s'il existe.
func linuxGPUs(sys string, idFiles []string) []protocol.SpecsGPU {
	entries, err := os.ReadDir(filepath.Join(sys, "class/drm"))
	if err != nil {
		return nil
	}
	type card struct {
		vendor, device, subVendor, subDevice, slot, driver string
		vram                                               uint64
	}
	var cards []card
	seen := map[string]bool{}
	for _, e := range entries {
		if !cardName.MatchString(e.Name()) {
			continue
		}
		dev := filepath.Join(sys, "class/drm", e.Name(), "device")
		c := card{
			vendor: strings.TrimPrefix(readText(filepath.Join(dev, "vendor")), "0x"), device: strings.TrimPrefix(readText(filepath.Join(dev, "device")), "0x"),
			subVendor: strings.TrimPrefix(readText(filepath.Join(dev, "subsystem_vendor")), "0x"), subDevice: strings.TrimPrefix(readText(filepath.Join(dev, "subsystem_device")), "0x"),
		}
		if c.vendor == "" || c.device == "" {
			continue // affichage simple sans carte PCI (simpledrm…)
		}
		for _, line := range strings.Split(readText(filepath.Join(dev, "uevent")), "\n") {
			if v, ok := strings.CutPrefix(line, "PCI_SLOT_NAME="); ok {
				c.slot = v
			}
			if v, ok := strings.CutPrefix(line, "DRIVER="); ok {
				c.driver = v
			}
		}
		key := c.slot
		if key == "" {
			key = e.Name()
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		if v, err := strconv.ParseUint(readText(filepath.Join(dev, "mem_info_vram_total")), 10, 64); err == nil {
			c.vram = v
		}
		cards = append(cards, c)
	}
	if len(cards) == 0 {
		return nil
	}
	wanted := map[string]bool{}
	for _, c := range cards {
		wanted[strings.ToLower(c.vendor+":"+c.device)] = true
	}
	names := pciNames(idFiles, wanted)
	var out []protocol.SpecsGPU
	for _, c := range cards {
		name := names[strings.ToLower(c.vendor+":"+c.device)]
		if name == "" {
			vendor := gpuVendors[strings.ToLower(c.vendor)]
			if vendor == "" {
				vendor = "PCI " + c.vendor
			}
			name = fmt.Sprintf("%s (%s:%s)", vendor, c.vendor, c.device)
		}
		g := protocol.SpecsGPU{Name: name, VRAM: c.vram}
		if c.driver != "" {
			g.Driver = c.driver
			if v := readText(filepath.Join(sys, "module", c.driver, "version")); v != "" {
				g.Driver += " " + v
			}
		}
		bus, device, function := -1, -1, -1
		if m := pciSlot.FindStringSubmatch(c.slot); m != nil {
			b, _ := strconv.ParseInt(m[1], 16, 32)
			d, _ := strconv.ParseInt(m[2], 16, 32)
			f, _ := strconv.ParseInt(m[3], 16, 32)
			bus, device, function = int(b), int(d), int(f)
		}
		g.Integrated = sensors.IntegratedGPU(name, bus, device, function)
		if g.Integrated {
			g.VRAM = 0
		}
		out = append(out, g)
	}
	return out
}

// pciNames cherche dans pci.ids le nom des périphériques « vendor:device » (hexadécimal, en
// minuscules) : « NVIDIA Corporation » + « AD104 [GeForce RTX 4070] » donnent « NVIDIA GeForce RTX
// 4070 ».
func pciNames(files []string, wanted map[string]bool) map[string]string {
	out := map[string]string{}
	for _, path := range files {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		vendor, vendorName := "", ""
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" || line[0] == '#' {
				continue
			}
			if line[0] != '\t' {
				if strings.HasPrefix(line, "C ") {
					break // classes de périphériques : fin des fabricants
				}
				vendor, vendorName = "", ""
				if id, name, ok := strings.Cut(line, "  "); ok && len(id) == 4 {
					vendor, vendorName = strings.ToLower(id), strings.TrimSpace(name)
				}
				continue
			}
			if vendor == "" || strings.HasPrefix(line, "\t\t") {
				continue
			}
			id, name, ok := strings.Cut(strings.TrimPrefix(line, "\t"), "  ")
			key := vendor + ":" + strings.ToLower(id)
			if ok && wanted[key] {
				out[key] = gpuName(vendorName, strings.TrimSpace(name))
			}
		}
		return out
	}
	return out
}

var bracket = regexp.MustCompile(`\[([^\]]+)\]`)

// gpuName compose un nom lisible : le nom commercial entre crochets s'il existe.
func gpuName(vendor, device string) string {
	short := vendor
	switch {
	case strings.HasPrefix(vendor, "NVIDIA"):
		short = "NVIDIA"
	case strings.Contains(vendor, "AMD") || strings.Contains(vendor, "ATI"):
		short = "AMD"
	case strings.HasPrefix(vendor, "Intel"):
		short = "Intel"
	}
	if m := bracket.FindAllStringSubmatch(device, -1); len(m) > 0 {
		device = m[len(m)-1][1]
	}
	if strings.HasPrefix(strings.ToLower(device), strings.ToLower(short)) {
		return device
	}
	return short + " " + device
}

// linuxOS lit /etc/os-release (« Ubuntu 24.04.1 LTS ») et la version du noyau.
func linuxOS(release, kernel string) *protocol.SpecsOS {
	values := map[string]string{}
	for _, line := range strings.Split(release, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		if unq, err := strconv.Unquote(v); err == nil {
			v = unq
		} else {
			v = strings.Trim(v, `"'`)
		}
		values[k] = v
	}
	name := clean(values["PRETTY_NAME"])
	if name == "" {
		name = strings.TrimSpace(clean(values["NAME"]) + " " + clean(values["VERSION"]))
	}
	if name == "" {
		name = "Linux"
	}
	o := &protocol.SpecsOS{Name: name}
	if kernel != "" {
		o.Version = "noyau " + kernel
	}
	return o
}

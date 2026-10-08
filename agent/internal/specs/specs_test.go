package specs

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol"
)

// smbiosStruct fabrique une structure SMBIOS : zone formatée (après l'en-tête) et chaînes.
func smbiosStruct(kind byte, handle uint16, formatted []byte, strs ...string) []byte {
	out := []byte{kind, byte(4 + len(formatted)), byte(handle), byte(handle >> 8)}
	out = append(out, formatted...)
	if len(strs) == 0 {
		return append(out, 0, 0)
	}
	for _, s := range strs {
		out = append(append(out, s...), 0)
	}
	return append(out, 0)
}

// area prépare une zone formatée de n octets (décalages SMBIOS, en-tête de 4 octets compris).
type area []byte

func newArea(length int) area { return make(area, length-4) }

func (a area) b(off int, v byte) area { a[off-4] = v; return a }

func (a area) w(off int, v uint16) area { binary.LittleEndian.PutUint16(a[off-4:], v); return a }

func (a area) d(off int, v uint32) area { binary.LittleEndian.PutUint32(a[off-4:], v); return a }

func sampleTable() []byte {
	var t []byte
	// BIOS : fabricant (1), version (2), date (3).
	t = append(t, smbiosStruct(0, 0, newArea(0x18).b(4, 1).b(5, 2).b(8, 3), "American Megatrends Inc.", "1.A0", "03/12/2024")...)
	// Système : textes par défaut d'un PC monté soi-même.
	t = append(t, smbiosStruct(1, 1, newArea(0x1B).b(4, 1).b(5, 2).b(7, 3), "System manufacturer", "System Product Name", "SERIAL-SECRET")...)
	// Carte mère.
	t = append(t, smbiosStruct(2, 2, newArea(0x0F).b(4, 1).b(5, 2).b(7, 3), "Micro-Star International Co., Ltd.", "MAG B550 TOMAHAWK (MS-7C91)", "BOARD-SERIAL")...)
	// Processeur installé (8 cœurs, 16 fils, 3800 MHz) puis un emplacement vide.
	cpu := newArea(0x30).b(0x10, 1).w(0x16, 3800).b(0x18, 0x41).b(0x23, 8).b(0x25, 16)
	t = append(t, smbiosStruct(4, 3, cpu, "AMD Ryzen 7 5800X 8-Core Processor            ")...)
	t = append(t, smbiosStruct(4, 4, newArea(0x30).b(0x18, 0x00))...)
	// Tableau de mémoire système : 4 emplacements ; un tableau de cache est ignoré.
	t = append(t, smbiosStruct(16, 0x10, newArea(0x17).b(5, 3).w(0x0D, 4))...)
	t = append(t, smbiosStruct(16, 0x11, newArea(0x17).b(5, 7).w(0x0D, 2))...)
	// Barrettes : 16 Go DDR4-3200 Samsung (code JEDEC), emplacement vide, 32 Go (taille étendue) DDR5.
	m1 := newArea(0x28).w(0x0C, 16384).b(0x10, 1).b(0x11, 2).b(0x12, 0x1A).w(0x15, 3600).b(0x17, 3).b(0x18, 4).b(0x1A, 5).w(0x20, 3200)
	t = append(t, smbiosStruct(17, 0x20, m1, "DIMM_A2", "BANK 1", "80CE000080CE", "MODULE-SERIAL", "M378A2G43AB3-CWE  ")...)
	t = append(t, smbiosStruct(17, 0x21, newArea(0x28).w(0x0C, 0).b(0x10, 1), "DIMM_A1")...)
	m3 := newArea(0x5C).w(0x0C, 0x7FFF).d(0x1C, 32768).b(0x10, 1).b(0x12, 0x22).w(0x15, 0xFFFF).d(0x54, 8000).b(0x17, 2).b(0x1A, 3).b(0x28, 3)
	t = append(t, smbiosStruct(17, 0x22, m3, "DIMM_B2", "Kingston", "KF560C36-32")...)
	// Barrette de 512 Ko (granularité en Ko) et mémoire non volatile, ignorée.
	t = append(t, smbiosStruct(17, 0x23, newArea(0x28).w(0x0C, 0x8000|512).b(0x10, 1), "ONBOARD")...)
	t = append(t, smbiosStruct(17, 0x24, newArea(0x2A).w(0x0C, 8192).b(0x28, 7).b(0x10, 1), "NVDIMM")...)
	return append(t, smbiosStruct(127, 0xFFFF, nil)...)
}

func TestSMBIOS(t *testing.T) {
	structures, err := parseSMBIOS(sampleTable())
	if err != nil {
		t.Fatal(err)
	}
	if len(structures) != 13 || structures[len(structures)-1].kind != 127 {
		t.Fatalf("%d structures", len(structures))
	}
	var s protocol.Specs
	fromFirmware(&s, readFirmware(structures))
	if s.Model != "" {
		t.Errorf("modèle « %s » : textes par défaut à ignorer", s.Model)
	}
	board := protocol.SpecsBoard{Maker: "MSI", Model: "MAG B550 TOMAHAWK (MS-7C91)", BIOS: "1.A0", BIOSDate: "2024-03-12"}
	if s.Board == nil || *s.Board != board {
		t.Errorf("carte mère %+v", s.Board)
	}
	cpu := protocol.SpecsCPU{Name: "AMD Ryzen 7 5800X", Cores: 8, Threads: 16, MHz: 3800}
	if s.CPU == nil || *s.CPU != cpu {
		t.Errorf("processeur %+v", s.CPU)
	}
	if s.Memory == nil || s.Memory.Slots != 4 || len(s.Memory.Modules) != 3 {
		t.Fatalf("mémoire %+v", s.Memory)
	}
	want := []protocol.SpecsModule{
		{Slot: "DIMM_A2", Size: 16 << 30, Type: "DDR4", MTs: 3200, Maker: "Samsung", Part: "M378A2G43AB3-CWE"},
		{Slot: "DIMM_B2", Size: 32 << 30, Type: "DDR5", MTs: 8000, Maker: "Kingston", Part: "KF560C36-32"},
		{Slot: "ONBOARD", Size: 512 << 10},
	}
	for i, m := range want {
		if s.Memory.Modules[i] != m {
			t.Errorf("barrette %d : %+v, attendu %+v", i, s.Memory.Modules[i], m)
		}
	}
	if s.Memory.Total != 48<<30+512<<10 {
		t.Errorf("total %d", s.Memory.Total)
	}
	raw, _ := json.Marshal(s)
	if strings.Contains(string(raw), "SERIAL") {
		t.Errorf("numéro de série dans la fiche : %s", raw)
	}
}

func TestSMBIOSBrandedPC(t *testing.T) {
	var table []byte
	table = append(table, smbiosStruct(1, 1, newArea(0x1B).b(4, 1).b(5, 2), "Dell Inc.", "XPS 15 9520")...)
	table = append(table, smbiosStruct(2, 2, newArea(0x0F).b(4, 1).b(5, 2), "Dell Inc.", "0RH1JY")...)
	table = append(table, smbiosStruct(127, 3, nil)...)
	structures, err := parseSMBIOS(table)
	if err != nil {
		t.Fatal(err)
	}
	var s protocol.Specs
	fromFirmware(&s, readFirmware(structures))
	if s.Model != "Dell XPS 15 9520" || s.Board == nil || s.Board.Maker != "Dell" || s.Board.Model != "0RH1JY" {
		t.Errorf("%+v %+v", s.Model, s.Board)
	}
}

func TestSMBIOSTruncated(t *testing.T) {
	table := sampleTable()
	for _, n := range []int{5, 30, 60, len(table) - 8} {
		if _, err := parseSMBIOS(table[:n]); err == nil {
			t.Errorf("table tronquée à %d octets acceptée", n)
		}
	}
	// Moins d'un en-tête à la fin (structure de fin absente) : les structures entières sont gardées.
	if s, err := parseSMBIOS(table[:len(table)-3]); err != nil || len(s) != 12 {
		t.Errorf("%d structures, %v", len(s), err)
	}
	// Zone formatée trop courte : champs absents, sans panique.
	short := smbiosStruct(17, 1, newArea(0x0E).w(0x0C, 4096))
	structures, err := parseSMBIOS(append(short, smbiosStruct(127, 2, nil)...))
	if err != nil {
		t.Fatal(err)
	}
	fw := readFirmware(structures)
	if len(fw.modules) != 1 || fw.modules[0].size != 4<<30 || fw.modules[0].mts != 0 {
		t.Errorf("%+v", fw.modules)
	}
}

func TestClean(t *testing.T) {
	cases := map[string]string{
		"  To Be Filled By O.E.M.  ": "",
		"Default string":             "",
		"ASUS\x00 PRIME   Z790-P ":   "ASUS PRIME Z790-P",
		"N/A":                        "",
	}
	for in, want := range cases {
		if got := clean(in); got != want {
			t.Errorf("clean(%q) = %q, attendu %q", in, got, want)
		}
	}
	names := map[string]string{
		"Intel(R) Core(TM) i7-12700K":                       "Intel Core i7-12700K",
		"Intel(R) Core(TM) i5-8400 CPU @ 2.80GHz":           "Intel Core i5-8400 CPU @ 2.80GHz",
		"AMD Ryzen 9 7950X3D 16-Core Processor            ": "AMD Ryzen 9 7950X3D",
		"Intel(R) Xeon(R) CPU E5-2680 v4":                   "Intel Xeon CPU E5-2680 v4",
		"AMD Ryzen 5 PRO 4650G with Radeon Graphics":        "AMD Ryzen 5 PRO 4650G with Radeon Graphics",
	}
	for in, want := range names {
		if got := cpuName(in); got != want {
			t.Errorf("cpuName(%q) = %q, attendu %q", in, got, want)
		}
	}
	if got := joinMaker("LENOVO", "ThinkPad X1 Carbon"); got != "Lenovo ThinkPad X1 Carbon" {
		t.Error(got)
	}
	if got := joinMaker("HP", "HP EliteDesk 800 G6"); got != "HP EliteDesk 800 G6" {
		t.Error(got)
	}
	makers := map[string]string{"80CE000080CE": "Samsung", "0x029E": "Corsair", "Kingston": "Kingston", "ABCD1234": "", "Unknown": "", "G Skill Intl": "G Skill Intl"}
	for in, want := range makers {
		if got := memoryMaker(in); got != want {
			t.Errorf("memoryMaker(%q) = %q, attendu %q", in, got, want)
		}
	}
	dates := map[string]string{"03/12/2024": "2024-03-12", "12/31/99": "1999-12-31", "01/02/05": "2005-01-02", "2024": "", "13/01/2024": ""}
	for in, want := range dates {
		if got := biosDate(in); got != want {
			t.Errorf("biosDate(%q) = %q, attendu %q", in, got, want)
		}
	}
}

func TestWindowsParsing(t *testing.T) {
	os := windowsVersion("Windows 10 Pro", "26100", "24H2", 2314)
	if os == nil || os.Name != "Windows 11 Pro" || os.Version != "24H2, build 26100.2314" {
		t.Errorf("%+v", os)
	}
	os = windowsVersion("Windows 10 Home", "19045", "22H2", 0)
	if os == nil || os.Name != "Windows 10 Home" || os.Version != "22H2, build 19045" {
		t.Errorf("%+v", os)
	}
	locations := map[string][3]int{
		`@System32\drivers\pci.sys,#65536;PCI bus %1, device %2, function %3;(1,0,0)`: {1, 0, 0},
		"PCI bus 0, device 2, function 0":                                             {0, 2, 0},
		"Bus PCI 3, périphérique 0, fonction 0":                                       {3, 0, 0},
		"Port_#0001.Hub_#0001":                                                        {-1, -1, -1},
		"":                                                                            {-1, -1, -1},
	}
	for in, want := range locations {
		b, d, f := pciLocation(in)
		if [3]int{b, d, f} != want {
			t.Errorf("pciLocation(%q) = %d,%d,%d", in, b, d, f)
		}
	}
	if !virtualAdapter("Microsoft Basic Display Adapter", `PCI\VEN_1234`) || !virtualAdapter("Parsec Virtual Display Adapter", `ROOT\DISPLAY\0000`) ||
		virtualAdapter("NVIDIA GeForce RTX 4070", `PCI\VEN_10DE&DEV_2786`) {
		t.Error("adaptateurs virtuels mal reconnus")
	}
}

func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLinuxParsing(t *testing.T) {
	cpuinfo := ""
	for i := 0; i < 8; i++ {
		cpuinfo += "processor\t: " + string(rune('0'+i)) + "\nmodel name\t: Intel(R) Core(TM) i5-10400 CPU @ 2.90GHz\nphysical id\t: 0\ncore id\t\t: " + string(rune('0'+i%4)) + "\n\n"
	}
	cpu, model := linuxCPU(cpuinfo)
	if cpu == nil || cpu.Name != "Intel Core i5-10400 CPU @ 2.90GHz" || cpu.Cores != 4 || cpu.Threads != 8 || cpu.Count != 0 || model != "" {
		t.Errorf("%+v %q", cpu, model)
	}
	pi, model := linuxCPU("processor\t: 0\nBogoMIPS\t: 108.00\n\nprocessor\t: 1\n\nHardware\t: BCM2835\nModel\t\t: Raspberry Pi 4 Model B Rev 1.4\n")
	if pi == nil || pi.Threads != 2 || model != "Raspberry Pi 4 Model B Rev 1.4" {
		t.Errorf("%+v %q", pi, model)
	}
	if got := linuxMemTotal("MemTotal:       32768000 kB\nMemFree: 1 kB\n"); got != 32768000<<10 {
		t.Error(got)
	}

	sys := t.TempDir()
	ids := filepath.Join(t.TempDir(), "pci.ids")
	writeFiles(t, sys, map[string]string{
		"class/dmi/id/board_vendor":                      "ASUSTeK COMPUTER INC.\n",
		"class/dmi/id/board_name":                        "PRIME B460M-A\n",
		"class/dmi/id/bios_version":                      "1401\n",
		"class/dmi/id/bios_date":                         "06/04/2021\n",
		"class/dmi/id/sys_vendor":                        "System manufacturer\n",
		"class/dmi/id/product_name":                      "System Product Name\n",
		"class/drm/card0/device/vendor":                  "0x10de\n",
		"class/drm/card0/device/device":                  "0x2786\n",
		"class/drm/card0/device/uevent":                  "DRIVER=nvidia\nPCI_SLOT_NAME=0000:01:00.0\n",
		"module/nvidia/version":                          "560.35.03\n",
		"class/drm/card1/device/vendor":                  "0x8086\n",
		"class/drm/card1/device/device":                  "0x9bc8\n",
		"class/drm/card1/device/uevent":                  "DRIVER=i915\nPCI_SLOT_NAME=0000:00:02.0\n",
		"class/drm/card2/device/vendor":                  "0x1002\n",
		"class/drm/card2/device/device":                  "0x7480\n",
		"class/drm/card2/device/uevent":                  "DRIVER=amdgpu\nPCI_SLOT_NAME=0000:03:00.0\n",
		"class/drm/card2/device/mem_info_vram_total":     "8573157376\n",
		"class/drm/card1-HDMI-A-1/status":                "connected\n",
		"devices/system/cpu/cpu0/cpufreq/base_frequency": "2900000\n",
	})
	writeFiles(t, filepath.Dir(ids), map[string]string{"pci.ids": "# commentaire\n10de  NVIDIA Corporation\n\t2786  AD104 [GeForce RTX 4070]\n\t\t1043 8888  sous-système\n8086  Intel Corporation\n\t9bc8  CometLake-S GT2 [UHD Graphics 630]\nC 00  Unclassified device\n"})
	board := linuxDMI(sys)
	if board == nil || *board != (protocol.SpecsBoard{Maker: "ASUS", Model: "PRIME B460M-A", BIOS: "1401", BIOSDate: "2021-06-04"}) {
		t.Errorf("%+v", board)
	}
	if m := linuxDMIModel(sys); m != "" {
		t.Errorf("modèle %q", m)
	}
	if mhz := linuxBaseMHz(sys); mhz != 2900 {
		t.Error(mhz)
	}
	gpus := linuxGPUs(sys, []string{filepath.Join(sys, "absent"), ids})
	want := []protocol.SpecsGPU{
		{Name: "NVIDIA GeForce RTX 4070", Driver: "nvidia 560.35.03"},
		{Name: "Intel UHD Graphics 630", Driver: "i915", Integrated: true},
		{Name: "AMD (1002:7480)", Driver: "amdgpu", VRAM: 8573157376},
	}
	if len(gpus) != len(want) {
		t.Fatalf("%+v", gpus)
	}
	for i := range want {
		if gpus[i] != want[i] {
			t.Errorf("carte %d : %+v, attendu %+v", i, gpus[i], want[i])
		}
	}
	o := linuxOS("NAME=\"Ubuntu\"\nVERSION=\"24.04.1 LTS (Noble Numbat)\"\nPRETTY_NAME=\"Ubuntu 24.04.1 LTS\"\n", "6.8.0-45-generic")
	if o == nil || o.Name != "Ubuntu 24.04.1 LTS" || o.Version != "noyau 6.8.0-45-generic" {
		t.Errorf("%+v", o)
	}
	if o := linuxOS("", ""); o == nil || o.Name != "Linux" {
		t.Errorf("%+v", o)
	}
}

// maxSpecs : fiche la plus longue possible.
func maxSpecs() protocol.Specs {
	// « < » : le caractère le plus long une fois échappé deux fois (\\u003c).
	long := strings.Repeat("<", protocol.SpecsMaxText+10)
	s := protocol.Specs{
		Model:  long,
		CPU:    &protocol.SpecsCPU{Name: long, Cores: 256, Threads: 512, MHz: 5000, Count: 8},
		Memory: &protocol.SpecsMemory{Total: 1 << 42, Slots: 64},
		Board:  &protocol.SpecsBoard{Maker: long, Model: long, BIOS: long, BIOSDate: long},
		OS:     &protocol.SpecsOS{Name: long, Version: long},
	}
	for i := 0; i < 40; i++ {
		s.Memory.Modules = append(s.Memory.Modules, protocol.SpecsModule{Slot: long, Size: 1 << 38, Type: long, MTs: 8000, Maker: long, Part: long})
		s.GPUs = append(s.GPUs, protocol.SpecsGPU{Name: long, VRAM: 1 << 36, Driver: long, Integrated: i%2 == 0})
	}
	return s
}

// La plus longue fiche tient dans la réponse, enveloppe signée comprise.
func TestLimitFitsInResponse(t *testing.T) {
	s := limit(maxSpecs())
	if len(s.Memory.Modules) != protocol.SpecsMaxModules || len(s.GPUs) != protocol.SpecsMaxGPUs || s.GPUs[0].Integrated {
		t.Fatalf("%d barrettes, %d cartes", len(s.Memory.Modules), len(s.GPUs))
	}
	if n := len([]rune(s.CPU.Name)); n != protocol.SpecsMaxText {
		t.Errorf("nom de %d caractères", n)
	}
	body, _ := json.Marshal(protocol.ResponseBody{OK: true, Code: "ok", Hostname: strings.Repeat("h", 64), OS: "windows", Arch: "amd64", Version: "1.10.0", Specs: &s})
	line, _ := json.Marshal(protocol.Response{Body: string(body), Mac: strings.Repeat("m", 43)})
	if len(line)+1 > protocol.MaxSpecsBytes {
		t.Errorf("réponse de %d octets (limite %d)", len(line)+1, protocol.MaxSpecsBytes)
	}
}

func TestCache(t *testing.T) {
	var reads atomic.Int32
	release := make(chan struct{})
	c := NewCache(func() protocol.Specs {
		reads.Add(1)
		<-release
		return protocol.Specs{Model: "PC"}
	})
	now := time.Now()
	c.now = func() time.Time { return now }
	c.Warm()
	go func() { time.Sleep(50 * time.Millisecond); close(release) }()
	if s := c.Get(); s == nil || s.Model != "PC" {
		t.Fatalf("%+v", s)
	}
	if s := c.Get(); s == nil || reads.Load() != 1 {
		t.Fatalf("%d lectures", reads.Load())
	}
	// Fiche ancienne : servie tout de suite, relue en arrière-plan.
	now = now.Add(CacheAge)
	if s := c.Get(); s == nil {
		t.Fatal("fiche ancienne non servie")
	}
	deadline := time.Now().Add(2 * time.Second)
	for reads.Load() != 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if reads.Load() != 2 {
		t.Errorf("%d lectures", reads.Load())
	}
}

// Lecture réelle (Linux et Windows en CI) : processeur, mémoire et système au minimum.
func TestReadReal(t *testing.T) {
	s := Read()
	raw, _ := json.MarshalIndent(s, "", "  ")
	t.Logf("fiche :\n%s", raw)
	if s.CPU == nil || s.CPU.Threads == 0 {
		t.Error("processeur absent")
	}
	if s.Memory == nil || s.Memory.Total == 0 {
		t.Error("mémoire absente")
	}
	if s.OS == nil || s.OS.Name == "" {
		t.Error("système absent")
	}
}

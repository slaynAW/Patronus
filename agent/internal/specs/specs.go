// Package specs lit la fiche du PC (commande « specs ») : processeur, mémoire vive, cartes
// graphiques, carte mère et système. Elle ne change qu'avec le matériel : lue au démarrage de
// l'agent puis toutes les CacheAge, jamais à chaque demande.
package specs

import (
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/slaynaw/wakeonlan/agent/protocol"
)

// Read lit la fiche de ce PC (une implémentation par système ; peut prendre quelques secondes).
func Read() protocol.Specs { return limit(read()) }

const (
	// CacheAge : âge à partir duquel la fiche est relue (en arrière-plan).
	CacheAge = 6 * time.Hour
	// firstWait : attente maximale de la première lecture par une demande (le client attend 4 s).
	firstWait = 3 * time.Second
)

// Cache garde la fiche et la relit en arrière-plan quand elle a vieilli.
type Cache struct {
	read func() protocol.Specs
	now  func() time.Time

	mu      sync.Mutex
	specs   *protocol.Specs
	at      time.Time
	reading chan struct{} // fermé à la fin de la lecture en cours (nil : aucune)
}

// NewCache renvoie un cache autour de read (Read en service).
func NewCache(read func() protocol.Specs) *Cache {
	return &Cache{read: read, now: time.Now}
}

// Warm lance la première lecture en arrière-plan (au démarrage de l'agent).
func (c *Cache) Warm() {
	c.mu.Lock()
	c.startLocked()
	c.mu.Unlock()
}

// Get renvoie la fiche ; la première fois, attend la lecture (firstWait au plus, nil ensuite).
func (c *Cache) Get() *protocol.Specs {
	c.mu.Lock()
	if c.specs == nil || c.now().Sub(c.at) >= CacheAge {
		c.startLocked()
	}
	specs, reading := c.specs, c.reading
	c.mu.Unlock()
	if specs != nil || reading == nil {
		return specs
	}
	select {
	case <-reading:
	case <-time.After(firstWait):
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.specs
}

func (c *Cache) startLocked() {
	if c.reading != nil {
		return
	}
	done := make(chan struct{})
	c.reading = done
	go func() {
		s := c.read()
		c.mu.Lock()
		c.specs, c.at, c.reading = &s, c.now(), nil
		c.mu.Unlock()
		close(done)
	}()
}

// limit borne la fiche (taille de la réponse) et range les listes.
func limit(s protocol.Specs) protocol.Specs {
	s.Model = cut(s.Model)
	if s.CPU != nil {
		s.CPU.Name = cut(s.CPU.Name)
	}
	if s.Memory != nil {
		if len(s.Memory.Modules) > protocol.SpecsMaxModules {
			s.Memory.Modules = s.Memory.Modules[:protocol.SpecsMaxModules]
		}
		for i := range s.Memory.Modules {
			m := &s.Memory.Modules[i]
			m.Slot, m.Type, m.Maker, m.Part = cut(m.Slot), cut(m.Type), cut(m.Maker), cut(m.Part)
		}
	}
	// Carte dédiée d'abord, puis la plus grosse mémoire vidéo.
	slices.SortStableFunc(s.GPUs, func(a, b protocol.SpecsGPU) int {
		if a.Integrated != b.Integrated {
			if a.Integrated {
				return 1
			}
			return -1
		}
		switch {
		case a.VRAM > b.VRAM:
			return -1
		case a.VRAM < b.VRAM:
			return 1
		}
		return 0
	})
	if len(s.GPUs) > protocol.SpecsMaxGPUs {
		s.GPUs = s.GPUs[:protocol.SpecsMaxGPUs]
	}
	for i := range s.GPUs {
		s.GPUs[i].Name, s.GPUs[i].Driver = cut(s.GPUs[i].Name), cut(s.GPUs[i].Driver)
	}
	if s.Board != nil {
		s.Board.Maker, s.Board.Model, s.Board.BIOS, s.Board.BIOSDate = cut(s.Board.Maker), cut(s.Board.Model), cut(s.Board.BIOS), cut(s.Board.BIOSDate)
	}
	if s.OS != nil {
		s.OS.Name, s.OS.Version = cut(s.OS.Name), cut(s.OS.Version)
	}
	return s
}

func cut(s string) string {
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "")
	}
	if utf8.RuneCountInString(s) <= protocol.SpecsMaxText {
		return s
	}
	return strings.TrimSpace(string([]rune(s)[:protocol.SpecsMaxText-1])) + "…"
}

// placeholders : textes laissés par défaut par les fabricants, sans information.
var placeholders = []string{
	"to be filled by o.e.m.", "to be filled by oem", "default string", "system manufacturer", "system product name",
	"system version", "base board manufacturer", "base board product name", "not specified", "not applicable",
	"not available", "none", "n/a", "na", "unknown", "o.e.m.", "oem", "undefined", "empty", "123456789",
	"type1productconfigid", "invalid", "x.x", "0", "00000000",
}

var (
	spaces     = regexp.MustCompile(`\s+`)
	coreSuffix = regexp.MustCompile(`(?i)\s+\d+-core processor$`)
)

// clean retire espaces superflus, caractères de contrôle et textes par défaut ("" s'il ne reste rien).
func clean(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == utf8.RuneError || unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	s = strings.TrimSpace(spaces.ReplaceAllString(s, " "))
	if slices.Contains(placeholders, strings.ToLower(s)) {
		return ""
	}
	return s
}

// cpuName nettoie le nom commercial d'un processeur (« Intel(R) Core(TM) i7-12700K CPU @ 3.60GHz »).
func cpuName(s string) string {
	s = clean(s)
	for _, mark := range []string{"(R)", "(r)", "(TM)", "(tm)", "®", "™"} {
		s = strings.ReplaceAll(s, mark, "")
	}
	s = strings.TrimSpace(spaces.ReplaceAllString(s, " "))
	// « AMD Ryzen 7 5800X 8-Core Processor » : le nombre de cœurs est affiché à part.
	s = strings.TrimSpace(coreSuffix.ReplaceAllString(s, ""))
	s = strings.TrimSuffix(s, " CPU")
	return s
}

// makers : noms courts des fabricants (« Micro-Star International Co., Ltd. » → « MSI »).
var makers = []struct{ prefix, name string }{
	{"micro-star", "MSI"}, {"asustek", "ASUS"}, {"gigabyte", "Gigabyte"}, {"asrock", "ASRock"},
	{"hewlett-packard", "HP"}, {"hp ", "HP"}, {"dell", "Dell"}, {"lenovo", "Lenovo"}, {"acer", "Acer"},
	{"intel corporation", "Intel"}, {"apple", "Apple"}, {"microsoft corporation", "Microsoft"},
	{"samsung", "Samsung"}, {"biostar", "Biostar"}, {"evga", "EVGA"}, {"nzxt", "NZXT"}, {"razer", "Razer"},
	{"framework", "Framework"}, {"supermicro", "Supermicro"},
}

// shortMaker raccourcit le nom d'un fabricant connu.
func shortMaker(s string) string {
	s = clean(s)
	lower := strings.ToLower(s) + " "
	for _, m := range makers {
		if strings.HasPrefix(lower, m.prefix) {
			return m.name
		}
	}
	return s
}

// joinMaker associe fabricant et modèle sans répéter le fabricant (« Dell » + « XPS 15 9520 »).
func joinMaker(maker, model string) string {
	maker, model = shortMaker(maker), clean(model)
	switch {
	case model == "":
		return ""
	case maker == "" || strings.HasPrefix(strings.ToLower(model), strings.ToLower(maker)):
		return model
	}
	return maker + " " + model
}

// jedecMakers : fabricants de mémoire désignés par leur code JEDEC (« 80CE000080CE » : Samsung).
var jedecMakers = map[string]string{
	"80CE": "Samsung", "CE00": "Samsung", "80AD": "SK hynix", "AD00": "SK hynix", "802C": "Micron",
	"2C00": "Micron", "0198": "Kingston", "9801": "Kingston", "029E": "Corsair", "9E02": "Corsair",
	"04CD": "G.Skill", "CD04": "G.Skill", "859B": "Crucial", "9B05": "Crucial", "0443": "Ramaxel",
	"4304": "Ramaxel", "8551": "Qimonda", "02FE": "Elpida", "FE02": "Elpida", "0194": "Smart Modular",
	"04EF": "Team Group", "EF04": "Team Group", "0783": "Transcend", "8394": "Mushkin", "04CB": "A-DATA",
	"CB04": "A-DATA", "0B4E": "Patriot", "4E0B": "Patriot",
}

var hexCode = regexp.MustCompile(`^(0x)?[0-9A-Fa-f]{4,16}$`)

// memoryMaker donne le fabricant d'une barrette ; un code JEDEC inconnu est ignoré.
func memoryMaker(s string) string {
	s = clean(s)
	if !hexCode.MatchString(s) {
		return shortMaker(s)
	}
	code := strings.ToUpper(strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X"))
	return jedecMakers[code[:4]]
}

// fromFirmware complète la fiche avec les tables SMBIOS.
func fromFirmware(s *protocol.Specs, fw firmware) {
	if s.Model == "" {
		s.Model = fw.model
	}
	if fw.board != nil && s.Board == nil {
		s.Board = &protocol.SpecsBoard{Maker: shortMaker(fw.board.maker), Model: fw.board.model, BIOS: fw.board.bios, BIOSDate: fw.board.biosDate}
	}
	if len(fw.cpus) > 0 {
		if s.CPU == nil {
			s.CPU = &protocol.SpecsCPU{}
		}
		cores, threads := 0, 0
		for _, c := range fw.cpus {
			cores += c.cores
			threads += c.threads
		}
		if s.CPU.Name == "" {
			s.CPU.Name = cpuName(fw.cpus[0].name)
		}
		if s.CPU.Cores == 0 {
			s.CPU.Cores = cores
		}
		if s.CPU.Threads == 0 {
			s.CPU.Threads = threads
		}
		if s.CPU.MHz == 0 {
			s.CPU.MHz = fw.cpus[0].mhz
		}
		if len(fw.cpus) > 1 {
			s.CPU.Count = len(fw.cpus)
		}
	}
	if len(fw.modules) > 0 {
		if s.Memory == nil {
			s.Memory = &protocol.SpecsMemory{}
		}
		var total uint64
		s.Memory.Modules = nil
		for _, m := range fw.modules {
			total += m.size
			s.Memory.Modules = append(s.Memory.Modules, protocol.SpecsModule{Slot: m.slot, Size: m.size, Type: m.kind, MTs: m.mts, Maker: m.maker, Part: m.part})
		}
		// Mémoire installée (le système en voit un peu moins : réservée au matériel).
		s.Memory.Total = total
	}
	if s.Memory != nil && fw.slots >= len(fw.modules) && fw.slots > 0 {
		s.Memory.Slots = fw.slots
	}
}

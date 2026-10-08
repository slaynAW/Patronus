package specs

import (
	"encoding/binary"
	"fmt"
	"strings"
)

// Lecture des tables SMBIOS du micrologiciel (DMI), communes à Windows (GetSystemFirmwareTable) et
// à Linux (/sys/firmware/dmi/tables/DMI) : BIOS, modèle du PC, carte mère, processeurs, barrettes.
// Les numéros de série et identifiants uniques ne sont jamais lus.

// structure SMBIOS : type, zone formatée (en-tête compris) et chaînes.
type structure struct {
	kind    byte
	data    []byte
	strings []string
}

// byteAt, word, dword et qword lisent la zone formatée ; ok est faux si elle est trop courte.
func (s structure) byteAt(off int) (byte, bool) {
	if off >= len(s.data) {
		return 0, false
	}
	return s.data[off], true
}

func (s structure) word(off int) (uint16, bool) {
	if off+2 > len(s.data) {
		return 0, false
	}
	return binary.LittleEndian.Uint16(s.data[off:]), true
}

func (s structure) dword(off int) (uint32, bool) {
	if off+4 > len(s.data) {
		return 0, false
	}
	return binary.LittleEndian.Uint32(s.data[off:]), true
}

// str renvoie la chaîne désignée par l'octet à off (numérotées à partir de 1, 0 : aucune).
func (s structure) str(off int) string {
	i, ok := s.byteAt(off)
	if !ok || i == 0 || int(i) > len(s.strings) {
		return ""
	}
	return s.strings[i-1]
}

// parseSMBIOS découpe une table SMBIOS brute en structures (jusqu'à la structure de fin, type 127).
func parseSMBIOS(table []byte) ([]structure, error) {
	var out []structure
	for pos := 0; pos+4 <= len(table); {
		kind, length := table[pos], int(table[pos+1])
		if length < 4 || pos+length > len(table) {
			return out, fmt.Errorf("structure SMBIOS invalide à l'octet %d", pos)
		}
		s := structure{kind: kind, data: table[pos : pos+length]}
		// Chaînes : terminées par un octet nul, la liste par un second.
		end := pos + length
		for {
			if end >= len(table) {
				return out, fmt.Errorf("chaînes SMBIOS non terminées à l'octet %d", pos)
			}
			n := 0
			for end+n < len(table) && table[end+n] != 0 {
				n++
			}
			if n == 0 {
				end++
				break
			}
			s.strings = append(s.strings, string(table[end:end+n]))
			end += n + 1
		}
		if len(s.strings) == 0 && end < len(table) && table[end] == 0 {
			end++ // pas de chaînes : deux octets nuls
		}
		out = append(out, s)
		if kind == 127 {
			break
		}
		pos = end
	}
	return out, nil
}

// firmware : ce que les tables SMBIOS décrivent.
type firmware struct {
	model   string
	board   *boardInfo
	cpus    []cpuInfo
	slots   int
	modules []moduleInfo
}

type boardInfo struct {
	maker, model, bios, biosDate string
}

type cpuInfo struct {
	name           string
	cores, threads int
	mhz            int
}

type moduleInfo struct {
	slot        string
	size        uint64
	kind        string
	mts         int
	maker, part string
}

// memoryTypes : type de mémoire SMBIOS (structure 17, octet 0x12).
var memoryTypes = map[byte]string{
	0x12: "DDR", 0x13: "DDR2", 0x14: "DDR2", 0x18: "DDR3", 0x1A: "DDR4", 0x1B: "LPDDR", 0x1C: "LPDDR2",
	0x1D: "LPDDR3", 0x1E: "LPDDR4", 0x22: "DDR5", 0x23: "LPDDR5",
}

// readFirmware interprète les structures SMBIOS.
func readFirmware(structures []structure) firmware {
	var fw firmware
	var bios boardInfo
	var board boardInfo
	var systemMaker, systemModel string
	arrays := map[uint16]int{} // tableaux de mémoire système : emplacements
	for _, s := range structures {
		switch s.kind {
		case 0: // BIOS
			bios.bios = clean(s.str(5))
			bios.biosDate = biosDate(s.str(8))
		case 1: // système
			systemMaker, systemModel = clean(s.str(4)), clean(s.str(5))
		case 2: // carte mère
			board.maker, board.model = clean(s.str(4)), clean(s.str(5))
		case 4: // processeur
			status, _ := s.byteAt(0x18)
			if status&0x40 == 0 {
				continue // emplacement vide
			}
			c := cpuInfo{name: clean(s.str(0x10))}
			if v, ok := s.word(0x16); ok && v > 0 && v < 0xFFFF {
				c.mhz = int(v)
			}
			if v, ok := s.byteAt(0x23); ok {
				c.cores = int(v)
				if v == 0xFF {
					if w, ok := s.word(0x2A); ok {
						c.cores = int(w)
					}
				}
			}
			if v, ok := s.byteAt(0x25); ok {
				c.threads = int(v)
				if v == 0xFF {
					if w, ok := s.word(0x2E); ok {
						c.threads = int(w)
					}
				}
			}
			fw.cpus = append(fw.cpus, c)
		case 16: // tableau de mémoire
			use, _ := s.byteAt(5)
			if use != 3 { // mémoire système (pas la mémoire vidéo, le cache…)
				continue
			}
			if n, ok := s.word(0x0D); ok {
				handle, _ := s.word(2)
				arrays[handle] = int(n)
			}
		case 17: // barrette
			if m, ok := memoryModule(s); ok {
				fw.modules = append(fw.modules, m)
			}
		}
	}
	for _, n := range arrays {
		fw.slots += n
	}
	if board.maker != "" || board.model != "" || bios.bios != "" {
		board.bios, board.biosDate = bios.bios, bios.biosDate
		fw.board = &board
	}
	// Modèle du PC : seulement s'il dit quelque chose de plus que la carte mère (PC de marque, portable).
	if systemModel != "" && !strings.EqualFold(systemModel, board.model) {
		fw.model = joinMaker(systemMaker, systemModel)
	}
	return fw
}

// memoryModule lit une barrette (structure 17) ; ok est faux pour un emplacement vide.
func memoryModule(s structure) (moduleInfo, bool) {
	raw, ok := s.word(0x0C)
	if !ok || raw == 0 || raw == 0xFFFF {
		return moduleInfo{}, false
	}
	var size uint64
	switch {
	case raw == 0x7FFF:
		ext, ok := s.dword(0x1C)
		if !ok {
			return moduleInfo{}, false
		}
		size = uint64(ext&0x7FFFFFFF) << 20
	case raw&0x8000 != 0:
		size = uint64(raw&0x7FFF) << 10
	default:
		size = uint64(raw) << 20
	}
	if tech, ok := s.byteAt(0x28); ok && tech > 3 {
		return moduleInfo{}, false // mémoire non volatile (NVDIMM, Optane…)
	}
	m := moduleInfo{slot: clean(s.str(0x10)), size: size, maker: memoryMaker(s.str(0x17)), part: clean(s.str(0x1A))}
	if t, ok := s.byteAt(0x12); ok {
		m.kind = memoryTypes[t]
	}
	speed := func(off, extOff int) int {
		v, ok := s.word(off)
		if !ok || v == 0 {
			return 0
		}
		if v == 0xFFFF {
			ext, ok := s.dword(extOff)
			if !ok {
				return 0
			}
			return int(ext)
		}
		return int(v)
	}
	if m.mts = speed(0x20, 0x58); m.mts == 0 {
		m.mts = speed(0x15, 0x54)
	}
	if m.slot == "" {
		m.slot = clean(s.str(0x11))
	}
	return m, true
}

// biosDate convertit la date du BIOS (« 03/12/2024 ») en « 2024-03-12 ».
func biosDate(s string) string {
	var month, day, year int
	if n, _ := fmt.Sscanf(strings.TrimSpace(s), "%d/%d/%d", &month, &day, &year); n != 3 || month < 1 || month > 12 || day < 1 || day > 31 {
		return ""
	}
	if year < 100 {
		year += 1900
		if year < 1980 {
			year += 100
		}
	}
	return fmt.Sprintf("%04d-%02d-%02d", year, month, day)
}

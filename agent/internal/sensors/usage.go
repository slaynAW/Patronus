package sensors

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// usageWindow : durée de la mesure de l'utilisation (le Gestionnaire des tâches se rafraîchit
// chaque seconde). Le relevé est fait en arrière-plan : la réponse à « status » n'attend pas.
const usageWindow = time.Second

// percent arrondit une utilisation au dixième, ramenée entre 0 et 100 ; nil si elle n'a pas de sens.
func percent(v float64) *float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return nil
	}
	r := math.Round(min(v, 100)*10) / 10
	return &r
}

// busyPercent : part du temps où le processeur a travaillé entre deux relevés de ses compteurs
// cumulés (temps total et temps d'inactivité, dans la même unité).
func busyPercent(idle0, total0, idle1, total1 uint64) *float64 {
	if total1 <= total0 || idle1 < idle0 {
		return nil
	}
	total, idle := float64(total1-total0), float64(idle1-idle0)
	return percent((1 - idle/total) * 100)
}

// parseProcStat lit la ligne « cpu » de /proc/stat (Linux) : temps d'inactivité (idle + iowait)
// et temps total, en centièmes de seconde.
func parseProcStat(data string) (idle, total uint64, ok bool) {
	for line := range strings.SplitSeq(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || fields[0] != "cpu" {
			continue
		}
		// user nice system idle iowait irq softirq steal guest guest_nice ; guest et guest_nice
		// sont déjà comptés dans user et nice.
		for i, f := range fields[1:min(len(fields), 9)] {
			v, err := strconv.ParseUint(f, 10, 64)
			if err != nil {
				return 0, 0, false
			}
			total += v
			if i == 3 || i == 4 {
				idle += v
			}
		}
		return idle, total, true
	}
	return 0, 0, false
}

// gpuEngineInstance : instance du compteur « GPU Engine » de Windows, une par processus et par
// moteur : « pid_1234_luid_0x00000000_0x0000F0B6_phys_0_eng_0_engtype_3D ».
var gpuEngineInstance = regexp.MustCompile(`(?i)luid_0x([0-9a-f]+)_0x([0-9a-f]+)_phys_(\d+)_eng_(\d+)`)

// engineSample : utilisation d'un moteur graphique par un processus.
type engineSample struct {
	instance string
	value    float64
}

// gpuEngineLoads calcule l'utilisation de chaque carte graphique (clé : LUID) comme le
// Gestionnaire des tâches : utilisation de chaque moteur (3D, copie, vidéo…) additionnée sur les
// processus, puis moteur le plus occupé de la carte.
func gpuEngineLoads(samples []engineSample) map[uint64]float64 {
	type engine struct {
		luid      uint64
		phys, eng string
	}
	engines := map[engine]float64{}
	for _, s := range samples {
		m := gpuEngineInstance.FindStringSubmatch(s.instance)
		if m == nil || math.IsNaN(s.value) || s.value < 0 {
			continue
		}
		high, err1 := strconv.ParseUint(m[1], 16, 32)
		low, err2 := strconv.ParseUint(m[2], 16, 32)
		if err1 != nil || err2 != nil {
			continue
		}
		engines[engine{high<<32 | low, m[3], m[4]}] += s.value
	}
	loads := map[uint64]float64{}
	for e, v := range engines {
		loads[e.luid] = max(loads[e.luid], min(v, 100))
	}
	return loads
}

// loadCard : carte graphique vue par Windows et son identifiant (LUID).
type loadCard struct {
	name string
	luid uint64
}

// pickGPULoad renvoie l'utilisation de la carte graphique affichée (name, celle dont la
// température est connue) ; sans carte affichée, celle de la carte la plus occupée et son nom.
// loads vient du compteur « GPU Engine » (nil s'il n'existe pas) : une carte qui n'y figure pas
// est inactive. Une seule carte : c'est forcément elle, même si les noms diffèrent (lu par LHM).
func pickGPULoad(cards []loadCard, loads map[uint64]float64, name string) (*float64, string) {
	if loads == nil || len(cards) == 0 {
		return nil, ""
	}
	if name != "" {
		for _, c := range cards {
			if strings.EqualFold(strings.TrimSpace(c.name), strings.TrimSpace(name)) {
				return percent(loads[c.luid]), c.name
			}
		}
		if len(cards) == 1 {
			return percent(loads[cards[0].luid]), name
		}
		return nil, ""
	}
	best := cards[0]
	for _, c := range cards[1:] {
		if loads[c.luid] > loads[best.luid] {
			best = c
		}
	}
	return percent(loads[best.luid]), best.name
}

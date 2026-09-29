// Package sensors lit les températures du processeur et de la carte graphique, renvoyées par la
// commande « status » :
//
//   - Windows : carte graphique NVIDIA par NVML (bibliothèque installée avec le pilote), processeur
//     par LibreHardwareMonitor s'il tourne (Windows ne donne pas cette température sans pilote
//     noyau, que l'agent n'installe pas) ;
//   - Linux : capteurs du noyau (hwmon), et nvidia-smi pour les cartes NVIDIA.
//
// Les relevés sont faits en arrière-plan, au plus toutes les quelques secondes et seulement tant
// que des applications les demandent : répondre à « status » reste immédiat.
package sensors

import (
	"encoding/json"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol"
)

// Read lit les capteurs de ce PC (une implémentation par système ; peut prendre une seconde).
func Read() protocol.Temperatures { return read() }

// Cache sert le dernier relevé tout de suite et le renouvelle en arrière-plan.
type Cache struct {
	read func() protocol.Temperatures
	// Interval : âge à partir duquel une demande déclenche un nouveau relevé.
	Interval time.Duration
	// MaxAge : au-delà, le relevé n'est plus renvoyé (plus personne ne demandait).
	MaxAge time.Duration
	now    func() time.Time

	mu         sync.Mutex
	last       protocol.Temperatures
	at         time.Time
	refreshing bool
}

// NewCache renvoie un cache (relevé toutes les 5 s au plus, valable 30 s) autour de read.
func NewCache(read func() protocol.Temperatures) *Cache {
	return &Cache{read: read, Interval: 5 * time.Second, MaxAge: 30 * time.Second, now: time.Now}
}

// Get renvoie le dernier relevé (nil s'il n'y en a pas encore, ou s'il est trop ancien) et lance
// un nouveau relevé en arrière-plan quand celui-ci a vieilli.
func (c *Cache) Get() *protocol.Temperatures {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if !c.refreshing && now.Sub(c.at) >= c.Interval {
		c.refreshing = true
		go c.refresh()
	}
	if c.at.IsZero() || now.Sub(c.at) > c.MaxAge || c.last == (protocol.Temperatures{}) {
		return nil
	}
	t := c.last
	return &t
}

func (c *Cache) refresh() {
	t := c.read()
	c.mu.Lock()
	c.last, c.at, c.refreshing = t, c.now(), false
	c.mu.Unlock()
}

// celsius arrondit au dixième ; nil pour une valeur hors de toute plage plausible.
func celsius(v float64) *float64 {
	if math.IsNaN(v) || v <= 0 || v >= 150 {
		return nil
	}
	r := math.Round(v*10) / 10
	return &r
}

// --- LibreHardwareMonitor ---

// lhmSensor est un capteur de température publié par LibreHardwareMonitor.
type lhmSensor struct {
	// Identifier : « /intelcpu/0/temperature/0 », « /gpu-nvidia/0/temperature/0 »…
	Identifier string
	Name       string
	Value      float64
}

// cpuPreference : noms des capteurs qui représentent le mieux le processeur, du meilleur au moins bon.
var cpuPreference = []string{"cpu package", "core (tctl/tdie)", "core (tdie)", "core (tctl)", "package", "core max", "core average"}

// pickCPU choisit la température du processeur parmi les capteurs de LibreHardwareMonitor.
func pickCPU(sensors []lhmSensor) *float64 {
	var cpu []lhmSensor
	for _, s := range sensors {
		if strings.HasPrefix(s.Identifier, "/intelcpu/") || strings.HasPrefix(s.Identifier, "/amdcpu/") {
			cpu = append(cpu, s)
		}
	}
	for _, want := range cpuPreference {
		for _, s := range cpu {
			if strings.EqualFold(strings.TrimSpace(s.Name), want) {
				if t := celsius(s.Value); t != nil {
					return t
				}
			}
		}
	}
	// Sinon : le cœur le plus chaud.
	var best *float64
	for _, s := range cpu {
		if t := celsius(s.Value); t != nil && (best == nil || *t > *best) {
			best = t
		}
	}
	return best
}

// pickGPU choisit la température de la carte graphique (« GPU Core »), en secours de NVML.
func pickGPU(sensors []lhmSensor) *float64 {
	for _, s := range sensors {
		if strings.HasPrefix(s.Identifier, "/gpu-") && strings.EqualFold(strings.TrimSpace(s.Name), "GPU Core") {
			if t := celsius(s.Value); t != nil {
				return t
			}
		}
	}
	return nil
}

var leadingNumber = regexp.MustCompile(`^-?[0-9]+(?:[.,][0-9]+)?`)

// parseLHMWeb lit l'arbre publié par le serveur web de LibreHardwareMonitor (data.json), dont les
// valeurs sont du texte formaté selon la langue de Windows (« 54,5 °C »).
func parseLHMWeb(data []byte) ([]lhmSensor, error) {
	type node struct {
		Text     string `json:"Text"`
		Value    string `json:"Value"`
		SensorID string `json:"SensorId"`
		Type     string `json:"Type"`
		Children []node `json:"Children"`
	}
	var root node
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	var out []lhmSensor
	var walk func(n node)
	walk = func(n node) {
		if n.SensorID != "" && (n.Type == "Temperature" || strings.Contains(n.SensorID, "/temperature/")) {
			if m := leadingNumber.FindString(strings.TrimSpace(n.Value)); m != "" {
				if v, err := strconv.ParseFloat(strings.Replace(m, ",", ".", 1), 64); err == nil {
					out = append(out, lhmSensor{Identifier: n.SensorID, Name: n.Text, Value: v})
				}
			}
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(root)
	return out, nil
}

// --- Linux ---

// hwmonChip est une puce de capteurs du noyau (/sys/class/hwmon/hwmonN).
type hwmonChip struct {
	Name  string
	Temps map[string]float64 // libellé (ou « tempN ») → °C
}

// fromHwmon choisit les températures du processeur et de la carte graphique parmi les puces hwmon.
func fromHwmon(chips []hwmonChip) (cpu, gpu *float64) {
	label := func(c hwmonChip, labels ...string) *float64 {
		for _, l := range labels {
			if v, ok := c.Temps[l]; ok {
				if t := celsius(v); t != nil {
					return t
				}
			}
		}
		// Sinon, la première température (dans l'ordre des noms).
		names := make([]string, 0, len(c.Temps))
		for n := range c.Temps {
			names = append(names, n)
		}
		slices.Sort(names)
		for _, n := range names {
			if t := celsius(c.Temps[n]); t != nil {
				return t
			}
		}
		return nil
	}
	for _, c := range chips {
		switch c.Name {
		case "coretemp":
			cpu = firstNonNil(cpu, label(c, "Package id 0"))
		case "k10temp", "zenpower":
			cpu = firstNonNil(cpu, label(c, "Tdie", "Tctl"))
		case "cpu_thermal", "soc_thermal":
			cpu = firstNonNil(cpu, label(c))
		case "amdgpu", "nouveau", "radeon":
			gpu = firstNonNil(gpu, label(c, "edge"))
		}
	}
	return cpu, gpu
}

func firstNonNil(a, b *float64) *float64 {
	if a != nil {
		return a
	}
	return b
}

// parseNvidiaSMI lit « nvidia-smi --query-gpu=temperature.gpu,name --format=csv,noheader,nounits »
// (une ligne par carte : la plus chaude est retenue).
func parseNvidiaSMI(out string) (gpu *float64, name string) {
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		temp, n, ok := strings.Cut(line, ",")
		if !ok {
			continue
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(temp), 64)
		if err != nil {
			continue
		}
		if t := celsius(v); t != nil && (gpu == nil || *t > *gpu) {
			gpu, name = t, strings.TrimSpace(n)
		}
	}
	return gpu, name
}

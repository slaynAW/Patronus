// Package sensors lit les températures du processeur et de la carte graphique, renvoyées par la
// commande « status » :
//
//   - Windows : cartes graphiques par l'interface du noyau graphique (comme le Gestionnaire des
//     tâches) et par NVML pour NVIDIA, processeur par LibreHardwareMonitor s'il tourne (Windows ne
//     donne pas cette température sans pilote noyau, que l'agent n'installe pas) ;
//   - Linux : capteurs du noyau (hwmon), et nvidia-smi pour les cartes NVIDIA.
//
// Les relevés sont faits en arrière-plan, au plus toutes les quelques secondes et seulement tant
// que des applications les demandent : répondre à « status » reste immédiat.
package sensors

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"math"
	"net"
	"net/netip"
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

// Details décrit ce que l'agent trouve pour lire les températures (cartes graphiques,
// LibreHardwareMonitor…), pour « wol-agent status » et le rapport de diagnostic.
func Details() []string { return details() }

// gpuReading est la température d'une carte graphique.
type gpuReading struct {
	temp float64
	name string
}

// hottestGPU renvoie la carte graphique la plus chaude parmi les relevés valables.
func hottestGPU(readings []gpuReading) (*float64, string) {
	var best *float64
	var name string
	for _, r := range readings {
		if t := celsius(r.temp); t != nil && (best == nil || *t > *best) {
			best, name = t, r.name
		}
	}
	return best, name
}

// --- LibreHardwareMonitor ---

// lhmSensor est un capteur de température publié par LibreHardwareMonitor.
type lhmSensor struct {
	// Identifier : « /intelcpu/0/temperature/0 », « /gpu-nvidia/0/temperature/0 »…
	Identifier string
	Name       string
	Value      float64
	// Hardware : nom du matériel (« NVIDIA GeForce RTX 4070 ») quand il est connu.
	Hardware string
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

// gpuSecondary : capteurs d'une carte graphique qui ne représentent pas la puce elle-même.
var gpuSecondary = []string{"memory", "hot spot", "hotspot", "junction", "vram", "vr ", "vrm", "board"}

// pickGPU choisit la température de la carte graphique (en secours des lectures directes) :
// « GPU Core », sinon un capteur de la puce (les puces intégrées Intel, lues par LHM 0.9.6 ou
// plus, n'ont pas toujours ce nom), sinon n'importe quel capteur de la carte.
func pickGPU(sensors []lhmSensor) (*float64, string) {
	var gpu []lhmSensor
	for _, s := range sensors {
		if strings.HasPrefix(s.Identifier, "/gpu-") && celsius(s.Value) != nil {
			gpu = append(gpu, s)
		}
	}
	core := func(s lhmSensor) bool { return strings.EqualFold(strings.TrimSpace(s.Name), "GPU Core") }
	chip := func(s lhmSensor) bool {
		name := strings.ToLower(s.Name) + " "
		return !slices.ContainsFunc(gpuSecondary, func(w string) bool { return strings.Contains(name, w) })
	}
	for _, match := range []func(lhmSensor) bool{core, chip, func(lhmSensor) bool { return true }} {
		for _, s := range gpu {
			if match(s) {
				return celsius(s.Value), s.Hardware
			}
		}
	}
	return nil, ""
}

var leadingNumber = regexp.MustCompile(`^-?[0-9]+(?:[.,][0-9]+)?`)

// parseLHMWeb lit l'arbre publié par le serveur web de LibreHardwareMonitor (data.json). La valeur
// brute (RawValue, en °C) est préférée ; sinon Value, texte formaté selon la langue de Windows et
// l'unité choisie dans LHM (« 54,5 °C », « 130,1 °F »).
func parseLHMWeb(data []byte) ([]lhmSensor, error) {
	type node struct {
		Text     string          `json:"Text"`
		Value    json.RawMessage `json:"Value"`
		RawValue json.RawMessage `json:"RawValue"`
		SensorID string          `json:"SensorId"`
		Type     string          `json:"Type"`
		Children []node          `json:"Children"`
	}
	var root node
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	var out []lhmSensor
	// Arbre : ordinateur → matériel → groupe (« Temperatures ») → capteur ; parents = textes des
	// nœuds au-dessus du nœud courant.
	var walk func(n node, parents []string)
	walk = func(n node, parents []string) {
		if n.SensorID != "" && (n.Type == "Temperature" || strings.Contains(n.SensorID, "/temperature/")) {
			if v, ok := lhmValue(n.RawValue, n.Value); ok {
				s := lhmSensor{Identifier: n.SensorID, Name: n.Text, Value: v}
				if len(parents) >= 2 {
					s.Hardware = parents[len(parents)-2]
				}
				out = append(out, s)
			}
		}
		parents = append(parents, n.Text)
		for _, c := range n.Children {
			walk(c, parents)
		}
	}
	walk(root, nil)
	return out, nil
}

// lhmValue lit la valeur d'un capteur de data.json en °C.
func lhmValue(raw, text json.RawMessage) (float64, bool) {
	if v, ok := jsonNumber(raw); ok {
		return v, true
	}
	var s string
	if json.Unmarshal(text, &s) != nil {
		return jsonNumber(text)
	}
	s = strings.TrimSpace(s)
	m := leadingNumber.FindString(s)
	if m == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(strings.Replace(m, ",", ".", 1), 64)
	if err != nil {
		return 0, false
	}
	if strings.HasSuffix(s, "°F") {
		v = (v - 32) * 5 / 9
	}
	return v, true
}

// jsonNumber lit un nombre JSON, ou un texte qui en contient un (« 54.5 ») ; null n'en est pas un.
func jsonNumber(raw json.RawMessage) (float64, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return 0, false
	}
	var f float64
	if json.Unmarshal(raw, &f) == nil {
		return f, true
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if f, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

// lhmConfig : réglages de LibreHardwareMonitor utiles à l'agent, lus dans LibreHardwareMonitor.config
// (à côté du programme). LHM ne les enregistre pas toujours tout de suite : ils guident la lecture
// sans la conditionner.
type lhmConfig struct {
	// WebServer : Options → Remote Web Server → Run (runWebServerMenuItem).
	WebServer bool
	// IP : adresse d'écoute choisie (listenerIp) ; « ? » ou vide = toutes les adresses.
	IP string
	// Port : listenerPort, 8085 par défaut.
	Port int
	// Auth : mot de passe demandé (authenticationEnabled).
	Auth bool
}

const lhmDefaultPort = 8085

func parseLHMConfig(data []byte) (lhmConfig, error) {
	cfg := lhmConfig{Port: lhmDefaultPort}
	var doc struct {
		Settings []struct {
			Key   string `xml:"key,attr"`
			Value string `xml:"value,attr"`
		} `xml:"appSettings>add"`
	}
	if err := xml.Unmarshal(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")), &doc); err != nil {
		return cfg, err
	}
	for _, s := range doc.Settings {
		value := strings.TrimSpace(s.Value)
		switch s.Key {
		case "runWebServerMenuItem":
			cfg.WebServer = strings.EqualFold(value, "true")
		case "listenerIp":
			cfg.IP = value
		case "listenerPort":
			if p, err := strconv.Atoi(value); err == nil && p > 0 && p < 65536 {
				cfg.Port = p
			}
		case "authenticationEnabled":
			cfg.Auth = strings.EqualFold(value, "true")
		}
	}
	return cfg, nil
}

// lhmAddresses renvoie les adresses où interroger le serveur web : celle choisie dans LHM
// (Interface / Port) d'abord si c'est une adresse de ce PC (le serveur ne répond alors que sur
// elle), puis la boucle locale. Jamais une autre machine.
func lhmAddresses(cfg lhmConfig, local []netip.Addr) []string {
	port := strconv.Itoa(cfg.Port)
	var out []string
	if ip, err := netip.ParseAddr(cfg.IP); err == nil {
		ip = ip.Unmap()
		if !ip.IsLoopback() && !ip.IsUnspecified() && slices.Contains(local, ip) {
			out = append(out, net.JoinHostPort(ip.String(), port))
		}
	}
	return append(out, net.JoinHostPort("127.0.0.1", port))
}

// lhmState explique l'absence de température du processeur (protocol.LHM…) d'après ce qui a été
// constaté : programme trouvé, mot de passe demandé, capteurs lus.
func lhmState(running, auth, answered bool) string {
	switch {
	case answered:
		return protocol.LHMNoSensor
	case auth:
		return protocol.LHMAuth
	case running:
		return protocol.LHMWebOff
	}
	return protocol.LHMNotRunning
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

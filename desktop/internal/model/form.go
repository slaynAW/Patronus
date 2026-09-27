package model

import (
	"crypto/rand"
	"fmt"
	"strconv"
	"strings"
)

// DeviceForm est le contenu brut du formulaire d'édition, tel que saisi (comme sur Android).
type DeviceForm struct {
	Name         string `json:"name"`
	MAC          string `json:"mac"`
	Host         string `json:"host"`
	AgentEnabled bool   `json:"agentEnabled"`
	AgentPort    string `json:"agentPort"`
	AgentKey     string `json:"agentKey"`
	Broadcast    string `json:"broadcast"`
	WolPort      string `json:"wolPort"`
	ProbePorts   string `json:"probePorts"`
	SecureOn     string `json:"secureOn"`
}

// NewForm renvoie le formulaire d'un nouveau PC.
func NewForm() DeviceForm {
	return DeviceForm{
		AgentPort:  strconv.Itoa(DefaultAgentPort),
		WolPort:    strconv.Itoa(DefaultWolPort),
		ProbePorts: joinPorts(DefaultProbePorts()),
	}
}

// FormFrom remplit le formulaire depuis un PC existant.
func FormFrom(d Device) DeviceForm {
	f := DeviceForm{
		Name:       d.Name,
		MAC:        d.MAC.String(),
		Host:       d.Host,
		AgentPort:  strconv.Itoa(DefaultAgentPort),
		WolPort:    strconv.Itoa(d.WolPort),
		ProbePorts: joinPorts(d.ProbePorts),
	}
	if d.Agent != nil {
		f.AgentEnabled = true
		f.AgentPort = strconv.Itoa(d.Agent.Port)
		f.AgentKey = d.Agent.Key
	}
	if d.BroadcastAddress != nil {
		f.Broadcast = *d.BroadcastAddress
	}
	if d.SecureOnPassword != nil {
		f.SecureOn = *d.SecureOnPassword
	}
	return f
}

// Build convertit le formulaire en PC ; en cas d'erreur, renvoie un message par champ.
func (f DeviceForm) Build(id string) (Device, map[Field]string) {
	errs := map[Field]string{}
	mac, macErr := ParseMAC(f.MAC)
	if macErr != nil {
		errs[FieldMAC] = "Adresse MAC invalide (ex. AA:BB:CC:DD:EE:FF)"
	}
	wolPort, wolOK := parseInt(f.WolPort)
	if !wolOK {
		errs[FieldWolPort] = "Nombre attendu"
	}
	var probePorts []int
	seen := map[int]bool{}
	for _, part := range strings.FieldsFunc(f.ProbePorts, func(r rune) bool { return r == ',' || r == ';' || r == ' ' }) {
		if strings.TrimSpace(part) == "" {
			continue
		}
		port, ok := parseInt(part)
		if !ok {
			errs[FieldProbePorts] = "Liste de ports séparés par des virgules"
			continue
		}
		if !seen[port] {
			seen[port] = true
			probePorts = append(probePorts, port)
		}
	}
	agentPort, agentOK := parseInt(f.AgentPort)
	if f.AgentEnabled && !agentOK {
		errs[FieldAgentPort] = "Nombre attendu"
	}
	if macErr != nil || len(errs) > 0 {
		return Device{}, errs
	}

	if id == "" {
		id = NewID()
	}
	device := Device{
		ID:         id,
		Name:       strings.TrimSpace(f.Name),
		MAC:        mac,
		Host:       strings.TrimSpace(f.Host),
		WolPort:    wolPort,
		ProbePorts: probePorts,
	}
	if probePorts == nil {
		device.ProbePorts = []int{}
	}
	if v := strings.TrimSpace(f.Broadcast); v != "" {
		device.BroadcastAddress = &v
	}
	if v := strings.TrimSpace(f.SecureOn); v != "" {
		device.SecureOnPassword = &v
	}
	if f.AgentEnabled {
		device.Agent = &AgentSettings{Port: agentPort, Key: strings.TrimSpace(f.AgentKey)}
	}
	for _, e := range Validate(device) {
		if _, exists := errs[e.Field]; !exists {
			errs[e.Field] = e.Message
		}
	}
	if len(errs) > 0 {
		return Device{}, errs
	}
	return device, nil
}

// NewID génère un identifiant aléatoire au format UUID v4 (comme Android).
func NewID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func parseInt(s string) (int, bool) {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	return v, err == nil
}

func joinPorts(ports []int) string {
	parts := make([]string, len(ports))
	for i, p := range ports {
		parts[i] = strconv.Itoa(p)
	}
	return strings.Join(parts, ", ")
}

package model

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	// DefaultWolPort est le port UDP standard du Wake-on-LAN (« discard »).
	DefaultWolPort = 9
	// DefaultAgentPort est le port TCP par défaut de l'agent installé sur les PC.
	DefaultAgentPort = 9770
)

// DefaultProbePorts sont les ports TCP testés pour savoir si un PC est allumé sans agent :
// RDP, SMB, SSH et NetBIOS. Une connexion acceptée OU refusée prouve que la machine répond.
func DefaultProbePorts() []int { return []int{3389, 445, 22, 139} }

// AgentSettings contient les paramètres de connexion à l'agent d'un PC.
type AgentSettings struct {
	Port int `json:"port"`
	// Key est la clé secrète partagée (32 octets en Base64 URL). Vide = à renseigner.
	Key string `json:"key"`
}

// HasKey indique si une clé est renseignée.
func (a AgentSettings) HasKey() bool { return a.Key != "" }

func (a AgentSettings) String() string {
	key := "<vide>"
	if a.HasKey() {
		key = "***"
	}
	return fmt.Sprintf("AgentSettings(port=%d, key=%s)", a.Port, key)
}

func (a *AgentSettings) UnmarshalJSON(data []byte) error {
	var raw struct {
		Port *int    `json:"port"`
		Key  *string `json:"key"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw.Key == nil {
		return missing("agent.key")
	}
	a.Port = DefaultAgentPort
	if raw.Port != nil {
		a.Port = *raw.Port
	}
	a.Key = *raw.Key
	return nil
}

// Device est un terminal (PC, serveur, NAS...) à réveiller et surveiller.
type Device struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	MAC  MAC    `json:"mac"`
	// Host est l'adresse IP ou le nom d'hôte, pour le suivi d'état et l'agent. Vide = pas de suivi.
	Host string `json:"host"`
	// BroadcastAddress force l'adresse de diffusion ; nil = calculée automatiquement.
	BroadcastAddress *string `json:"broadcastAddress,omitempty"`
	WolPort          int     `json:"wolPort"`
	// SecureOnPassword est exigé par de rares cartes réseau (6 octets hexadécimaux).
	SecureOnPassword *string        `json:"secureOnPassword,omitempty"`
	ProbePorts       []int          `json:"probePorts"`
	Agent            *AgentSettings `json:"agent,omitempty"`
}

// HasHost indique si une adresse est renseignée (suivi d'état possible).
func (d Device) HasHost() bool { return strings.TrimSpace(d.Host) != "" }

// CanShutdown indique si le PC peut être éteint à distance (agent configuré).
func (d Device) CanShutdown() bool { return d.HasHost() && d.Agent != nil }

// WithoutSecrets renvoie une copie sans clé d'agent ni mot de passe SecureOn (export en clair).
func (d Device) WithoutSecrets() Device {
	d.SecureOnPassword = nil
	if d.Agent != nil {
		d.Agent = &AgentSettings{Port: d.Agent.Port, Key: ""}
	}
	d.ProbePorts = append([]int(nil), d.ProbePorts...)
	return d
}

// Clone renvoie une copie indépendante (pointeurs et listes dupliqués).
func (d Device) Clone() Device {
	if d.BroadcastAddress != nil {
		v := *d.BroadcastAddress
		d.BroadcastAddress = &v
	}
	if d.SecureOnPassword != nil {
		v := *d.SecureOnPassword
		d.SecureOnPassword = &v
	}
	if d.Agent != nil {
		v := *d.Agent
		d.Agent = &v
	}
	d.ProbePorts = append([]int{}, d.ProbePorts...)
	return d
}

// String masque les secrets : jamais de clé dans les journaux.
func (d Device) String() string {
	secureOn := "none"
	if d.SecureOnPassword != nil {
		secureOn = "***"
	}
	return fmt.Sprintf("Device(id=%s, name=%s, mac=%s, host=%s, wolPort=%d, secureOn=%s, probePorts=%v, agent=%v)",
		d.ID, d.Name, d.MAC, d.Host, d.WolPort, secureOn, d.ProbePorts, d.Agent)
}

func (d Device) MarshalJSON() ([]byte, error) {
	type plain Device
	if d.ProbePorts == nil {
		d.ProbePorts = []int{}
	}
	return json.Marshal(plain(d))
}

func (d *Device) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID               *string        `json:"id"`
		Name             *string        `json:"name"`
		MAC              *MAC           `json:"mac"`
		Host             *string        `json:"host"`
		BroadcastAddress *string        `json:"broadcastAddress"`
		WolPort          *int           `json:"wolPort"`
		SecureOnPassword *string        `json:"secureOnPassword"`
		ProbePorts       *[]int         `json:"probePorts"`
		Agent            *AgentSettings `json:"agent"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	switch {
	case raw.ID == nil:
		return missing("id")
	case raw.Name == nil:
		return missing("name")
	case raw.MAC == nil:
		return missing("mac")
	}
	*d = Device{
		ID:               *raw.ID,
		Name:             *raw.Name,
		MAC:              *raw.MAC,
		BroadcastAddress: raw.BroadcastAddress,
		WolPort:          DefaultWolPort,
		SecureOnPassword: raw.SecureOnPassword,
		ProbePorts:       DefaultProbePorts(),
		Agent:            raw.Agent,
	}
	if raw.Host != nil {
		d.Host = *raw.Host
	}
	if raw.WolPort != nil {
		d.WolPort = *raw.WolPort
	}
	if raw.ProbePorts != nil {
		d.ProbePorts = append([]int{}, *raw.ProbePorts...)
	}
	return nil
}

func missing(field string) error { return fmt.Errorf("champ « %s » manquant", field) }

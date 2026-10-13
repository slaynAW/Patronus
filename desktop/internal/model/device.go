package model

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/slaynaw/wakeonlan/agent/protocol2"
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
	// Key est la clé secrète partagée de wolagent/1 (32 octets en Base64 URL). Vide = à renseigner,
	// ou effacée une fois le PC passé en wolagent/2.
	Key string `json:"key"`
	// Pin est l'empreinte wolagent/2 de l'agent (B64), lue dans une réponse signée par la clé ou dans
	// un lien d'appairage. Une fois connue, l'application ne parle plus que wolagent/2 à ce PC
	// (docs/PROTOCOLE-V2.md, §9). Ce n'est pas un secret.
	Pin string `json:"pin,omitempty"`
}

// HasKey indique si une clé wolagent/1 est renseignée.
func (a AgentSettings) HasKey() bool { return a.Key != "" }

// HasPin indique que l'agent est joint en wolagent/2.
func (a AgentSettings) HasPin() bool { return a.Pin != "" }

// Ready indique que l'agent peut être joint : par sa clé (wolagent/1) ou son empreinte (wolagent/2).
func (a AgentSettings) Ready() bool { return a.HasKey() || a.HasPin() }

// ShortPin renvoie l'empreinte courte de l'agent (« 7KQ2-9MXP-4D1Z »), ou "".
func (a AgentSettings) ShortPin() string {
	pin, err := protocol2.ParsePin(a.Pin)
	if err != nil {
		return ""
	}
	return pin.Short()
}

func (a AgentSettings) String() string {
	key := "<vide>"
	if a.HasKey() {
		key = "***"
	}
	pin := ""
	if a.HasPin() {
		pin = ", pin=" + a.ShortPin()
	}
	return fmt.Sprintf("AgentSettings(port=%d, key=%s%s)", a.Port, key, pin)
}

func (a *AgentSettings) UnmarshalJSON(data []byte) error {
	var raw struct {
		Port *int    `json:"port"`
		Key  *string `json:"key"`
		Pin  string  `json:"pin"`
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
	a.Pin = raw.Pin
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
// L'empreinte de l'agent, publique, est gardée.
func (d Device) WithoutSecrets() Device {
	d.SecureOnPassword = nil
	if d.Agent != nil {
		d.Agent = &AgentSettings{Port: d.Agent.Port, Key: "", Pin: d.Agent.Pin}
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

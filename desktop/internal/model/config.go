package model

import "encoding/json"

// CurrentSchemaVersion est la version du format de configuration (identique à Android).
const CurrentSchemaVersion = 1

// Bornes et valeurs par défaut des réglages (identiques à Android).
const (
	DefaultPollIntervalSeconds = 3
	DefaultWakeTimeoutSeconds  = 180
	MinPollIntervalSeconds     = 1
	MaxPollIntervalSeconds     = 60
	MinWakeTimeoutSeconds      = 30
	MaxWakeTimeoutSeconds      = 900
)

// AppSettings contient les réglages généraux.
type AppSettings struct {
	// PollIntervalSeconds est l'intervalle entre deux vérifications d'état.
	PollIntervalSeconds int `json:"pollIntervalSeconds"`
	// ConfirmPowerActions demande une confirmation avant d'éteindre / redémarrer / mettre en veille.
	ConfirmPowerActions bool `json:"confirmPowerActions"`
	// WakeTimeoutSeconds est l'attente maximale du démarrage après l'envoi du paquet magique.
	WakeTimeoutSeconds int `json:"wakeTimeoutSeconds"`
}

// DefaultSettings renvoie les réglages d'origine.
func DefaultSettings() AppSettings {
	return AppSettings{
		PollIntervalSeconds: DefaultPollIntervalSeconds,
		ConfirmPowerActions: true,
		WakeTimeoutSeconds:  DefaultWakeTimeoutSeconds,
	}
}

func (s *AppSettings) UnmarshalJSON(data []byte) error {
	type plain AppSettings
	value := plain(DefaultSettings())
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*s = AppSettings(value)
	return nil
}

// Clamped remet les réglages dans leurs bornes.
func (s AppSettings) Clamped() AppSettings {
	s.PollIntervalSeconds = clamp(s.PollIntervalSeconds, MinPollIntervalSeconds, MaxPollIntervalSeconds)
	s.WakeTimeoutSeconds = clamp(s.WakeTimeoutSeconds, MinWakeTimeoutSeconds, MaxWakeTimeoutSeconds)
	return s
}

// AppConfig est la configuration complète : ce qui est stocké (chiffré) et exporté / importé.
type AppConfig struct {
	SchemaVersion int         `json:"schemaVersion"`
	Devices       []Device    `json:"devices"`
	Settings      AppSettings `json:"settings"`
}

// NewConfig renvoie une configuration vide.
func NewConfig() AppConfig {
	return AppConfig{SchemaVersion: CurrentSchemaVersion, Devices: []Device{}, Settings: DefaultSettings()}
}

func (c AppConfig) MarshalJSON() ([]byte, error) {
	type plain AppConfig
	if c.Devices == nil {
		c.Devices = []Device{}
	}
	return json.Marshal(plain(c))
}

func (c *AppConfig) UnmarshalJSON(data []byte) error {
	type plain AppConfig
	value := plain(NewConfig())
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	if value.Devices == nil {
		value.Devices = []Device{}
	}
	*c = AppConfig(value)
	return nil
}

// Clone renvoie une copie indépendante.
func (c AppConfig) Clone() AppConfig {
	devices := make([]Device, len(c.Devices))
	for i, d := range c.Devices {
		devices[i] = d.Clone()
	}
	c.Devices = devices
	return c
}

// Device renvoie le terminal d'identifiant id.
func (c AppConfig) Device(id string) (Device, bool) {
	for _, d := range c.Devices {
		if d.ID == id {
			return d, true
		}
	}
	return Device{}, false
}

func (c AppConfig) index(id string) int {
	for i, d := range c.Devices {
		if d.ID == id {
			return i
		}
	}
	return -1
}

// Upsert ajoute ou remplace (même identifiant) un terminal, en conservant l'ordre.
func (c AppConfig) Upsert(device Device) AppConfig {
	c = c.Clone()
	if i := c.index(device.ID); i >= 0 {
		c.Devices[i] = device
	} else {
		c.Devices = append(c.Devices, device)
	}
	return c
}

// Remove retire un terminal.
func (c AppConfig) Remove(id string) AppConfig {
	c = c.Clone()
	kept := c.Devices[:0]
	for _, d := range c.Devices {
		if d.ID != id {
			kept = append(kept, d)
		}
	}
	c.Devices = kept
	return c
}

// Move déplace un terminal de offset positions (−1 = monter, +1 = descendre).
func (c AppConfig) Move(id string, offset int) AppConfig {
	i := c.index(id)
	target := i + offset
	if i < 0 || target < 0 || target >= len(c.Devices) {
		return c
	}
	c = c.Clone()
	device := c.Devices[i]
	c.Devices = append(c.Devices[:i], c.Devices[i+1:]...)
	c.Devices = append(c.Devices[:target], append([]Device{device}, c.Devices[target:]...)...)
	return c
}

// MergeDevicesFrom fusionne une configuration importée : terminaux de même identifiant remplacés,
// nouveaux ajoutés à la fin, réglages locaux conservés.
func (c AppConfig) MergeDevicesFrom(other AppConfig) AppConfig {
	for _, d := range other.Devices {
		c = c.Upsert(d)
	}
	return c
}

// HasSecrets indique si au moins un terminal contient une clé d'agent ou un mot de passe SecureOn.
func (c AppConfig) HasSecrets() bool {
	for _, d := range c.Devices {
		if (d.Agent != nil && d.Agent.HasKey()) || d.SecureOnPassword != nil {
			return true
		}
	}
	return false
}

func clamp(v, lo, hi int) int {
	return max(lo, min(v, hi))
}

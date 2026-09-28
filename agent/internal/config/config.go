// Package config gère le fichier de configuration de l'agent (clé secrète, port, réseaux autorisés).
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"

	"github.com/slaynaw/wakeonlan/agent/protocol"
)

// DefaultPort est le port TCP d'écoute par défaut.
const DefaultPort = 9770

// Commandes que l'agent sait exécuter.
var AllCommands = []string{"status", "shutdown", "reboot", "sleep"}

// DefaultAllow liste les réseaux autorisés par défaut : uniquement des adresses privées
// (réseau local, VPN type Tailscale en 100.64.0.0/10). Internet n'est jamais autorisé par défaut.
var DefaultAllow = []string{
	"127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "169.254.0.0/16", "100.64.0.0/10",
	"::1/128", "fc00::/7", "fe80::/10",
}

// Config est le contenu de config.json.
type Config struct {
	// Name est le nom proposé à l'application lors de l'appairage (nom du PC par défaut).
	Name string `json:"name"`
	// Port d'écoute TCP.
	Port int `json:"port"`
	// Key est la clé partagée (32 octets, Base64 URL). À garder secrète.
	Key string `json:"key"`
	// Allow liste les réseaux (CIDR) autorisés à se connecter.
	Allow []string `json:"allow"`
	// Commands liste les commandes autorisées (ex. retirer "shutdown" pour n'autoriser que la veille).
	Commands []string `json:"commands"`
}

// New crée une configuration neuve avec une clé aléatoire.
func New(name string, port int) (*Config, error) {
	key, err := protocol.NewKey()
	if err != nil {
		return nil, err
	}
	if name == "" {
		name, _ = os.Hostname()
	}
	if port == 0 {
		port = DefaultPort
	}
	return &Config{
		Name:     name,
		Port:     port,
		Key:      key,
		Allow:    append([]string(nil), DefaultAllow...),
		Commands: append([]string(nil), AllCommands...),
	}, nil
}

// Validate vérifie la cohérence de la configuration.
func (c *Config) Validate() error {
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("port invalide : %d", c.Port)
	}
	if _, err := protocol.DecodeKey(c.Key); err != nil {
		return err
	}
	if _, err := c.Networks(); err != nil {
		return err
	}
	for _, cmd := range c.Commands {
		if !contains(AllCommands, cmd) {
			return fmt.Errorf("commande inconnue : %q", cmd)
		}
	}
	return nil
}

// KeyBytes renvoie la clé décodée.
func (c *Config) KeyBytes() ([]byte, error) { return protocol.DecodeKey(c.Key) }

// Networks renvoie les réseaux autorisés.
func (c *Config) Networks() ([]*net.IPNet, error) {
	nets := make([]*net.IPNet, 0, len(c.Allow))
	for _, cidr := range c.Allow {
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("réseau autorisé invalide %q : %w", cidr, err)
		}
		nets = append(nets, n)
	}
	return nets, nil
}

// Allows indique si une commande est autorisée.
func (c *Config) Allows(cmd string) bool { return contains(c.Commands, cmd) }

// ListenAddr renvoie l'adresse d'écoute (toutes interfaces, IPv4 et IPv6).
func (c *Config) ListenAddr() string { return net.JoinHostPort("", strconv.Itoa(c.Port)) }

// Load lit et valide un fichier de configuration.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("configuration introuvable (%s) : lancez d'abord « wol-agent install »", path)
		}
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("configuration illisible (%s) : %w", path, err)
	}
	if c.Commands == nil {
		c.Commands = append([]string(nil), AllCommands...)
	}
	if c.Allow == nil {
		c.Allow = append([]string(nil), DefaultAllow...)
	}
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("configuration invalide (%s) : %w", path, err)
	}
	return &c, nil
}

// Save écrit la configuration de façon atomique, lisible uniquement par l'administrateur.
func Save(path string, c *Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := applyPermissions(path); err != nil {
		return err
	}
	// Journal et signe de vie sont créés dans ce dossier par le service : ils héritent de ses droits.
	return applyDirPermissions(filepath.Dir(path))
}

// applyPermissions et applyDirPermissions sont remplaçables dans les tests (les ACL Windows
// dépendent de l'environnement).
var (
	applyPermissions    = restrictPermissions
	applyDirPermissions = restrictDirPermissions
)

func contains(list []string, value string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}

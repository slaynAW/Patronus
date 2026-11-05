package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/slaynaw/wakeonlan/agent/protocol"
)

func TestNewSaveLoad(t *testing.T) {
	c, err := New("PC-TEST", 0)
	if err != nil {
		t.Fatal(err)
	}
	if c.Port != DefaultPort || c.Name != "PC-TEST" || !c.Allows("shutdown") {
		t.Fatalf("valeurs par défaut inattendues : %+v", c)
	}
	path := filepath.Join(t.TempDir(), "sub", "config.json")
	if runtime.GOOS == "windows" {
		// Les ACL Windows dépendent du compte de test : on ne teste ici que la sérialisation.
		applyPermissions = func(string) error { return nil }
		applyDirPermissions = func(string) error { return nil }
		defer func() { applyPermissions, applyDirPermissions = restrictPermissions, restrictDirPermissions }()
	}
	if err := Save(path, c); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Key != c.Key || loaded.Port != c.Port {
		t.Fatal("aller-retour incorrect")
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(path)
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("permissions %v", info.Mode().Perm())
		}
	}
}

func TestValidate(t *testing.T) {
	c, _ := New("x", 1234)
	c.Allow = []string{"pas-un-cidr"}
	if c.Validate() == nil {
		t.Fatal("CIDR invalide accepté")
	}
	c, _ = New("x", 1234)
	c.Commands = []string{"format-c"}
	if c.Validate() == nil {
		t.Fatal("commande inconnue acceptée")
	}
	c, _ = New("x", 1234)
	c.Key = "abc"
	if c.Validate() == nil {
		t.Fatal("clé invalide acceptée")
	}
	if _, err := Load(filepath.Join(t.TempDir(), "absent.json")); err == nil {
		t.Fatal("fichier absent accepté")
	}
}

func TestProtocols(t *testing.T) {
	c, err := New("PC-TEST", 0)
	if err != nil {
		t.Fatal(err)
	}
	// Nouvelle installation (agent 1.14.0) : wolagent/2 seulement, sans clé.
	if c.Accepts(ProtocolV1) || !c.Accepts(ProtocolV2) || c.Accepts("wolagent/3") || c.Key != "" {
		t.Fatalf("nouvelle installation : %+v", c)
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("nouvelle installation refusée : %v", err)
	}
	if raw, _ := json.Marshal(c); strings.Contains(string(raw), `"key"`) {
		t.Errorf("clé vide écrite : %s", raw)
	}
	// Installation antérieure : « protocols » absent, les deux protocoles acceptés avec la clé.
	key, err := protocol.NewKey()
	if err != nil {
		t.Fatal(err)
	}
	legacy := *c
	legacy.Protocols, legacy.Key = nil, key
	if err := legacy.Validate(); err != nil || !legacy.Accepts(ProtocolV1) || !legacy.Accepts(ProtocolV2) {
		t.Fatalf("installation antérieure : %v", err)
	}
	for name, mutate := range map[string]func(*Config){
		"aucun protocole":                func(c *Config) { c.Protocols = []string{} },
		"protocole inconnu":              func(c *Config) { c.Protocols = []string{ProtocolV2, "wolagent/3"} },
		"wolagent/1 sans clé":            func(c *Config) { c.Protocols, c.Key = []string{ProtocolV1}, "" },
		"clé invalide inutilisée":        func(c *Config) { c.Protocols, c.Key = []string{ProtocolV2}, "abc" },
		"wolagent/1 par défaut sans clé": func(c *Config) { c.Protocols, c.Key = nil, "" },
	} {
		bad := *c
		mutate(&bad)
		if err := bad.Validate(); err == nil {
			t.Errorf("%s : configuration acceptée", name)
		}
	}
}

func TestEnableDisableV1(t *testing.T) {
	c, err := New("PC-TEST", 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.EnableV1(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(c.Protocols, []string{ProtocolV1, ProtocolV2}) || c.Validate() != nil {
		t.Fatalf("wolagent/1 rétabli : %v", c.Protocols)
	}
	key := c.Key
	if _, err := c.KeyBytes(); err != nil {
		t.Fatalf("clé créée invalide : %v", err)
	}
	// Deuxième appel : rien ne change, la clé est gardée (les appareils déjà appairés la connaissent).
	if err := c.EnableV1(); err != nil || c.Key != key || len(c.Protocols) != 2 {
		t.Fatalf("second EnableV1 : %v %v", err, c.Protocols)
	}
	c.DisableV1()
	if !slices.Equal(c.Protocols, []string{ProtocolV2}) || c.Key != "" || c.Validate() != nil {
		t.Fatalf("wolagent/1 refusé : %v, clé %q", c.Protocols, c.Key)
	}
	// Installation antérieure (les deux par défaut) : wolagent/1 refusé, clé effacée.
	legacy := Config{Port: 1, Key: key}
	legacy.DisableV1()
	if legacy.Accepts(ProtocolV1) || legacy.Key != "" || legacy.Validate() != nil {
		t.Fatalf("installation antérieure : %v, clé %q", legacy.Protocols, legacy.Key)
	}
	// wolagent/1 seul (wolagent/2 retiré à la main) : EnableV1 ne rajoute pas wolagent/2.
	only := Config{Port: 1, Key: key, Protocols: []string{ProtocolV1}}
	if err := only.EnableV1(); err != nil || !slices.Equal(only.Protocols, []string{ProtocolV1}) || only.Key != key {
		t.Fatalf("wolagent/1 seul : %v %v", err, only.Protocols)
	}
}

package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
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
	if !c.Accepts(ProtocolV1) || !c.Accepts(ProtocolV2) || c.Accepts("wolagent/3") {
		t.Fatal("protocoles par défaut")
	}
	v2only := *c
	v2only.Protocols, v2only.Key = []string{ProtocolV2}, ""
	if err := v2only.Validate(); err != nil || v2only.Accepts(ProtocolV1) {
		t.Errorf("wolagent/2 seul, sans clé : %v", err)
	}
	for name, mutate := range map[string]func(*Config){
		"aucun protocole":                func(c *Config) { c.Protocols = []string{} },
		"protocole inconnu":              func(c *Config) { c.Protocols = []string{ProtocolV2, "wolagent/3"} },
		"wolagent/1 sans clé":            func(c *Config) { c.Protocols, c.Key = []string{ProtocolV1}, "" },
		"clé invalide inutilisée":        func(c *Config) { c.Protocols, c.Key = []string{ProtocolV2}, "abc" },
		"wolagent/1 par défaut sans clé": func(c *Config) { c.Key = "" },
	} {
		bad := *c
		mutate(&bad)
		if err := bad.Validate(); err == nil {
			t.Errorf("%s : configuration acceptée", name)
		}
	}
}

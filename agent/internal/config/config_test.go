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

package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/slaynaw/wakeonlan/desktop/internal/model"
)

// Store enregistre la configuration sur le disque, chiffrée pour l'utilisateur Windows courant
// (DPAPI : illisible depuis un autre compte ou un autre PC, même en copiant le fichier).
type Store struct {
	path string
}

// NewStore crée un magasin dans le dossier dir (créé si besoin).
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Store{path: filepath.Join(dir, fileName)}, nil
}

// Path renvoie le chemin du fichier de configuration.
func (s *Store) Path() string { return s.path }

// Load lit la configuration. Un fichier absent donne une configuration vide.
//
// Si le fichier est illisible (copié depuis un autre compte, abîmé...), il est mis de côté
// (renommé, jamais supprimé) et une configuration vide est renvoyée avec une erreur explicative.
func (s *Store) Load() (model.AppConfig, error) {
	sealed, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return model.NewConfig(), nil
	}
	if err != nil {
		return model.NewConfig(), err
	}
	plain, err := unprotect(sealed)
	var cfg model.AppConfig
	if err == nil {
		cfg, err = Decode(plain)
		clear(plain)
	}
	if err != nil {
		backup := s.path + ".illisible-" + time.Now().Format("20060102-150405")
		if renameErr := os.Rename(s.path, backup); renameErr != nil {
			return model.NewConfig(), fmt.Errorf("configuration illisible (%v)", err)
		}
		return model.NewConfig(), fmt.Errorf("configuration illisible, mise de côté dans %s (%v)", backup, err)
	}
	return cfg, nil
}

// Save enregistre la configuration de façon atomique (fichier temporaire puis renommage).
func (s *Store) Save(c model.AppConfig) error {
	plain, err := Encode(c)
	if err != nil {
		return err
	}
	sealed, err := protect(plain)
	clear(plain)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".config-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(sealed); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.path)
}

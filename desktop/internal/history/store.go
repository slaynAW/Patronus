package history

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Store enregistre l'historique sur le disque, chiffré pour l'utilisateur Windows (DPAPI) comme la
// configuration.
type Store struct {
	path string
}

// NewStore crée un magasin dans le dossier dir.
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Store{path: filepath.Join(dir, fileName)}, nil
}

// Load lit l'historique ; un fichier absent ou illisible donne un historique vide (ce n'est qu'un
// journal : il se reconstitue, notamment depuis les agents).
func (s *Store) Load() Data {
	sealed, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) || err != nil {
		return New()
	}
	plain, err := unprotect(sealed)
	if err != nil {
		return New()
	}
	var d Data
	if json.Unmarshal(plain, &d) != nil || d.Version != version {
		return New()
	}
	if d.Events == nil {
		d.Events = []Event{}
	}
	if d.Coverage == nil {
		d.Coverage = map[string]Coverage{}
	}
	return d
}

// Save enregistre l'historique de façon atomique.
func (s *Store) Save(d Data) error {
	plain, err := json.Marshal(d)
	if err != nil {
		return err
	}
	sealed, err := protect(plain)
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, sealed, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

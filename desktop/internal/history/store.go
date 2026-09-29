package history

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
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

// Load lit l'historique. Un fichier absent donne un historique vide ; un fichier illisible aussi
// (ce n'est qu'un journal : il se reconstitue, notamment depuis les agents), mais il est mis de côté
// (jamais supprimé) et l'erreur est renvoyée pour le journal de diagnostic.
func (s *Store) Load() (Data, error) {
	sealed, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return New(), nil
	}
	if err != nil {
		return New(), fmt.Errorf("historique illisible (%v)", err)
	}
	plain, err := unprotect(sealed)
	var d Data
	if err == nil {
		err = json.Unmarshal(plain, &d)
	}
	if err == nil && d.Version != version {
		err = fmt.Errorf("version %d inconnue", d.Version)
	}
	if err != nil {
		backup := s.path + ".illisible-" + time.Now().Format("20060102-150405")
		if renameErr := os.Rename(s.path, backup); renameErr != nil {
			return New(), fmt.Errorf("historique illisible (%v)", err)
		}
		return New(), fmt.Errorf("historique illisible, mis de côté dans %s (%v)", filepath.Base(backup), err)
	}
	if d.Events == nil {
		d.Events = []Event{}
	}
	if d.Coverage == nil {
		d.Coverage = map[string]Coverage{}
	}
	return d, nil
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

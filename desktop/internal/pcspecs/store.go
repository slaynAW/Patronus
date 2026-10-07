// Package pcspecs garde la fiche de chaque PC (commande « specs » de l'agent 1.10.0) sur le disque,
// chiffrée pour l'utilisateur Windows (DPAPI) comme l'historique : elle reste affichée PC éteint.
package pcspecs

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol"
)

const version = 1

// Entry est la fiche d'un PC.
type Entry struct {
	Specs protocol.Specs `json:"specs"`
	// Fetched : date de lecture (ms) ; Agent : version de l'agent qui l'a donnée.
	Fetched int64  `json:"fetched"`
	Agent   string `json:"agent,omitempty"`
}

type file struct {
	Version int              `json:"version"`
	Devices map[string]Entry `json:"devices"`
}

// Store enregistre les fiches ; nil garde tout en mémoire (tests).
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

// Load lit les fiches. Un fichier absent ou illisible donne une liste vide (elles se relisent sur
// les agents) ; un fichier illisible est mis de côté et l'erreur renvoyée pour le journal.
func (s *Store) Load() (map[string]Entry, error) {
	if s == nil {
		return map[string]Entry{}, nil
	}
	sealed, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]Entry{}, nil
	}
	if err != nil {
		return map[string]Entry{}, fmt.Errorf("fiches illisibles (%v)", err)
	}
	plain, err := unprotect(sealed)
	var f file
	if err == nil {
		err = json.Unmarshal(plain, &f)
	}
	if err == nil && f.Version != version {
		err = fmt.Errorf("version %d inconnue", f.Version)
	}
	if err != nil {
		backup := s.path + ".illisible-" + time.Now().Format("20060102-150405")
		if renameErr := os.Rename(s.path, backup); renameErr != nil {
			return map[string]Entry{}, fmt.Errorf("fiches illisibles (%v)", err)
		}
		return map[string]Entry{}, fmt.Errorf("fiches illisibles, mises de côté dans %s (%v)", filepath.Base(backup), err)
	}
	if f.Devices == nil {
		f.Devices = map[string]Entry{}
	}
	return f.Devices, nil
}

// Save enregistre les fiches de façon atomique.
func (s *Store) Save(devices map[string]Entry) error {
	if s == nil {
		return nil
	}
	plain, err := json.Marshal(file{Version: version, Devices: devices})
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

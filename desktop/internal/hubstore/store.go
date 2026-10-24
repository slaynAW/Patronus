// Package hubstore garde le hub auquel cette installation est appairée (docs/HUB.md, §5.2) : son nom,
// son adresse, son empreinte et le rôle reçu, chiffrés pour l'utilisateur Windows (DPAPI). Comme
// l'identité de l'installation, il ne figure dans aucune sauvegarde ni aucun partage : l'appairage
// appartient à cette installation.
package hubstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

const version = 1

// Hub est le hub appairé.
type Hub struct {
	Name string `json:"name"`
	Host string `json:"host"`
	Port int    `json:"port"`
	Pin  string `json:"pin"`
	// Role : rôle donné à cette installation par le hub (view, wake ou admin).
	Role string `json:"role"`
	// Paired : date de l'appairage (secondes Unix).
	Paired int64 `json:"paired"`
	// EventsSeen : heure (horloge du hub, secondes Unix) du dernier évènement vu dans « Pendant votre
	// absence » ; 0 : aucun, les évènements depuis l'appairage sont à voir.
	EventsSeen int64 `json:"eventsSeen,omitempty"`
}

type file struct {
	Version int  `json:"version"`
	Hub     *Hub `json:"hub"`
}

// Store enregistre le hub ; nil le garde en mémoire seulement (tests).
type Store struct {
	path string
	mem  *Hub
}

// NewStore crée un magasin dans le dossier dir.
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Store{path: filepath.Join(dir, fileName)}, nil
}

// Load lit le hub appairé : nil si aucun. Un fichier illisible est mis de côté (renommé, jamais
// supprimé) et l'erreur renvoyée pour le journal : il suffit d'appairer de nouveau.
func (s *Store) Load() (*Hub, error) {
	if s == nil {
		return nil, nil
	}
	if s.path == "" {
		return s.mem, nil
	}
	sealed, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("hub illisible (%v)", err)
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
			return nil, fmt.Errorf("hub illisible (%v)", err)
		}
		return nil, fmt.Errorf("hub illisible, mis de côté dans %s (%v)", filepath.Base(backup), err)
	}
	return f.Hub, nil
}

// Save enregistre le hub de façon atomique ; nil oublie le hub (fichier retiré).
func (s *Store) Save(h *Hub) error {
	if s == nil {
		return nil
	}
	if s.path == "" {
		s.mem = h
		return nil
	}
	if h == nil {
		if err := os.Remove(s.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	plain, err := json.Marshal(file{Version: version, Hub: h})
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

// Memory renvoie un magasin gardé en mémoire (tests).
func Memory() *Store { return &Store{} }

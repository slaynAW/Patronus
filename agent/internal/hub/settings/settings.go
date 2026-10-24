// Package settings garde les réglages du hub (docs/HUB.md, §9.1) : la durée de conservation de
// l'archive, dans settings.db du stockage chiffré.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/slaynaw/wakeonlan/agent/internal/hub/archive"
)

// FileName est le fichier des réglages, dans le stockage chiffré du hub.
const FileName = "settings.db"

// Storage garde les réglages (store.File en service).
type Storage interface {
	Load() ([]byte, error)
	Save(data []byte) error
}

// Values sont les réglages.
type Values struct {
	// RetentionDays est la durée de conservation de l'archive, en jours.
	RetentionDays int `json:"retention"`
}

// Defaults sont les réglages d'un hub neuf.
var Defaults = Values{RetentionDays: archive.DefaultRetentionDays}

// Settings garde les réglages, sûrs pour un usage concurrent.
type Settings struct {
	mu    sync.Mutex
	store Storage
	v     Values
}

// Open lit les réglages (ceux par défaut s'ils n'existent pas encore).
func Open(store Storage) (*Settings, error) {
	s := &Settings{store: store, v: Defaults}
	raw, err := store.Load()
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var v Values
	if err := json.Unmarshal(raw, &v); err != nil || Validate(v) != nil {
		return nil, fmt.Errorf("réglages du hub illisibles")
	}
	s.v = v
	return s, nil
}

// Validate vérifie des réglages.
func Validate(v Values) error {
	if v.RetentionDays < archive.MinRetentionDays || v.RetentionDays > archive.MaxRetentionDays {
		return fmt.Errorf("durée de conservation invalide : de %d à %d jours", archive.MinRetentionDays, archive.MaxRetentionDays)
	}
	return nil
}

// Get renvoie les réglages.
func (s *Settings) Get() Values {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.v
}

// Set remplace les réglages ; en cas d'échec, rien ne change.
func (s *Settings) Set(v Values) error {
	if err := Validate(v); err != nil {
		return err
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.store.Save(raw); err != nil {
		return err
	}
	s.v = v
	return nil
}

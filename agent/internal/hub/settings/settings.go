// Package settings garde les réglages du hub (docs/HUB.md, §9.1) : la durée de conservation de
// l'archive et le fuseau horaire des programmations, dans settings.db du stockage chiffré.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
	// Base des fuseaux horaires incluse dans le binaire : l'image du conteneur n'en a pas.
	_ "time/tzdata"

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
	// Timezone est le fuseau horaire des programmations (nom IANA, docs/PROTOCOLE-HUB.md, §6.7).
	Timezone string `json:"timezone,omitempty"`
}

// DefaultTimezone est le fuseau horaire d'un hub neuf.
const DefaultTimezone = "Europe/Paris"

// Defaults sont les réglages d'un hub neuf.
var Defaults = Values{RetentionDays: archive.DefaultRetentionDays, Timezone: DefaultTimezone}

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
	err = json.Unmarshal(raw, &v)
	if err == nil && v.Timezone == "" {
		v.Timezone = DefaultTimezone // réglages d'avant les programmations
	}
	if err != nil || Validate(v) != nil {
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
	if _, err := Location(v.Timezone); err != nil {
		return err
	}
	return nil
}

// Location renvoie le fuseau horaire de nom IANA name (« Europe/Paris »). « Local » est refusé : il
// dépendrait du système du conteneur.
func Location(name string) (*time.Location, error) {
	if name == "" || name == "Local" || len(name) > 64 {
		return nil, fmt.Errorf("fuseau horaire invalide : %q", name)
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("fuseau horaire inconnu : %q", name)
	}
	return loc, nil
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

package app

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/slaynaw/wakeonlan/agent/update"
)

// UpdateOptions active les mises à jour intégrées (nil : désactivées, par exemple pour une version
// de développement).
type UpdateOptions struct {
	Source update.Source
	// Code du build en cours (numéro d'exécution de la CI) : une version n'est proposée que si
	// son code est plus grand.
	Code int64
	// Platform : fichier à télécharger (« windows-amd64 », « windows-arm64 »).
	Platform string
	// Exe : exécutable à remplacer. Vide : la nouvelle version est téléchargée et vérifiée, puis
	// seulement annoncée (mode développement).
	Exe string
}

// Délais des vérifications automatiques.
const (
	updateFirstCheck = 10 * time.Second
	updateInterval   = 24 * time.Hour
	updatePostpone   = 24 * time.Hour
)

// UpdateView est l'état des mises à jour affiché par l'interface.
type UpdateView struct {
	// Enabled : mises à jour intégrées disponibles pour cette version de l'application.
	Enabled   bool        `json:"enabled"`
	Auto      bool        `json:"auto"`
	Checking  bool        `json:"checking"`
	LastCheck int64       `json:"lastCheck,omitempty"`
	Error     string      `json:"error,omitempty"`
	Available *UpdateInfo `json:"available,omitempty"`
	// Postponed : « Plus tard » choisi pour cette version (ne plus la proposer d'elle-même pendant 24 h).
	Postponed bool `json:"postponed,omitempty"`
	// Stage : "" (rien en cours), "downloading", "installing".
	Stage    string  `json:"stage,omitempty"`
	Progress float64 `json:"progress,omitempty"`
}

// UpdateInfo décrit la nouvelle version proposée.
type UpdateInfo struct {
	Version string `json:"version"`
	Date    string `json:"date,omitempty"`
	Notes   string `json:"notes"`
	Size    int64  `json:"size"`
}

// updatePrefs est conservé dans updates.json (réglage et dernière vérification ; rien de secret).
type updatePrefs struct {
	Auto             bool   `json:"auto"`
	LastCheck        int64  `json:"lastCheck,omitempty"`
	PostponedVersion string `json:"postponedVersion,omitempty"`
	PostponedAt      int64  `json:"postponedAt,omitempty"`
}

type updater struct {
	opts  *UpdateOptions
	path  string
	now   func() time.Time
	mu    sync.Mutex
	prefs updatePrefs
	view  UpdateView
	found *update.Manifest
}

func newUpdater(opts *UpdateOptions, dir string, now func() time.Time) *updater {
	u := &updater{opts: opts, path: filepath.Join(dir, "updates.json"), now: now, prefs: updatePrefs{Auto: true}}
	if data, err := os.ReadFile(u.path); err == nil {
		_ = json.Unmarshal(data, &u.prefs)
	}
	return u
}

func (u *updater) savePrefs() {
	data, _ := json.MarshalIndent(u.prefs, "", "  ")
	tmp := u.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err == nil {
		_ = os.Rename(tmp, u.path)
	}
}

func (u *updater) snapshot() UpdateView {
	u.mu.Lock()
	defer u.mu.Unlock()
	v := u.view
	v.Enabled = u.opts != nil
	v.Auto = u.prefs.Auto
	v.LastCheck = u.prefs.LastCheck
	if v.Available != nil {
		v.Postponed = u.prefs.PostponedVersion == v.Available.Version &&
			u.now().UnixMilli()-u.prefs.PostponedAt < updatePostpone.Milliseconds()
	}
	return v
}

// runUpdates vérifie les mises à jour peu après le démarrage puis une fois par jour.
func (s *Service) runUpdates(ctx context.Context) {
	if s.updates.opts == nil {
		return
	}
	timer := time.NewTimer(updateFirstCheck)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		s.updates.mu.Lock()
		auto := s.updates.prefs.Auto
		due := s.now().UnixMilli()-s.updates.prefs.LastCheck >= updateInterval.Milliseconds()-time.Minute.Milliseconds()
		s.updates.mu.Unlock()
		if auto && due {
			_ = s.checkUpdate(ctx)
		}
		timer.Reset(time.Hour)
	}
}

// checkUpdate recherche une nouvelle version (manuellement ou automatiquement).
func (s *Service) checkUpdate(ctx context.Context) error {
	u := s.updates
	if u.opts == nil {
		return errors.New("mises à jour indisponibles pour cette version de l'application")
	}
	u.mu.Lock()
	if u.view.Checking || u.view.Stage != "" {
		u.mu.Unlock()
		return nil
	}
	u.view.Checking = true
	u.view.Error = ""
	u.mu.Unlock()
	s.notify()

	m, err := u.opts.Source.Latest(ctx)
	published := false
	u.mu.Lock()
	u.view.Checking = false
	switch {
	case errors.Is(err, update.ErrNotPublished):
		err = nil
		u.found, u.view.Available = nil, nil
	case err != nil:
		u.view.Error = "Recherche impossible : " + err.Error()
	default:
		published = true
		file, ok := m.File(u.opts.Platform)
		if m.Code > u.opts.Code && ok {
			u.found = &m
			u.view.Available = &UpdateInfo{Version: m.Version, Date: m.Date, Notes: m.Notes, Size: file.Size}
		} else {
			u.found, u.view.Available = nil, nil
		}
	}
	if err == nil {
		u.prefs.LastCheck = s.now().UnixMilli()
		u.savePrefs()
	}
	u.mu.Unlock()
	if published {
		s.noteAgentRelease(m) // hors du verrou : notify relit tout l'état
	}
	s.notify()
	return err
}

// installUpdate télécharge la nouvelle version, la vérifie, remplace l'exécutable et relance
// l'application (en arrière-plan : l'avancement est visible dans l'état).
func (s *Service) installUpdate() error {
	u := s.updates
	u.mu.Lock()
	if u.opts == nil || u.found == nil {
		u.mu.Unlock()
		return errors.New("aucune mise à jour à installer")
	}
	if u.view.Stage != "" {
		u.mu.Unlock()
		return nil
	}
	m := *u.found
	file, _ := m.File(u.opts.Platform)
	u.view.Stage, u.view.Progress, u.view.Error = "downloading", 0, ""
	u.mu.Unlock()
	s.notify()

	go func() {
		err := s.downloadAndApply(m, file)
		u.mu.Lock()
		u.view.Stage, u.view.Progress = "", 0
		if err != nil {
			u.view.Error = "Mise à jour impossible : " + err.Error()
		}
		u.mu.Unlock()
		s.notify()
		if err != nil {
			log.Printf("mise à jour : %v", err)
		}
	}()
	return nil
}

func (s *Service) downloadAndApply(m update.Manifest, file update.File) error {
	u := s.updates
	dir := os.TempDir()
	if u.opts.Exe != "" {
		dir = filepath.Dir(u.opts.Exe) // même volume : remplacement par simple renommage
	}
	next := filepath.Join(dir, "WakeOnLan.update.exe")
	lastNotify := time.Time{}
	err := u.opts.Source.Download(context.Background(), m, file, next, func(done, total int64) {
		u.mu.Lock()
		u.view.Progress = float64(done) / float64(total)
		u.mu.Unlock()
		if time.Since(lastNotify) > 150*time.Millisecond {
			lastNotify = time.Now()
			s.notify()
		}
	})
	if err != nil {
		return err
	}
	u.mu.Lock()
	u.view.Stage, u.view.Progress = "installing", 1
	u.mu.Unlock()
	s.notify()
	if u.opts.Exe == "" {
		_ = os.Remove(next) // mode développement : vérifiée, mais pas installée
	} else if err := update.Replace(u.opts.Exe, next); err != nil {
		_ = os.Remove(next)
		return err
	}
	return s.platform.Relaunch()
}

func (s *Service) postponeUpdate() {
	u := s.updates
	u.mu.Lock()
	if u.view.Available != nil {
		u.prefs.PostponedVersion = u.view.Available.Version
		u.prefs.PostponedAt = s.now().UnixMilli()
		u.savePrefs()
	}
	u.mu.Unlock()
	s.notify()
}

func (s *Service) setAutoUpdate(enabled bool) {
	u := s.updates
	u.mu.Lock()
	u.prefs.Auto = enabled
	u.savePrefs()
	u.mu.Unlock()
	s.notify()
}

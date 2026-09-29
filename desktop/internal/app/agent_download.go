package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/slaynaw/wakeonlan/agent/update"
)

// Téléchargement de l'agent depuis l'application : la dernière version publiée est vérifiée avec le
// manifeste signé (signature, taille et empreinte SHA-256, comme les mises à jour de l'application),
// puis enregistrée où l'utilisateur le souhaite (pour un autre PC) ou installée sur ce PC.

// AgentOptions active le téléchargement de l'agent (nil : indisponible).
type AgentOptions struct {
	Source update.Source
	// Platform : plateforme de ce PC (« windows-amd64 », « windows-arm64 »).
	Platform string
	// Dir : dossier de travail des téléchargements, réservé à cet usage (vidé au démarrage).
	Dir string
}

// agentPlatforms : agents proposés par l'application Windows.
var agentPlatforms = []string{"windows-amd64", "windows-arm64"}

// AgentView est l'état du téléchargement de l'agent affiché par l'interface.
type AgentView struct {
	Enabled bool `json:"enabled"`
	// Latest : dernière version publiée de l'agent, connue après une recherche de mise à jour ou
	// l'ouverture du téléchargement (vide : inconnue).
	Latest string `json:"latest,omitempty"`
	// Busy et Progress : téléchargement en cours (avancement de 0 à 1).
	Busy     bool    `json:"busy,omitempty"`
	Progress float64 `json:"progress,omitempty"`
}

// AgentRelease décrit la dernière version de l'agent (réponse à « agentInfo »).
type AgentRelease struct {
	Version string `json:"version"`
	Date    string `json:"date,omitempty"`
	Notes   string `json:"notes"`
	// Platform : plateforme de ce PC ; Sizes : taille du fichier de chaque plateforme disponible.
	Platform string           `json:"platform"`
	Sizes    map[string]int64 `json:"sizes"`
}

type agentDownloads struct {
	opts     *AgentOptions
	mu       sync.Mutex
	manifest *update.Manifest
	view     AgentView
}

func newAgentDownloads(opts *AgentOptions) *agentDownloads {
	if opts != nil && opts.Dir != "" {
		_ = os.RemoveAll(opts.Dir) // restes d'un téléchargement précédent
	}
	return &agentDownloads{opts: opts}
}

func (a *agentDownloads) snapshot() AgentView {
	a.mu.Lock()
	defer a.mu.Unlock()
	v := a.view
	v.Enabled = a.opts != nil
	return v
}

// noteRelease retient le manifeste d'une version publiée (recherche de mise à jour comprise) : la
// fiche des PC peut alors signaler un agent plus ancien.
func (s *Service) noteAgentRelease(m update.Manifest) {
	if m.Agent == nil {
		return
	}
	a := s.agents
	a.mu.Lock()
	changed := a.view.Latest != m.Agent.Version
	a.manifest, a.view.Latest = &m, m.Agent.Version
	a.mu.Unlock()
	if changed {
		s.notify()
	}
}

// agentInfo recherche la dernière version publiée de l'agent.
func (s *Service) agentInfo(ctx context.Context) (AgentRelease, error) {
	a := s.agents
	if a.opts == nil {
		return AgentRelease{}, errors.New("téléchargement de l'agent indisponible pour cette version de l'application")
	}
	m, err := a.opts.Source.Latest(ctx)
	if errors.Is(err, update.ErrNotPublished) {
		return AgentRelease{}, errors.New("aucune version publiée pour le moment")
	}
	if err != nil {
		return AgentRelease{}, errors.New("Recherche impossible : " + err.Error())
	}
	if m.Agent == nil {
		return AgentRelease{}, errors.New("la dernière version publiée ne contient pas l'agent")
	}
	s.noteAgentRelease(m)
	info := AgentRelease{Version: m.Agent.Version, Date: m.Date, Notes: m.Agent.Notes, Platform: a.opts.Platform, Sizes: map[string]int64{}}
	for _, p := range agentPlatforms {
		if f, ok := m.Agent.File(p); ok {
			info.Sizes[p] = f.Size
		}
	}
	return info, nil
}

// agentDownload télécharge et vérifie l'agent d'une plateforme, puis l'installe sur ce PC (install)
// ou propose de l'enregistrer.
func (s *Service) agentDownload(ctx context.Context, platform string, install bool) (any, error) {
	a := s.agents
	if a.opts == nil {
		return nil, errors.New("téléchargement de l'agent indisponible pour cette version de l'application")
	}
	if !slices.Contains(agentPlatforms, platform) {
		return nil, errors.New("plateforme inconnue")
	}
	if install && platform != a.opts.Platform {
		return nil, errors.New("cet agent n'est pas prévu pour le processeur de ce PC")
	}
	a.mu.Lock()
	if a.view.Busy {
		a.mu.Unlock()
		return nil, errors.New("téléchargement déjà en cours")
	}
	a.view.Busy, a.view.Progress = true, 0
	m := a.manifest
	a.mu.Unlock()
	s.notify()
	defer func() {
		a.mu.Lock()
		a.view.Busy, a.view.Progress = false, 0
		a.mu.Unlock()
		s.notify()
	}()

	if m == nil {
		if _, err := s.agentInfo(ctx); err != nil {
			return nil, err
		}
		a.mu.Lock()
		m = a.manifest
		a.mu.Unlock()
	}
	file, ok := m.Agent.File(platform)
	if !ok {
		return nil, errors.New("agent introuvable pour ce processeur dans la dernière version")
	}
	if err := os.MkdirAll(a.opts.Dir, 0o700); err != nil {
		return nil, err
	}
	// Nom validé par le manifeste signé (lettres, chiffres, « . », « _ », « - ») : reste dans Dir.
	dest := filepath.Join(a.opts.Dir, file.Name)
	lastNotify := time.Time{}
	err := a.opts.Source.Download(ctx, *m, file, dest, func(done, total int64) {
		a.mu.Lock()
		a.view.Progress = float64(done) / float64(total)
		a.mu.Unlock()
		if time.Since(lastNotify) > 150*time.Millisecond {
			lastNotify = time.Now()
			s.notify()
		}
	})
	if err != nil {
		return nil, errors.New("Téléchargement impossible : " + err.Error())
	}

	if install {
		// La plateforme revérifie l'empreinte sur le fichier verrouillé avant de le lancer, puis
		// l'efface une fois l'installation terminée.
		err := s.platform.RunInstaller(dest, file.SHA256)
		if errors.Is(err, ErrCancelled) {
			_ = os.Remove(dest)
			return map[string]any{"cancelled": true}, nil
		}
		if err != nil {
			_ = os.Remove(dest)
			return nil, err
		}
		return map[string]any{"installing": true, "version": m.Agent.Version}, nil
	}

	data, err := os.ReadFile(dest)
	_ = os.Remove(dest)
	if err != nil {
		return nil, err
	}
	path, err := s.platform.SaveFile(file.Name, data)
	if errors.Is(err, ErrCancelled) {
		return map[string]any{"cancelled": true}, nil
	}
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, errors.New("enregistrement indisponible dans ce mode")
	}
	return map[string]any{"path": path, "version": m.Agent.Version}, nil
}

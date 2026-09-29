// Package selfupdate met l'agent à jour à partir des versions publiées (manifeste signé, voir le
// package update) :
//
//   - une fois par jour, le service lit le manifeste de la dernière version et compare le numéro de
//     l'agent au sien ;
//   - si une version plus récente existe, l'utilisateur connecté au PC est prévenu par une fenêtre et
//     choisit « Oui » (installer) ou « Non » (rappel le lendemain) ;
//   - la nouvelle version est téléchargée à côté de l'exécutable installé, contrôlée (taille,
//     empreinte SHA-256 annoncée par le manifeste signé, numéro affiché par « wol-agent version »),
//     puis remplace l'ancienne, gardée en « .old » ;
//   - un processus détaché (« wol-agent update-finish », nouvelle version) redémarre le service et
//     vérifie qu'il répond, sinon remet l'ancienne version en place.
//
// La configuration (clé, port, réseaux autorisés) et le journal ne changent pas : pas de ré-appairage.
// L'installation n'est prise en charge que sous Windows.
package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/slaynaw/wakeonlan/agent/update"
)

const (
	// Interval sépare deux recherches de mise à jour.
	Interval = 24 * time.Hour
	// FirstDelay laisse le PC finir de démarrer avant la première recherche.
	FirstDelay = 3 * time.Minute
	// RemindAfter est le délai avant de reproposer une version refusée (« Non »).
	RemindAfter = 24 * time.Hour
	// NoUserRetry : personne n'est connecté au PC, nouvel essai plus tard.
	NoUserRetry = time.Hour
	// AskTimeout : sans réponse, la fenêtre se ferme et la question sera reposée le lendemain.
	AskTimeout = 12 * time.Hour
	// Title est le titre des fenêtres affichées à l'utilisateur.
	Title = "Patronus – agent"

	nextName = "wol-agent-update.exe"
)

// testAccept : test de bout en bout de la CI uniquement (-ldflags -X …testAccept=1), jamais dans les
// versions publiées : la mise à jour est recherchée dès le démarrage du service et acceptée sans fenêtre.
var testAccept string

// StartDelay est le délai avant la première recherche.
func StartDelay() time.Duration {
	if testAccept == "1" {
		return 2 * time.Second
	}
	return FirstDelay
}

// Offer est une nouvelle version de l'agent, prête à être proposée.
type Offer struct {
	Manifest update.Manifest
	// Version est le numéro de l'agent proposé (distinct de celui des applications).
	Version string
	Notes   string
	File    update.File
}

// Platform est la plateforme des fichiers de l'agent pour ce système (« windows-amd64 »…).
func Platform() string { return runtime.GOOS + "-" + runtime.GOARCH }

// Check renvoie la version de l'agent à proposer, ou nil si current est à jour (ou si la dernière
// version publiée ne contient pas d'agent pour ce système).
func Check(ctx context.Context, src update.Source, current string) (*Offer, error) {
	m, err := src.Latest(ctx)
	if err != nil {
		return nil, err
	}
	if m.Agent == nil || !update.Newer(m.Agent.Version, current) {
		return nil, nil
	}
	f, ok := m.Agent.File(Platform())
	if !ok {
		return nil, nil
	}
	return &Offer{Manifest: m, Version: m.Agent.Version, Notes: m.Agent.Notes, File: f}, nil
}

// Download télécharge la version proposée à côté de exe (même volume, dossier réservé aux
// administrateurs), vérifie qu'elle se lance et annonce le numéro attendu, et renvoie son chemin.
func Download(ctx context.Context, src update.Source, o Offer, exe string) (string, error) {
	next := filepath.Join(filepath.Dir(exe), nextName)
	_ = os.Remove(next)
	if err := src.Download(ctx, o.Manifest, o.File, next, nil); err != nil {
		return "", err
	}
	_ = os.Chmod(next, 0o755) // sans effet sous Windows
	got, err := ProbeVersion(ctx, next)
	if err == nil && got != o.Version {
		err = fmt.Errorf("elle annonce la version %q", got)
	}
	if err != nil {
		_ = os.Remove(next)
		return "", fmt.Errorf("la version téléchargée ne démarre pas correctement : %w", err)
	}
	return next, nil
}

// ProbeVersion lance « <exe> version » et renvoie le numéro affiché.
func ProbeVersion(ctx context.Context, exe string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, exe, "version").Output()
	if err != nil {
		return "", err
	}
	v, ok := strings.CutPrefix(strings.TrimSpace(string(out)), "wol-agent ")
	if !ok || v == "" {
		return "", errors.New("numéro de version illisible")
	}
	return v, nil
}

// Message est le texte de la fenêtre qui propose la mise à jour.
func Message(current string, o Offer) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Une nouvelle version de l'agent Patronus est disponible : %s → %s.\n", current, o.Version)
	if notes := plainNotes(o.Notes); notes != "" {
		b.WriteString("\n" + notes + "\n")
	}
	b.WriteString("\nL'installer maintenant ? L'agent redémarre en quelques secondes ; " +
		"sa clé et l'appairage avec vos appareils sont conservés.\n\n" +
		"Oui : installer        Non : me le rappeler demain")
	return b.String()
}

// plainNotes adapte les nouveautés (Markdown simple) à une fenêtre de texte brut.
func plainNotes(notes string) string {
	var lines []string
	for line := range strings.SplitSeq(notes, "\n") {
		line = strings.TrimSpace(strings.ReplaceAll(line, "**", ""))
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "#"):
			line = strings.TrimSpace(strings.TrimLeft(line, "#"))
		case strings.HasPrefix(line, "- "):
			line = "• " + line[2:]
		}
		lines = append(lines, line)
		if len(lines) == 8 {
			break
		}
	}
	return strings.Join(lines, "\n")
}

// Answer est la réponse de l'utilisateur à la proposition de mise à jour.
type Answer int

const (
	// Later : « Non », fenêtre fermée ou restée sans réponse.
	Later Answer = iota
	// Accept : « Oui ».
	Accept
	// NoUser : personne n'est connecté au PC pour répondre.
	NoUser
)

// state est conservé à côté de la configuration : un refus survit au redémarrage du PC.
type state struct {
	RemindAfter time.Time `json:"remindAfter"`
	Version     string    `json:"version,omitempty"`
}

// Auto recherche et propose les mises à jour tant que le service tourne.
type Auto struct {
	Source  update.Source
	Current string
	// StatePath : fichier du report (« Non »).
	StatePath string
	// Ask pose la question à l'utilisateur connecté.
	Ask func(ctx context.Context, o Offer) (Answer, error)
	// Apply installe la version acceptée (téléchargement, remplacement, redémarrage du service).
	Apply  func(ctx context.Context, o Offer) error
	Logger *log.Logger
	// Now et FirstDelay sont remplaçables pour les tests.
	Now        func() time.Time
	FirstDelay time.Duration
}

// Run vérifie une première fois après FirstDelay, puis une fois par jour, jusqu'à l'arrêt du service.
func (a *Auto) Run(ctx context.Context) {
	wait := a.FirstDelay
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = a.Once(ctx)
	}
}

// Once fait une recherche (et, le cas échéant, propose la mise à jour) ; renvoie le délai avant la
// suivante.
func (a *Auto) Once(ctx context.Context) time.Duration {
	now := a.now()
	st := a.load()
	if wait := st.RemindAfter.Sub(now); wait > 0 {
		return min(wait, Interval)
	}
	offer, err := Check(ctx, a.Source, a.Current)
	switch {
	case errors.Is(err, update.ErrNotPublished):
		return Interval
	case err != nil:
		a.logf("mise à jour : recherche impossible : %v", err)
		return 6 * time.Hour
	case offer == nil:
		return Interval
	}
	answer, err := a.Ask(ctx, *offer)
	if err != nil {
		a.logf("mise à jour : question impossible : %v", err)
	}
	switch answer {
	case NoUser:
		return NoUserRetry
	case Accept:
		a.logf("mise à jour : installation de la version %s acceptée", offer.Version)
		if err := a.Apply(ctx, *offer); err != nil {
			a.logf("mise à jour : échec : %v", err)
			a.save(state{RemindAfter: now.Add(RemindAfter), Version: offer.Version})
		}
		return Interval
	default:
		a.logf("mise à jour : version %s reportée", offer.Version)
		a.save(state{RemindAfter: now.Add(RemindAfter), Version: offer.Version})
		return RemindAfter
	}
}

func (a *Auto) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

func (a *Auto) load() state {
	var st state
	if data, err := os.ReadFile(a.StatePath); err == nil {
		_ = json.Unmarshal(data, &st)
	}
	return st
}

func (a *Auto) save(st state) {
	data, _ := json.Marshal(st)
	if err := os.WriteFile(a.StatePath, data, 0o600); err != nil {
		a.logf("mise à jour : %v", err)
	}
}

func (a *Auto) logf(format string, args ...any) {
	if a.Logger != nil {
		a.Logger.Printf(format, args...)
	}
}

// Finish termine une mise à jour, une fois exe remplacé : redémarre le service et vérifie qu'il
// répond ; sinon remet l'ancienne version en place et la redémarre. En cas de succès, l'ancienne
// version (« .old ») reste à supprimer par l'appelant.
func Finish(exe string, restart func() error, healthy func() bool) error {
	err := restart()
	if err == nil && healthy() {
		return nil
	}
	if err == nil {
		err = errors.New("le service ne répond pas")
	}
	if rbErr := Restore(exe); rbErr != nil {
		return fmt.Errorf("%w ; retour à la version précédente impossible : %v", err, rbErr)
	}
	if rsErr := restart(); rsErr != nil {
		return fmt.Errorf("%w ; version précédente remise en place mais non redémarrée : %v", err, rsErr)
	}
	return fmt.Errorf("%w : version précédente remise en place", err)
}

// Restore remet l'ancienne version (« .old ») à la place de la nouvelle, gardée en « .failed ».
func Restore(exe string) error {
	old := exe + ".old"
	if _, err := os.Stat(old); err != nil {
		return err
	}
	failed := exe + ".failed"
	_ = os.Remove(failed)
	if err := os.Rename(exe, failed); err != nil {
		return err
	}
	return os.Rename(old, exe)
}

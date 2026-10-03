package backup

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/slaynaw/wakeonlan/desktop/internal/dpapi"
)

// Settings sont les réglages des sauvegardes de cet appareil. Le mot de passe et le jeton GitHub sont
// des secrets : le fichier est chiffré par DPAPI (compte Windows), comme la configuration.
type Settings struct {
	Version int `json:"version"`
	// Enabled : sauvegardes automatiques actives (Password renseigné).
	Enabled  bool   `json:"enabled"`
	Password string `json:"password,omitempty"`
	// Device : identifiant de cet appareil dans les noms de fichiers (DeviceID).
	Device string `json:"device,omitempty"`
	// GitHub : compte connecté (jeton « gist ») et Gist des sauvegardes.
	GitHub *GitHub `json:"github,omitempty"`
	// Folder : dossier choisi ("" : aucun).
	Folder string `json:"folder,omitempty"`
	// LastGitHub et LastFolder : dernière sauvegarde réussie (Unix ms).
	LastGitHub int64 `json:"lastGithub,omitempty"`
	LastFolder int64 `json:"lastFolder,omitempty"`
	// Uploaded : fichiers de cet appareil présents dans le Gist (pour ne garder que Keep versions).
	Uploaded []string `json:"uploaded,omitempty"`
	// Archive : archives chiffrées des mesures et du journal des PC (même compte et même mot de passe).
	Archive *ArchiveSettings `json:"archive,omitempty"`
}

// ArchiveSettings sont les réglages des archives de cet appareil (docs/ARCHIVES.md).
type ArchiveSettings struct {
	Enabled bool `json:"enabled"`
	// Synced : par PC (adresse MAC), début de la dernière minute archivée (secondes Unix).
	Synced map[string]int64 `json:"synced,omitempty"`
	// Gists : Gist d'archives de chaque mois (« 2026-10 ») utilisé par cet appareil.
	Gists map[string]string `json:"gists,omitempty"`
	// Last : dernier archivage réussi (Unix ms).
	Last int64 `json:"last,omitempty"`
}

// ResetProgress oublie l'avancement (autre compte ou autre mot de passe) : tout ce que les agents
// gardent encore sera archivé de nouveau, dans des Gists que le nouveau mot de passe ouvre.
func (a *ArchiveSettings) ResetProgress() {
	if a != nil {
		a.Synced, a.Gists, a.Last = nil, nil, 0
	}
}

// GitHub est le compte GitHub des sauvegardes.
type GitHub struct {
	Token string `json:"token"`
	User  string `json:"user"`
	Gist  string `json:"gist,omitempty"`
}

const settingsVersion = 1

var entropy = []byte("patronus-desktop-backup/1")

// Store enregistre les réglages (backup.dat, chiffré par DPAPI).
type Store struct {
	path string
}

// NewStore crée le magasin du dossier dir.
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Store{path: filepath.Join(dir, "backup.dat")}, nil
}

// Load lit les réglages. Un fichier illisible est mis de côté et des réglages vides sont renvoyés
// avec l'erreur (les sauvegardes déjà faites ne sont pas touchées).
func (s *Store) Load() (Settings, error) {
	sealed, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return Settings{Version: settingsVersion}, nil
	}
	if err != nil {
		return Settings{Version: settingsVersion}, err
	}
	plain, err := dpapi.Unprotect(sealed, entropy)
	var st Settings
	if err == nil {
		err = json.Unmarshal(plain, &st)
		clear(plain)
	}
	if err == nil && st.Version != settingsVersion {
		err = fmt.Errorf("version %d inconnue", st.Version)
	}
	if err != nil {
		_ = os.Rename(s.path, s.path+".illisible-"+time.Now().Format("20060102-150405"))
		return Settings{Version: settingsVersion}, fmt.Errorf("réglages des sauvegardes illisibles, mis de côté (%v)", err)
	}
	return st, nil
}

// Save enregistre les réglages de façon atomique.
func (s *Store) Save(st Settings) error {
	st.Version = settingsVersion
	plain, err := json.Marshal(st)
	if err != nil {
		return err
	}
	sealed, err := dpapi.Protect(plain, entropy)
	clear(plain)
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

// WriteFolder écrit la sauvegarde name dans dir (remplace celle du jour) puis supprime les versions de
// l'appareil au-delà de Keep. Renvoie le chemin écrit.
func WriteFolder(dir, device, name string, content []byte) (string, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return "", fmt.Errorf("dossier inaccessible : %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("le chemin choisi n'est pas un dossier")
	}
	path := filepath.Join(dir, name)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, content, 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return path, nil
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	for _, old := range Outdated(names, device, Keep) {
		_ = os.Remove(filepath.Join(dir, old))
	}
	return path, nil
}

package share

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Store enregistre l'état du partage, chiffré pour l'utilisateur Windows (DPAPI) comme la configuration :
// il contient des clés privées et le jeton GitHub.
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

// Load lit l'état. Un fichier illisible est mis de côté (jamais supprimé) et un état vide est renvoyé.
func (s *Store) Load() (State, error) {
	sealed, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return NewState(), nil
	}
	if err != nil {
		return NewState(), err
	}
	plain, err := unprotect(sealed)
	var st State
	if err == nil {
		err = json.Unmarshal(plain, &st)
		clear(plain)
	}
	if err == nil && st.Version != stateVersion {
		err = fmt.Errorf("version %d inconnue", st.Version)
	}
	if err != nil {
		backup := s.path + ".illisible-" + time.Now().Format("20060102-150405")
		_ = os.Rename(s.path, backup)
		return NewState(), fmt.Errorf("partage illisible, mis de côté (%v)", err)
	}
	if st.Received == nil {
		st.Received = []Access{}
	}
	return st, nil
}

// Save enregistre l'état de façon atomique.
func (s *Store) Save(st State) error {
	st.Version = stateVersion
	plain, err := json.Marshal(st)
	if err != nil {
		return err
	}
	sealed, err := protect(plain)
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

// Package identity garde l'identité wolagent/2 de cette installation de l'application
// (docs/PROTOCOLE-V2.md, §2) : une clé ECDSA P-256 créée sur ce PC, chiffrée par DPAPI pour
// l'utilisateur Windows (illisible depuis un autre compte ou un autre PC). Elle ne figure dans aucune
// sauvegarde ni aucun partage : une nouvelle installation a une nouvelle identité.
package identity

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol2"
	"github.com/slaynaw/wakeonlan/desktop/internal/dpapi"
)

// FileName est le nom du fichier de la clé, dans le dossier des données de l'application.
const FileName = "identity.dat"

// entropy est propre à ce fichier : un autre programme de l'utilisateur ne peut pas le déchiffrer
// par un simple appel à CryptUnprotectData.
var entropy = []byte("patronus-desktop-identity/1")

// Identity est l'identité de cet appareil : le certificat qui présente sa clé dans TLS, et son
// empreinte.
type Identity struct {
	Cert tls.Certificate
	Pin  protocol2.Pin
}

// Store lit l'identité, ou la crée au premier besoin.
type Store struct {
	path string
	now  func() time.Time
	// warn note un incident (fichier illisible mis de côté), sans aucun secret.
	warn func(format string, args ...any)

	mu sync.Mutex
	id *Identity
}

// NewStore prépare l'identité rangée dans le dossier dir. warn peut être nil.
func NewStore(dir string, now func() time.Time, warn func(format string, args ...any)) *Store {
	if now == nil {
		now = time.Now
	}
	if warn == nil {
		warn = func(string, ...any) {}
	}
	return &Store{path: filepath.Join(dir, FileName), now: now, warn: warn}
}

// Path renvoie l'emplacement du fichier de la clé.
func (s *Store) Path() string { return s.path }

// Get renvoie l'identité, créée au premier appel si elle n'existe pas encore. Un fichier illisible
// (copié depuis un autre compte Windows, abîmé) est mis de côté, jamais supprimé, et remplacé par
// une nouvelle identité : les PC devront alors reconnaître à nouveau cet ordinateur.
func (s *Store) Get() (*Identity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.id != nil {
		return s.id, nil
	}
	id, err := s.load()
	switch {
	case err == nil:
	case errors.Is(err, fs.ErrNotExist):
		if id, err = s.create(); err != nil {
			return nil, err
		}
	default:
		aside := s.path + ".illisible-" + s.now().Format("20060102-150405")
		if renameErr := os.Rename(s.path, aside); renameErr != nil {
			return nil, fmt.Errorf("identité de cet ordinateur illisible (%v)", err)
		}
		s.warn("identité wolagent/2 illisible (%v) : mise de côté dans %s, nouvelle identité créée", err, filepath.Base(aside))
		if id, err = s.create(); err != nil {
			return nil, err
		}
	}
	s.id = id
	return id, nil
}

// Existing renvoie l'identité si elle existe et se lit, sans jamais la créer ni la remplacer (nil
// sinon) : pour le rapport de diagnostic.
func (s *Store) Existing() *Identity {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.id == nil {
		s.id, _ = s.load()
	}
	return s.id
}

func (s *Store) load() (*Identity, error) {
	sealed, err := os.ReadFile(s.path)
	if err != nil {
		return nil, err
	}
	der, err := dpapi.Unprotect(sealed, entropy)
	if err != nil {
		return nil, errors.New("déchiffrement impossible (fichier d'un autre compte Windows ?)")
	}
	defer clear(der)
	parsed, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, errors.New("clé illisible")
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok || key.Curve != elliptic.P256() {
		return nil, errors.New("clé autre que ECDSA P-256")
	}
	return s.fromKey(key)
}

func (s *Store) create() (*Identity, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	sealed, err := dpapi.Protect(der, entropy)
	clear(der)
	if err != nil {
		return nil, fmt.Errorf("protection de l'identité de cet ordinateur : %w", err)
	}
	if err := writeFile(s.path, sealed); err != nil {
		return nil, fmt.Errorf("enregistrement de l'identité de cet ordinateur : %w", err)
	}
	return s.fromKey(key)
}

// fromKey prépare le certificat autosigné (recréé à chaque lancement : l'empreinte ne dépend que de
// la clé).
func (s *Store) fromKey(key *ecdsa.PrivateKey) (*Identity, error) {
	der, err := protocol2.NewCertificate(key, s.now())
	if err != nil {
		return nil, err
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return &Identity{
		Cert: tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf},
		Pin:  protocol2.PinOf(leaf.RawSubjectPublicKeyInfo),
	}, nil
}

// writeFile écrit de façon atomique un fichier lisible par son seul propriétaire.
func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

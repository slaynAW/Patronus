// Package identity garde la clé d'identité wolagent/2 de l'agent (docs/PROTOCOLE-V2.md, §2) : une clé
// ECDSA P-256 créée sur le PC, qui n'en sort jamais, rangée dans le dossier de configuration (réservé à
// root, ou à SYSTEM et aux Administrateurs) et chiffrée en plus par DPAPI (machine) sous Windows.
package identity

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol2"
)

// FileName est le nom du fichier de la clé, dans le dossier de configuration.
const FileName = "identity.key"

// Identity est l'identité de l'agent : sa clé, le certificat qui la présente dans TLS et son empreinte.
type Identity struct {
	Key  *ecdsa.PrivateKey
	Cert tls.Certificate
	Pin  protocol2.Pin
}

// Path renvoie l'emplacement de la clé dans le dossier dir.
func Path(dir string) string { return filepath.Join(dir, FileName) }

// Load lit l'identité ; l'erreur vérifie errors.Is(err, os.ErrNotExist) si elle n'existe pas encore.
func Load(dir string, now time.Time) (*Identity, error) {
	data, err := os.ReadFile(Path(dir))
	if err != nil {
		return nil, err
	}
	der, err := unseal(data)
	if err != nil {
		return nil, fmt.Errorf("clé d'identité illisible : %w", err)
	}
	parsed, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, fmt.Errorf("clé d'identité illisible : %w", err)
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok || key.Curve != elliptic.P256() {
		return nil, errors.New("clé d'identité illisible : clé autre que ECDSA P-256")
	}
	return fromKey(key, now)
}

// LoadOrCreate lit l'identité, ou la crée au premier lancement ; created indique une création.
func LoadOrCreate(dir string, now time.Time) (id *Identity, created bool, err error) {
	id, err = Load(dir, now)
	if !errors.Is(err, os.ErrNotExist) {
		return id, false, err
	}
	id, err = create(dir, now)
	return id, err == nil, err
}

// Rotate remplace la clé (wol-agent rotate-identity) : les appareils devront épingler la nouvelle.
func Rotate(dir string, now time.Time) (*Identity, error) { return create(dir, now) }

func create(dir string, now time.Time) (*Identity, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	sealed, err := seal(der)
	if err != nil {
		return nil, fmt.Errorf("protection de la clé d'identité : %w", err)
	}
	if err := writeFile(Path(dir), sealed); err != nil {
		return nil, fmt.Errorf("enregistrement de la clé d'identité : %w", err)
	}
	return fromKey(key, now)
}

// fromKey prépare le certificat autosigné (recréé à chaque démarrage : l'empreinte ne dépend que de la
// clé).
func fromKey(key *ecdsa.PrivateKey, now time.Time) (*Identity, error) {
	der, err := protocol2.NewCertificate(key, now)
	if err != nil {
		return nil, err
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return &Identity{
		Key:  key,
		Cert: tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf},
		Pin:  protocol2.PinOf(leaf.RawSubjectPublicKeyInfo),
	}, nil
}

// writeFile écrit de façon atomique un fichier lisible par son seul propriétaire (le dossier, réservé
// aux administrateurs sous Windows, transmet ses droits).
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

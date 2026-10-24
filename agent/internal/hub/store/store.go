// Package store garde les données du hub chiffrées au repos (docs/HUB.md, §9) : une clé des données tirée
// au hasard, rangée hors du dossier de données, dont est dérivée une clé propre à chaque fichier
// (HKDF-SHA256). Deux formes de fichier :
//
//   - fichier entier (hub.db) : réécrit à chaque changement, d'un bloc (fichier temporaire puis renommage) ;
//   - journal (relevés d'un PC sur un mois) : ajout seul, un enregistrement chiffré à la fois. Un
//     enregistrement tronqué en fin de fichier (coupure de courant) est ignoré, puis remplacé au prochain
//     ajout ; un enregistrement modifié, retiré ou déplacé rend le journal illisible.
//
// Format exact et vecteurs communs : protocol/hub-store-vectors.json, recalculés indépendamment par
// .github/scripts/check-hub-store-vectors.py.
package store

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

const (
	// KeyFileName est le nom de la clé des données, dans le dossier des clés.
	KeyFileName = "data.key"
	// KeySize est la taille de la clé des données et des clés de fichier (AES-256).
	KeySize = 32
	// NonceSize et TagSize : AES-GCM, nonce de 96 bits tiré au hasard, étiquette de 128 bits.
	NonceSize = 12
	TagSize   = 16
	// MaxFileBytes borne un fichier entier déchiffré ; MaxRecordBytes, un enregistrement de journal.
	MaxFileBytes   = 64 << 20
	MaxRecordBytes = 1 << 20
	// maxJournalBytes borne la lecture d'un journal (un mois de relevés en fait quelques Mo).
	maxJournalBytes = 256 << 20
	maxPathLength   = 200

	keyPEMType = "PATRONUS HUB DATA KEY"
	// fileKeyInfo sépare la dérivation des clés de fichier de tout autre usage de la clé des données.
	fileKeyInfo = "patronus-hub/1 file key"
)

var (
	// FileMagic et JournalMagic ouvrent chaque fichier, et entrent dans les données authentifiées : un
	// fichier entier ne peut pas passer pour un journal, ni l'inverse.
	FileMagic    = []byte("PHF1")
	JournalMagic = []byte("PHJ1")

	// ErrCorrupted : déchiffrement impossible (autre clé, fichier modifié, déplacé ou abîmé).
	ErrCorrupted = errors.New("données du hub illisibles : autre clé, ou fichier modifié ou abîmé")
	// ErrBadPath : chemin hors du format permis (relatif, sans « .. », lettres, chiffres, « . », « _ », « - »).
	ErrBadPath = errors.New("chemin de fichier du hub invalide")
	// ErrTooLarge : fichier ou enregistrement au-delà des limites.
	ErrTooLarge = errors.New("données du hub trop volumineuses")

	pathPattern = regexp.MustCompile(`^[A-Za-z0-9_-][A-Za-z0-9._-]*(/[A-Za-z0-9_-][A-Za-z0-9._-]*)*$`)
)

// Key est la clé des données du hub.
type Key [KeySize]byte

// KeyPath renvoie l'emplacement de la clé des données dans le dossier des clés.
func KeyPath(dir string) string { return filepath.Join(dir, KeyFileName) }

// LoadOrCreateKey lit la clé des données, ou la crée au premier démarrage (created). Une clé illisible est
// une erreur, jamais remplacée : une nouvelle clé rendrait toutes les données illisibles.
func LoadOrCreateKey(dir string) (key Key, created bool, err error) {
	raw, err := os.ReadFile(KeyPath(dir))
	switch {
	case errors.Is(err, os.ErrNotExist):
		if _, err := io.ReadFull(rand.Reader, key[:]); err != nil {
			return Key{}, false, err
		}
		block := pem.EncodeToMemory(&pem.Block{Type: keyPEMType, Bytes: key[:]})
		if err := writeAtomic(KeyPath(dir), block); err != nil {
			return Key{}, false, fmt.Errorf("enregistrement de la clé des données : %w", err)
		}
		return key, true, nil
	case err != nil:
		return Key{}, false, err
	}
	block, rest := pem.Decode(raw)
	if block == nil || block.Type != keyPEMType || len(block.Bytes) != KeySize || len(bytes.TrimSpace(rest)) != 0 {
		return Key{}, false, fmt.Errorf("clé des données illisible (%s)", KeyPath(dir))
	}
	copy(key[:], block.Bytes)
	return key, false, nil
}

// FileKey dérive la clé d'un fichier de son chemin relatif (HKDF-SHA256, sans sel).
func (k Key) FileKey(rel string) []byte {
	out, err := hkdf.Key(sha256.New, k[:], nil, fileKeyInfo+"\x00"+rel, KeySize)
	if err != nil {
		panic(err) // taille fixe, toujours permise
	}
	return out
}

func (k Key) aead(rel string) cipher.AEAD {
	block, err := aes.NewCipher(k.FileKey(rel))
	if err != nil {
		panic(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		panic(err)
	}
	return gcm
}

// ValidPath vérifie un chemin relatif de fichier du hub.
func ValidPath(rel string) bool {
	return len(rel) <= maxPathLength && pathPattern.MatchString(rel) && !strings.HasSuffix(rel, ".tmp")
}

// fileAAD : en-tête puis chemin.
func fileAAD(rel string) []byte { return append(bytes.Clone(FileMagic), rel...) }

// recordAAD : en-tête, rang de l'enregistrement (8 octets, gros-boutiste), puis chemin.
func recordAAD(rel string, index uint64) []byte {
	aad := binary.BigEndian.AppendUint64(bytes.Clone(JournalMagic), index)
	return append(aad, rel...)
}

// SealFile chiffre un fichier entier : en-tête, nonce, puis texte chiffré et étiquette.
func (k Key) SealFile(rel string, plain, nonce []byte) []byte {
	out := append(bytes.Clone(FileMagic), nonce...)
	return k.aead(rel).Seal(out, nonce, plain, fileAAD(rel))
}

// OpenFile déchiffre un fichier entier.
func (k Key) OpenFile(rel string, data []byte) ([]byte, error) {
	if len(data) < len(FileMagic)+NonceSize+TagSize || !bytes.Equal(data[:len(FileMagic)], FileMagic) {
		return nil, ErrCorrupted
	}
	nonce := data[len(FileMagic) : len(FileMagic)+NonceSize]
	plain, err := k.aead(rel).Open(nil, nonce, data[len(FileMagic)+NonceSize:], fileAAD(rel))
	if err != nil {
		return nil, ErrCorrupted
	}
	return plain, nil
}

// SealRecord chiffre l'enregistrement de rang index d'un journal : longueur (4 octets, gros-boutiste) du
// reste, nonce, puis texte chiffré et étiquette.
func (k Key) SealRecord(rel string, index uint64, plain, nonce []byte) []byte {
	out := binary.BigEndian.AppendUint32(nil, uint32(NonceSize+len(plain)+TagSize))
	out = append(out, nonce...)
	return k.aead(rel).Seal(out, nonce, plain, recordAAD(rel, index))
}

// OpenJournal déchiffre un journal entier. Un enregistrement incomplet en fin de fichier est ignoré :
// end donne la longueur de la partie complète (data[:end]).
func (k Key) OpenJournal(rel string, data []byte) (records [][]byte, end int, err error) {
	if len(data) < len(JournalMagic) {
		// Coupure pendant la création : journal vide, réécrit au prochain ajout.
		if !bytes.HasPrefix(JournalMagic, data) {
			return nil, 0, ErrCorrupted
		}
		return nil, 0, nil
	}
	if !bytes.Equal(data[:len(JournalMagic)], JournalMagic) {
		return nil, 0, ErrCorrupted
	}
	gcm := k.aead(rel)
	pos := len(JournalMagic)
	for index := uint64(0); ; index++ {
		if len(data)-pos < 4 {
			return records, pos, nil
		}
		n := int(binary.BigEndian.Uint32(data[pos:]))
		if n < NonceSize+TagSize || n > NonceSize+MaxRecordBytes+TagSize {
			return nil, 0, ErrCorrupted
		}
		if len(data)-pos-4 < n {
			return records, pos, nil
		}
		body := data[pos+4 : pos+4+n]
		plain, err := gcm.Open(nil, body[:NonceSize], body[NonceSize:], recordAAD(rel, index))
		if err != nil {
			return nil, 0, ErrCorrupted
		}
		records = append(records, plain)
		pos += 4 + n
	}
}

// Store est le dossier des données du hub, sûr pour un usage concurrent.
type Store struct {
	dir  string
	key  Key
	rand io.Reader

	mu     sync.Mutex
	counts map[string]uint64 // nombre d'enregistrements des journaux déjà ouverts en écriture
}

// Open ouvre le dossier des données (créé au besoin, réservé à son propriétaire).
func Open(dir string, key Key) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Store{dir: dir, key: key, rand: rand.Reader, counts: map[string]uint64{}}, nil
}

func (s *Store) path(rel string) (string, error) {
	if !ValidPath(rel) {
		return "", ErrBadPath
	}
	return filepath.Join(s.dir, filepath.FromSlash(rel)), nil
}

func (s *Store) nonce() ([]byte, error) {
	nonce := make([]byte, NonceSize)
	_, err := io.ReadFull(s.rand, nonce)
	return nonce, err
}

// ReadFile lit un fichier entier ; l'erreur vérifie errors.Is(err, os.ErrNotExist) s'il n'existe pas.
func (s *Store) ReadFile(rel string) ([]byte, error) {
	p, err := s.path(rel)
	if err != nil {
		return nil, err
	}
	data, err := readLimited(p, len(FileMagic)+NonceSize+MaxFileBytes+TagSize)
	if err != nil {
		return nil, err
	}
	return s.key.OpenFile(rel, data)
}

// WriteFile remplace un fichier entier, d'un bloc : après une coupure, il reste l'ancien ou le nouveau.
func (s *Store) WriteFile(rel string, plain []byte) error {
	if len(plain) > MaxFileBytes {
		return ErrTooLarge
	}
	p, err := s.path(rel)
	if err != nil {
		return err
	}
	nonce, err := s.nonce()
	if err != nil {
		return err
	}
	s.mu.Lock() // un seul fichier temporaire à la fois
	defer s.mu.Unlock()
	return writeAtomic(p, s.key.SealFile(rel, plain, nonce))
}

// Records lit tous les enregistrements d'un journal (aucun s'il n'existe pas).
func (s *Store) Records(rel string) ([][]byte, error) {
	p, err := s.path(rel)
	if err != nil {
		return nil, err
	}
	data, err := readLimited(p, maxJournalBytes)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	records, _, err := s.key.OpenJournal(rel, data)
	return records, err
}

// Append ajoute des enregistrements à la fin d'un journal, créé au besoin.
func (s *Store) Append(rel string, records ...[]byte) error {
	p, err := s.path(rel)
	if err != nil {
		return err
	}
	for _, r := range records {
		if len(r) > MaxRecordBytes {
			return ErrTooLarge
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	count, known := s.counts[rel]
	if !known {
		if count, err = s.prepare(p, rel); err != nil {
			return err
		}
	}
	var buf []byte
	for i, r := range records {
		nonce, err := s.nonce()
		if err != nil {
			return err
		}
		buf = append(buf, s.key.SealRecord(rel, count+uint64(i), r, nonce)...)
	}
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(buf)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		delete(s.counts, rel) // état du fichier incertain : relu au prochain ajout
		return err
	}
	s.counts[rel] = count + uint64(len(records))
	return nil
}

// prepare ouvre un journal en écriture pour la première fois : le crée, ou le relit pour compter ses
// enregistrements et retirer un enregistrement incomplet laissé par une coupure.
func (s *Store) prepare(p, rel string) (uint64, error) {
	data, err := readLimited(p, maxJournalBytes)
	if errors.Is(err, os.ErrNotExist) || err == nil && len(data) < len(JournalMagic) && bytes.HasPrefix(JournalMagic, data) {
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			return 0, err
		}
		return 0, writeAtomic(p, JournalMagic)
	}
	if err != nil {
		return 0, err
	}
	records, end, err := s.key.OpenJournal(rel, data)
	if err != nil {
		return 0, err
	}
	if end < len(data) {
		if err := os.Truncate(p, int64(end)); err != nil {
			return 0, err
		}
	}
	return uint64(len(records)), nil
}

// Remove supprime un fichier ou un journal (sans erreur s'il n'existe pas).
func (s *Store) Remove(rel string) error {
	p, err := s.path(rel)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.counts, rel)
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// List renvoie les noms des fichiers d'un sous-dossier ("" : la racine), triés.
func (s *Store) List(dirRel string) ([]string, error) { return s.list(dirRel, false) }

// Dirs renvoie les noms des sous-dossiers d'un dossier ("" : la racine), triés.
func (s *Store) Dirs(dirRel string) ([]string, error) { return s.list(dirRel, true) }

func (s *Store) list(dirRel string, dirs bool) ([]string, error) {
	dir := s.dir
	if dirRel != "" {
		p, err := s.path(dirRel)
		if err != nil {
			return nil, err
		}
		dir = p
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if (dirs && e.IsDir() || !dirs && e.Type().IsRegular()) && ValidPath(e.Name()) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// readLimited lit un fichier de max octets au plus.
func readLimited(p string, max int) ([]byte, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, int64(max)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > max {
		return nil, ErrTooLarge
	}
	return data, nil
}

// writeAtomic écrit un fichier lisible par son seul propriétaire, d'un bloc et sur le disque.
func writeAtomic(p string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	tmp := p + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, p)
	}
	if err != nil {
		_ = os.Remove(tmp)
	}
	return err
}

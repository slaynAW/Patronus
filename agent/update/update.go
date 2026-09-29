// Package update recherche, télécharge et installe les nouvelles versions de l'application Windows
// et de l'agent (mêmes règles que l'application Android : core/update).
//
// Chaque version officielle publiée par la CI contient un manifeste « update.json » (numéro,
// nouveautés, taille et empreinte SHA-256 de chaque fichier) signé avec la clé de signature de
// l'APK (« update.json.sig », RSA PKCS#1 v1.5 / SHA-256, en base64). Le manifeste n'est accepté
// que si la signature est valide, puis chaque fichier téléchargé doit avoir exactement la taille et
// l'empreinte annoncées : un fichier modifié ou incomplet n'est jamais installé.
//
// Le manifeste décrit aussi l'agent joint à la version (section « agent ») : il a son propre numéro,
// qui ne change que lorsque l'agent change.
package update

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultBase est l'adresse des versions publiées (GitHub Releases du dépôt public).
	DefaultBase = "https://github.com/slaynAW/WakeOnLan/releases"
	// ManifestName est le nom du manifeste joint à chaque version.
	ManifestName = "update.json"
	// Format est la version du format de manifeste comprise par l'application.
	Format = 1
	// MaxFileSize borne la taille d'un fichier de mise à jour.
	MaxFileSize = 100 << 20
	maxManifest = 64 << 10
)

// releaseCert est le certificat de la clé de signature de l'APK (public) : il vérifie les manifestes.
//
//go:embed release-cert.pem
var releaseCert []byte

var (
	versionPattern = regexp.MustCompile(`^[0-9]{1,4}\.[0-9]{1,4}\.[0-9]{1,4}$`)
	namePattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	datePattern    = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
	sha256Pattern  = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// File est un fichier de la version, pour une plateforme (« android », « windows-amd64 »…).
type File struct {
	Platform string `json:"platform"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	SHA256   string `json:"sha256"`
}

// Manifest décrit une version publiée.
type Manifest struct {
	Format  int    `json:"format"`
	Version string `json:"version"`
	// Code croît à chaque build (numéro d'exécution de la CI) : c'est lui qui est comparé.
	Code  int64  `json:"code"`
	Date  string `json:"date"`
	Notes string `json:"notes"`
	Files []File `json:"files"`
	// Agent : agent joint à la version (absent des manifestes antérieurs à l'agent 1.4.0).
	Agent *Agent `json:"agent,omitempty"`
}

// Agent décrit l'agent joint à une version. Ses fichiers se téléchargent avec ceux de la version
// (FileURL), mais son numéro lui est propre : c'est lui qui est comparé (Newer).
type Agent struct {
	Version string `json:"version"`
	Notes   string `json:"notes"`
	Files   []File `json:"files"`
}

// ErrSignature signale un manifeste dont la signature est absente ou invalide.
var ErrSignature = errors.New("signature de la mise à jour invalide")

// ReleaseKey renvoie la clé publique de signature des versions officielles.
func ReleaseKey() (*rsa.PublicKey, error) {
	block, _ := pem.Decode(releaseCert)
	if block == nil {
		return nil, errors.New("certificat de mise à jour absent")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("certificat de mise à jour : clé RSA attendue")
	}
	return key, nil
}

// ParsePublicKey lit une clé publique RSA (SubjectPublicKeyInfo DER, en base64).
func ParsePublicKey(b64 string) (*rsa.PublicKey, error) {
	der, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, err
	}
	key, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, err
	}
	rsaKey, ok := key.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("clé RSA attendue")
	}
	return rsaKey, nil
}

// Verify vérifie la signature (base64) du manifeste brut.
func Verify(key *rsa.PublicKey, manifest []byte, signature string) error {
	sig, err := base64.StdEncoding.DecodeString(string(bytes.TrimSpace([]byte(signature))))
	if err != nil || key == nil {
		return ErrSignature
	}
	digest := sha256.Sum256(manifest)
	if rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], sig) != nil {
		return ErrSignature
	}
	return nil
}

// Parse lit et contrôle un manifeste (déjà authentifié par Verify).
func Parse(data []byte) (Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return m, fmt.Errorf("manifeste illisible : %w", err)
	}
	switch {
	case m.Format != Format:
		return m, fmt.Errorf("format de manifeste %d non pris en charge", m.Format)
	case !versionPattern.MatchString(m.Version):
		return m, fmt.Errorf("numéro de version invalide : %q", m.Version)
	case m.Code <= 0:
		return m, errors.New("code de version invalide")
	case m.Date != "" && !datePattern.MatchString(m.Date):
		return m, errors.New("date invalide")
	case len(m.Notes) > 32<<10:
		return m, errors.New("nouveautés trop longues")
	}
	if err := checkFiles(m.Files); err != nil {
		return m, err
	}
	if a := m.Agent; a != nil {
		switch {
		case !versionPattern.MatchString(a.Version):
			return m, fmt.Errorf("numéro de version de l'agent invalide : %q", a.Version)
		case len(a.Notes) > 8<<10:
			return m, errors.New("nouveautés de l'agent trop longues")
		}
		if err := checkFiles(a.Files); err != nil {
			return m, err
		}
	}
	return m, nil
}

func checkFiles(files []File) error {
	seen := map[string]bool{}
	for _, f := range files {
		switch {
		case f.Platform == "" || seen[f.Platform]:
			return fmt.Errorf("plateforme absente ou en double : %q", f.Platform)
		case !namePattern.MatchString(f.Name):
			return fmt.Errorf("nom de fichier invalide : %q", f.Name)
		case f.Size <= 0 || f.Size > MaxFileSize:
			return fmt.Errorf("taille invalide pour %s", f.Name)
		case !sha256Pattern.MatchString(f.SHA256):
			return fmt.Errorf("empreinte invalide pour %s", f.Name)
		}
		seen[f.Platform] = true
	}
	return nil
}

// File renvoie le fichier destiné à une plateforme.
func (m Manifest) File(platform string) (File, bool) { return findFile(m.Files, platform) }

// File renvoie le fichier de l'agent destiné à une plateforme (« windows-amd64 »…).
func (a Agent) File(platform string) (File, bool) { return findFile(a.Files, platform) }

func findFile(files []File, platform string) (File, bool) {
	for _, f := range files {
		if f.Platform == platform {
			return f, true
		}
	}
	return File{}, false
}

// Newer indique si la version candidate (« X.Y.Z ») est plus récente que current. Une pré-version
// (« 1.4.0-dev.12 ») précède la version « 1.4.0 » ; une version non numérotée (« dev », compilation
// locale) n'est jamais mise à jour.
func Newer(candidate, current string) bool {
	c, cPre, ok := parseVersion(candidate)
	n, nPre, ok2 := parseVersion(current)
	if !ok || !ok2 || cPre {
		return false
	}
	for i := range c {
		if c[i] != n[i] {
			return c[i] > n[i]
		}
	}
	return nPre
}

func parseVersion(v string) (nums [3]int, pre bool, ok bool) {
	base, suffix, hasSuffix := strings.Cut(v, "-")
	if !versionPattern.MatchString(base) || (hasSuffix && suffix == "") {
		return nums, false, false
	}
	for i, part := range strings.Split(base, ".") {
		nums[i], _ = strconv.Atoi(part)
	}
	return nums, hasSuffix, true
}

// Source est l'emplacement des versions publiées et la clé qui les authentifie.
type Source struct {
	// Base : « …/releases » (manifeste : Base/latest/download/update.json ; fichiers :
	// Base/download/vX.Y.Z/nom).
	Base   string
	Key    *rsa.PublicKey
	Client *http.Client
	// UserAgent identifie le programme auprès du serveur (« Patronus » par défaut).
	UserAgent string
}

// Source de test : remplacées uniquement pour le test de bout en bout de la CI (-ldflags -X), jamais
// dans les versions publiées. testKey est une clé publique RSA (SubjectPublicKeyInfo DER, base64).
var testBase, testKey string

// Official renvoie la source des versions officielles (GitHub, clé de signature de l'APK).
func Official() (Source, error) {
	if testBase != "" && testKey != "" {
		key, err := ParsePublicKey(testKey)
		return Source{Base: testBase, Key: key}, err
	}
	key, err := ReleaseKey()
	if err != nil {
		return Source{}, err
	}
	return Source{Base: DefaultBase, Key: key}, nil
}

func (s Source) userAgent() string {
	if s.UserAgent == "" {
		return "Patronus"
	}
	return s.UserAgent
}

func (s Source) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return &http.Client{Timeout: 10 * time.Minute}
}

// FileURL renvoie l'adresse de téléchargement d'un fichier de la version.
func (s Source) FileURL(m Manifest, f File) string {
	return s.Base + "/download/v" + m.Version + "/" + f.Name
}

// Latest renvoie le manifeste de la dernière version officielle, authentifié et contrôlé.
func (s Source) Latest(ctx context.Context) (Manifest, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	base := s.Base + "/latest/download/" + ManifestName
	data, err := s.get(ctx, base, maxManifest)
	if err != nil {
		return Manifest{}, err
	}
	sig, err := s.get(ctx, base+".sig", 4096)
	if err != nil {
		return Manifest{}, err
	}
	if err := Verify(s.Key, data, string(sig)); err != nil {
		return Manifest{}, err
	}
	return Parse(data)
}

// ErrNotPublished signale qu'aucune version publiée ne contient encore de manifeste.
var ErrNotPublished = errors.New("aucune mise à jour publiée")

func (s Source) get(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", s.userAgent())
	resp, err := s.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotPublished
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("réponse %d du serveur des mises à jour", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("réponse trop volumineuse")
	}
	return data, nil
}

// Download télécharge un fichier de la version dans dest (remplacé seulement s'il est complet et
// conforme à l'empreinte annoncée). progress reçoit le nombre d'octets reçus.
func (s Source) Download(ctx context.Context, m Manifest, f File, dest string, progress func(done, total int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.FileURL(m, f), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", s.userAgent())
	resp, err := s.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("téléchargement : réponse %d", resp.StatusCode)
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".update-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	hash := sha256.New()
	var done int64
	buf := make([]byte, 64<<10)
	body := io.LimitReader(resp.Body, f.Size+1)
	for {
		n, readErr := body.Read(buf)
		if n > 0 {
			if _, err := tmp.Write(buf[:n]); err != nil {
				tmp.Close()
				return err
			}
			hash.Write(buf[:n])
			done += int64(n)
			if progress != nil {
				progress(min(done, f.Size), f.Size)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			tmp.Close()
			return readErr
		}
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if done != f.Size || hex.EncodeToString(hash.Sum(nil)) != f.SHA256 {
		return errors.New("le fichier téléchargé ne correspond pas à la version publiée")
	}
	return os.Rename(tmp.Name(), dest)
}

// Replace remplace l'exécutable exe par next (sur le même volume). L'ancien est gardé à côté
// (« .old ») : Windows permet de renommer un programme en cours d'exécution, pas de l'effacer.
// En cas d'échec, l'exécutable d'origine est remis en place.
func Replace(exe, next string) error {
	old := exe + ".old"
	_ = os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		return fmt.Errorf("impossible de remplacer l'application (%w)", err)
	}
	if err := os.Rename(next, exe); err != nil {
		_ = os.Rename(old, exe)
		return fmt.Errorf("impossible d'installer la nouvelle version (%w)", err)
	}
	return nil
}

// CleanupOld supprime l'ancienne version laissée par Replace (une fois celle-ci fermée).
func CleanupOld(exe string) {
	old := exe + ".old"
	for range 20 {
		if err := os.Remove(old); err == nil || errors.Is(err, os.ErrNotExist) {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// Package archive implémente le format des archives chiffrées « patronus-archive/1 » (identique à
// core/archive côté Android) : mesures minute par minute et journal des démarrages et arrêts des PC,
// rangés sur GitHub dans un Gist secret par mois.
//
// Contenu d'un Gist (description « Patronus – archives chiffrées AAAA-MM ») :
//   - patronus-archive.json : manifeste en clair (sel et paramètres de dérivation, valeur de contrôle) ;
//   - LISEZMOI.md : explication ;
//   - mesures-AAAA-MM-JJ.txt : mesures d'un jour (UTC) de tous les PC ;
//   - journal-AAAA-MM.txt : journal du mois de tous les PC.
//
// Chaque fichier de données est du JSON compressé (gzip) puis chiffré en AES-256-GCM, nonce de 12
// octets en tête, le tout en Base64 ; la clé est dérivée du mot de passe des sauvegardes par
// PBKDF2-HMAC-SHA256 (600 000 itérations, sel aléatoire du mois) ; le nom du fichier est authentifié
// (données associées) : un fichier ne peut pas être remplacé par un autre. Seuls les dates
// apparaissent en clair (noms des fichiers).
package archive

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol"
)

const (
	Format     = "patronus-archive"
	Version    = 1
	KDF        = "PBKDF2WithHmacSHA256"
	Iterations = 600_000
	// MinIterations / MaxIterations bornent un manifeste lu (fichier modifiable par quiconque a le Gist).
	MinIterations = 100_000
	MaxIterations = 10_000_000
	// ManifestFile, ReadmeFile : fichiers en clair d'un Gist d'archives.
	ManifestFile = "patronus-archive.json"
	ReadmeFile   = "LISEZMOI.md"
	// DescriptionPrefix : début de la description des Gists d'archives (suivi du mois).
	DescriptionPrefix = "Patronus – archives chiffrées "
	// MaxFileBytes borne un fichier lu (texte Base64) ; MaxPlainBytes le JSON décompressé.
	MaxFileBytes  = 4 << 20
	MaxPlainBytes = 16 << 20
	checkName     = "check"
)

// ErrWrongPassword : le mot de passe ne correspond pas à ce mois d'archives.
var ErrWrongPassword = errors.New("mot de passe des archives incorrect")

var (
	monthPattern = regexp.MustCompile(`^\d{4}-\d{2}$`)
	dayFile      = regexp.MustCompile(`^mesures-(\d{4}-\d{2}-\d{2})\.txt$`)
)

// Description renvoie la description du Gist d'un mois (« 2026-10 »).
func Description(month string) string { return DescriptionPrefix + month }

// MonthOf renvoie le mois d'une description de Gist d'archives ("" si ce n'en est pas une).
func MonthOf(description string) string {
	month, ok := strings.CutPrefix(description, DescriptionPrefix)
	if !ok || !monthPattern.MatchString(month) {
		return ""
	}
	return month
}

// DayFile renvoie le nom du fichier des mesures d'un jour (« 2026-10-03 »).
func DayFile(day string) string { return "mesures-" + day + ".txt" }

// DayOf renvoie le jour d'un nom de fichier de mesures ("" si ce n'en est pas un).
func DayOf(name string) string {
	if m := dayFile.FindStringSubmatch(name); m != nil {
		return m[1]
	}
	return ""
}

// JournalFile renvoie le nom du fichier du journal d'un mois.
func JournalFile(month string) string { return "journal-" + month + ".txt" }

// Manifest est le manifeste d'un Gist d'archives.
type Manifest struct {
	Format     string `json:"format"`
	Version    int    `json:"version"`
	Month      string `json:"month"`
	KDF        string `json:"kdf"`
	Iterations int    `json:"iterations"`
	Salt       string `json:"salt"`
	// Check : valeur chiffrée connue, pour reconnaître le bon mot de passe.
	Check string `json:"check"`
}

// Key chiffre et déchiffre les fichiers d'un mois.
type Key struct {
	aead cipher.AEAD
}

// NewManifest crée le manifeste (sel aléatoire) et la clé d'un nouveau mois.
func NewManifest(month, password string) (Manifest, *Key, error) {
	if !monthPattern.MatchString(month) {
		return Manifest{}, nil, fmt.Errorf("mois invalide : %q", month)
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return Manifest{}, nil, err
	}
	key, err := deriveKey(password, salt, Iterations)
	if err != nil {
		return Manifest{}, nil, err
	}
	check, err := key.Seal(checkName, Format)
	if err != nil {
		return Manifest{}, nil, err
	}
	m := Manifest{Format: Format, Version: Version, Month: month, KDF: KDF, Iterations: Iterations,
		Salt: base64.StdEncoding.EncodeToString(salt), Check: check}
	return m, key, nil
}

// ParseManifest lit un manifeste et vérifie ses paramètres.
func ParseManifest(text string) (Manifest, error) {
	var m Manifest
	if err := json.Unmarshal([]byte(text), &m); err != nil {
		return Manifest{}, fmt.Errorf("manifeste illisible : %w", err)
	}
	switch {
	case m.Format != Format:
		return Manifest{}, errors.New("ce Gist n'est pas une archive Patronus")
	case m.Version != Version:
		return Manifest{}, fmt.Errorf("archive de version %d : mettez l'application à jour", m.Version)
	case m.KDF != KDF || m.Iterations < MinIterations || m.Iterations > MaxIterations:
		return Manifest{}, errors.New("paramètres de chiffrement non pris en charge")
	case !monthPattern.MatchString(m.Month):
		return Manifest{}, errors.New("mois invalide dans le manifeste")
	}
	return m, nil
}

// JSON renvoie le texte du manifeste.
func (m Manifest) JSON() string {
	raw, _ := json.MarshalIndent(m, "", "  ")
	return string(raw)
}

// Unlock vérifie le mot de passe et renvoie la clé du mois.
func (m Manifest) Unlock(password string) (*Key, error) {
	salt, err := base64.StdEncoding.DecodeString(m.Salt)
	if err != nil || len(salt) < 16 {
		return nil, errors.New("sel invalide dans le manifeste")
	}
	key, err := deriveKey(password, salt, m.Iterations)
	if err != nil {
		return nil, err
	}
	var check string
	if err := key.Open(checkName, m.Check, &check); err != nil || check != Format {
		return nil, ErrWrongPassword
	}
	return key, nil
}

func deriveKey(password string, salt []byte, iterations int) (*Key, error) {
	if password == "" {
		return nil, errors.New("mot de passe des sauvegardes requis")
	}
	raw, err := pbkdf2.Key(sha256.New, password, salt, iterations, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(raw)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Key{aead: aead}, nil
}

func aad(name string) []byte { return []byte(fmt.Sprintf("%s/%d:%s", Format, Version, name)) }

// Seal chiffre v (JSON, compressé) pour le fichier name.
func (k *Key) Seal(name string, v any) (string, error) {
	plain, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	var zipped bytes.Buffer
	zw := gzip.NewWriter(&zipped)
	if _, err := zw.Write(plain); err != nil {
		return "", err
	}
	if err := zw.Close(); err != nil {
		return "", err
	}
	nonce := make([]byte, k.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := k.aead.Seal(nonce, nonce, zipped.Bytes(), aad(name))
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Open déchiffre le fichier name dans v.
func (k *Key) Open(name, text string, v any) error {
	if len(text) > MaxFileBytes {
		return errors.New("fichier d'archive trop volumineux")
	}
	sealed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(text))
	if err != nil || len(sealed) < k.aead.NonceSize()+k.aead.Overhead() {
		return errors.New("fichier d'archive illisible")
	}
	n := k.aead.NonceSize()
	zipped, err := k.aead.Open(nil, sealed[:n], sealed[n:], aad(name))
	if err != nil {
		return errors.New("fichier d'archive illisible ou modifié")
	}
	zr, err := gzip.NewReader(bytes.NewReader(zipped))
	if err != nil {
		return fmt.Errorf("fichier d'archive illisible : %w", err)
	}
	plain, err := io.ReadAll(io.LimitReader(zr, MaxPlainBytes+1))
	if err != nil {
		return fmt.Errorf("fichier d'archive illisible : %w", err)
	}
	if len(plain) > MaxPlainBytes {
		return errors.New("fichier d'archive trop volumineux")
	}
	return json.Unmarshal(plain, v)
}

// --- Contenu ---

// Day : mesures d'un jour (UTC) de tous les PC.
type Day struct {
	Day string  `json:"day"`
	PCs []DayPC `json:"pcs"`
}

// DayPC : mesures d'un PC, reconnu par son adresse MAC (la même sur tous les appareils).
type DayPC struct {
	MAC  string                `json:"mac"`
	Name string                `json:"name"`
	Rows []protocol.MetricsRow `json:"rows"`
}

// Journal : journal d'un mois de tous les PC.
type Journal struct {
	Month string      `json:"month"`
	PCs   []JournalPC `json:"pcs"`
}

// JournalPC : journal de l'agent d'un PC (démarrages, arrêts, veille, commandes).
type JournalPC struct {
	MAC    string                  `json:"mac"`
	Name   string                  `json:"name"`
	Events []protocol.HistoryEvent `json:"events"`
}

// DayStart renvoie les bornes (secondes Unix, UTC) d'un jour « 2026-10-03 ».
func DayStart(day string) (int64, bool) {
	t, err := time.Parse("2006-01-02", day)
	if err != nil {
		return 0, false
	}
	return t.Unix(), true
}

// AddRows ajoute les mesures d'un PC au jour : une minute déjà présente est remplacée, les minutes
// hors du jour ignorées. changed indique si le contenu a changé.
func (d Day) AddRows(mac, name string, rows []protocol.MetricsRow) (out Day, changed bool) {
	start, ok := DayStart(d.Day)
	if !ok {
		return d, false
	}
	out = Day{Day: d.Day, PCs: slices.Clone(d.PCs)}
	i := slices.IndexFunc(out.PCs, func(p DayPC) bool { return p.MAC == mac })
	if i < 0 {
		out.PCs = append(out.PCs, DayPC{MAC: mac, Name: name})
		i = len(out.PCs) - 1
	}
	pc := out.PCs[i]
	byMinute := make(map[int64]protocol.MetricsRow, len(pc.Rows)+len(rows))
	for _, r := range pc.Rows {
		byMinute[r.T] = r
	}
	for _, r := range rows {
		if r.T < start || r.T >= start+86400 {
			continue
		}
		if old, ok := byMinute[r.T]; !ok || !sameRow(old, r) {
			byMinute[r.T] = r
			changed = true
		}
	}
	if name != "" && pc.Name != name {
		pc.Name = name
		changed = true
	}
	pc.Rows = make([]protocol.MetricsRow, 0, len(byMinute))
	for _, r := range byMinute {
		pc.Rows = append(pc.Rows, r)
	}
	slices.SortFunc(pc.Rows, func(a, b protocol.MetricsRow) int { return compare(a.T, b.T) })
	out.PCs[i] = pc
	slices.SortFunc(out.PCs, func(a, b DayPC) int { return strings.Compare(a.MAC, b.MAC) })
	return out, changed
}

// Rows renvoie les mesures d'un PC ce jour-là.
func (d Day) Rows(mac string) []protocol.MetricsRow {
	for _, p := range d.PCs {
		if p.MAC == mac {
			return p.Rows
		}
	}
	return nil
}

func sameRow(a, b protocol.MetricsRow) bool {
	ra, _ := json.Marshal(a)
	rb, _ := json.Marshal(b)
	return bytes.Equal(ra, rb)
}

// AddEvents ajoute le journal d'un PC au mois : évènements déjà présents ignorés, ceux d'un autre
// mois aussi. changed indique si le contenu a changé.
func (j Journal) AddEvents(mac, name string, events []protocol.HistoryEvent) (out Journal, changed bool) {
	out = Journal{Month: j.Month, PCs: slices.Clone(j.PCs)}
	i := slices.IndexFunc(out.PCs, func(p JournalPC) bool { return p.MAC == mac })
	if i < 0 {
		out.PCs = append(out.PCs, JournalPC{MAC: mac, Name: name})
		i = len(out.PCs) - 1
	}
	pc := out.PCs[i]
	pc.Events = slices.Clone(pc.Events)
	for _, e := range events {
		if time.Unix(e.T, 0).UTC().Format("2006-01") != j.Month || slices.Contains(pc.Events, e) {
			continue
		}
		pc.Events = append(pc.Events, e)
		changed = true
	}
	if name != "" && pc.Name != name {
		pc.Name = name
		changed = true
	}
	slices.SortStableFunc(pc.Events, func(a, b protocol.HistoryEvent) int { return compare(a.T, b.T) })
	out.PCs[i] = pc
	slices.SortFunc(out.PCs, func(a, b JournalPC) int { return strings.Compare(a.MAC, b.MAC) })
	return out, changed
}

// Events renvoie le journal d'un PC ce mois-là.
func (j Journal) Events(mac string) []protocol.HistoryEvent {
	for _, p := range j.PCs {
		if p.MAC == mac {
			return p.Events
		}
	}
	return nil
}

func compare(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// Readme : contenu de LISEZMOI.md.
const Readme = `# Patronus – archives chiffrées

Ce Gist secret contient les mesures (températures, utilisation du processeur et de la carte
graphique, minute par minute) et le journal des démarrages et arrêts de vos PC pour un mois,
envoyés par l'application Patronus.

Tout est **chiffré** avec le mot de passe des sauvegardes (AES-256-GCM, clé dérivée par
PBKDF2-HMAC-SHA256, 600 000 itérations) : sans lui, ces fichiers sont illisibles. Seules les dates
apparaissent dans les noms des fichiers.

Consultation : application Patronus → fiche d'un PC → Mesures. Ne modifiez pas ces fichiers ; vous
pouvez supprimer le Gist pour effacer ce mois d'archives.
`

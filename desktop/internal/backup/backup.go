// Package backup décrit les sauvegardes automatiques (docs/SAUVEGARDE.md) : noms des fichiers, versions
// conservées et réglages de l'appareil (chiffrés au repos). Chaque sauvegarde est une sauvegarde
// complète, chiffrée par le mot de passe des sauvegardes (même format que l'export complet), écrite
// dans un Gist secret du compte GitHub et/ou dans un dossier choisi.
package backup

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	// GistDescription identifie le Gist des sauvegardes (un par compte GitHub, commun aux appareils).
	GistDescription = "Patronus – sauvegardes chiffrées"
	// Keep : nombre de sauvegardes quotidiennes conservées par appareil.
	Keep   = 7
	prefix = "patronus-"
)

var namePattern = regexp.MustCompile(`^patronus-([a-z0-9-]{1,40})-(\d{4}-\d{2}-\d{2})\.json$`)

// DeviceID renvoie l'identifiant de fichier d'un appareil : son nom simplifié et 4 caractères
// aléatoires (deux appareils du même nom ne se remplacent pas). Ex. « windows-bureau-3fa2 ».
func DeviceID(kind, name string) string {
	random := make([]byte, 2)
	_, _ = rand.Read(random)
	slug := Slug(kind + "-" + name)
	if len(slug) > 30 {
		slug = strings.Trim(slug[:30], "-")
	}
	return slug + "-" + hex.EncodeToString(random)
}

// accents : lettres accentuées courantes et leur équivalent sans accent.
var accents = strings.NewReplacer(
	"à", "a", "â", "a", "ä", "a", "á", "a", "ã", "a", "å", "a", "ç", "c",
	"é", "e", "è", "e", "ê", "e", "ë", "e", "î", "i", "ï", "i", "í", "i", "ì", "i",
	"ñ", "n", "ô", "o", "ö", "o", "ó", "o", "ò", "o", "õ", "o", "ù", "u", "û", "u", "ü", "u", "ú", "u",
	"ÿ", "y", "œ", "oe", "æ", "ae", "ß", "ss",
)

// Slug simplifie un nom : minuscules sans accents, chiffres et tirets.
func Slug(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range accents.Replace(strings.ToLower(name)) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// FileName renvoie le nom de la sauvegarde du jour d'un appareil.
func FileName(device string, day time.Time) string {
	return prefix + device + "-" + day.Format("2006-01-02") + ".json"
}

// Entry est une sauvegarde repérée par son nom.
type Entry struct {
	Name   string    `json:"name"`
	Device string    `json:"device"`
	Day    time.Time `json:"-"`
	Date   string    `json:"date"`
	Size   int       `json:"size"`
}

// Parse lit le nom d'un fichier de sauvegarde.
func Parse(name string) (Entry, bool) {
	m := namePattern.FindStringSubmatch(name)
	if m == nil {
		return Entry{}, false
	}
	day, err := time.ParseInLocation("2006-01-02", m[2], time.Local)
	if err != nil {
		return Entry{}, false
	}
	return Entry{Name: name, Device: m[1], Day: day, Date: m[2]}, true
}

// Sorted garde les sauvegardes parmi names, de la plus récente à la plus ancienne.
func Sorted(names []string) []Entry {
	var out []Entry
	for _, n := range names {
		if e, ok := Parse(n); ok {
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].Day.Equal(out[j].Day) {
			return out[i].Day.After(out[j].Day)
		}
		return out[i].Device < out[j].Device
	})
	return out
}

// Outdated renvoie les sauvegardes de l'appareil au-delà des keep plus récentes (à supprimer).
func Outdated(names []string, device string, keep int) []string {
	var mine []Entry
	for _, e := range Sorted(names) {
		if e.Device == device {
			mine = append(mine, e)
		}
	}
	var out []string
	for i := keep; i < len(mine); i++ {
		out = append(out, mine[i].Name)
	}
	return out
}

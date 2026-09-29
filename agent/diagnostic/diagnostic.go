// Package diagnostic chiffre les rapports de diagnostic des applications et de l'agent (format
// « patronus-diagnostic/1 », docs/DIAGNOSTIC.md) : un texte chiffré par un mot de passe choisi par
// l'utilisateur, PBKDF2-HMAC-SHA256 puis AES-256-GCM, comme les sauvegardes complètes. Sans le mot
// de passe, le fichier est illisible ; il peut donc être transmis par n'importe quel canal.
package diagnostic

import (
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
	"unicode/utf8"
)

const (
	Format            = "patronus-diagnostic"
	Version           = 1
	KDF               = "PBKDF2WithHmacSHA256"
	Cipher            = "AES-256-GCM"
	DefaultIterations = 600_000
	MinIterations     = 100_000
	MaxIterations     = 10_000_000
	// MinPasswordLength : longueur minimale du mot de passe (caractères), comme les sauvegardes.
	MinPasswordLength = 8
	// MaxReport : taille maximale du texte d'un rapport (octets).
	MaxReport = 8 << 20
)

var aad = []byte(Format + "/1")

// ErrPassword signale un mot de passe incorrect (ou un fichier modifié).
var ErrPassword = errors.New("mot de passe incorrect ou rapport modifié")

// Envelope est le fichier d'un rapport.
type Envelope struct {
	Format     string     `json:"format"`
	Version    int        `json:"version"`
	App        string     `json:"app"`
	CreatedAt  string     `json:"createdAt"`
	Encryption Encryption `json:"encryption"`
	Data       string     `json:"data"`
}

// Encryption décrit le chiffrement (valeurs en Base64 standard).
type Encryption struct {
	KDF        string `json:"kdf"`
	Iterations int    `json:"iterations"`
	Salt       string `json:"salt"`
	Cipher     string `json:"cipher"`
	IV         string `json:"iv"`
}

// Seal chiffre un rapport. app : programme et version (« Patronus Windows 1.5.4 ») ; createdAt :
// date ISO 8601 ; ces deux champs restent lisibles.
func Seal(report, password, app, createdAt string) ([]byte, error) {
	return SealWith(report, password, app, createdAt, DefaultIterations, rand.Reader)
}

// SealWith chiffre avec un nombre d'itérations et une source d'aléa donnés (vecteurs de test).
func SealWith(report, password, app, createdAt string, iterations int, random io.Reader) ([]byte, error) {
	if utf8.RuneCountInString(password) < MinPasswordLength {
		return nil, fmt.Errorf("mot de passe trop court (%d caractères au moins)", MinPasswordLength)
	}
	if len(report) > MaxReport {
		report = report[len(report)-MaxReport:]
	}
	salt := make([]byte, 16)
	iv := make([]byte, 12)
	if _, err := io.ReadFull(random, salt); err != nil {
		return nil, err
	}
	if _, err := io.ReadFull(random, iv); err != nil {
		return nil, err
	}
	aead, err := newAEAD(password, salt, iterations)
	if err != nil {
		return nil, err
	}
	env := Envelope{
		Format: Format, Version: Version, App: app, CreatedAt: createdAt,
		Encryption: Encryption{KDF: KDF, Iterations: iterations, Salt: base64.StdEncoding.EncodeToString(salt),
			Cipher: Cipher, IV: base64.StdEncoding.EncodeToString(iv)},
		Data: base64.StdEncoding.EncodeToString(aead.Seal(nil, iv, []byte(report), aad)),
	}
	return json.MarshalIndent(env, "", "  ")
}

// Open déchiffre un rapport.
func Open(data []byte, password string) (Envelope, string, error) {
	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil || env.Format != Format {
		return env, "", errors.New("ce fichier n'est pas un rapport de diagnostic Patronus")
	}
	e := env.Encryption
	if env.Version != Version || e.KDF != KDF || e.Cipher != Cipher {
		return env, "", fmt.Errorf("rapport de version %d non pris en charge", env.Version)
	}
	if e.Iterations < MinIterations || e.Iterations > MaxIterations {
		return env, "", errors.New("paramètres de chiffrement invalides")
	}
	salt, err1 := base64.StdEncoding.DecodeString(e.Salt)
	iv, err2 := base64.StdEncoding.DecodeString(e.IV)
	sealed, err3 := base64.StdEncoding.DecodeString(env.Data)
	if err1 != nil || err2 != nil || err3 != nil || len(salt) < 16 || len(iv) != 12 {
		return env, "", errors.New("rapport abîmé")
	}
	aead, err := newAEAD(password, salt, e.Iterations)
	if err != nil {
		return env, "", err
	}
	plain, err := aead.Open(nil, iv, sealed, aad)
	if err != nil {
		return env, "", ErrPassword
	}
	return env, string(plain), nil
}

// newAEAD dérive la clé AES-256 du mot de passe (encodé en UTF-8, comme Java/Android).
func newAEAD(password string, salt []byte, iterations int) (cipher.AEAD, error) {
	key, err := pbkdf2.Key(sha256.New, password, salt, iterations, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	clear(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

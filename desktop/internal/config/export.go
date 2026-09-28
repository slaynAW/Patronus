package config

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"

	"github.com/slaynaw/wakeonlan/desktop/internal/model"
)

// Format d'export, identique à Android (ExportCodec).
const (
	Format            = "wakeonlan-config"
	FormatVersion     = 1
	KDF               = "PBKDF2WithHmacSHA256"
	Cipher            = "AES-256-GCM"
	DefaultIterations = 600_000
	MinIterations     = 100_000
	MaxIterations     = 10_000_000
	MinPasswordLength = 8
	// MaxImportBytes borne la taille d'un fichier importé.
	MaxImportBytes = 1024 * 1024
)

var aad = []byte(Format + "/1")

// Envelope est l'enveloppe d'un fichier de sauvegarde (*.json).
type Envelope struct {
	Format     string `json:"format"`
	Version    int    `json:"version"`
	ExportedAt string `json:"exportedAt"`
	App        string `json:"app,omitempty"`
	// Config est la configuration en clair (export SANS secrets).
	Config json.RawMessage `json:"config,omitempty"`
	// Encryption décrit le chiffrement (export AVEC secrets).
	Encryption *EncryptionInfo `json:"encryption,omitempty"`
	// Data est la configuration chiffrée, en Base64.
	Data string `json:"data,omitempty"`
}

// EncryptionInfo décrit les paramètres de chiffrement d'une sauvegarde.
type EncryptionInfo struct {
	KDF        string `json:"kdf"`
	Iterations int    `json:"iterations"`
	Salt       string `json:"salt"`
	Cipher     string `json:"cipher"`
	IV         string `json:"iv"`
}

// ExportOptions paramètre un export.
type ExportOptions struct {
	// Password nul (vide) = export lisible SANS aucun secret.
	Password   string
	ExportedAt string
	App        string
	// Iterations PBKDF2 ; 0 = valeur par défaut.
	Iterations int
	// Extra ajoute des données à la configuration chiffrée (ex. « sharing » : clé de partage),
	// ignorées par les versions qui ne les connaissent pas. Jamais dans un export lisible.
	Extra map[string]json.RawMessage
}

// Export produit le texte d'une sauvegarde.
//
// Sans mot de passe : fichier lisible, clés d'agent et mots de passe SecureOn retirés.
// Avec mot de passe : configuration complète chiffrée en AES-256-GCM, clé dérivée par
// PBKDF2-HMAC-SHA256 (600 000 itérations, sel aléatoire) ; toute modification est détectée.
func Export(c model.AppConfig, opts ExportOptions) ([]byte, error) {
	env := Envelope{Format: Format, Version: FormatVersion, ExportedAt: opts.ExportedAt, App: opts.App}
	if opts.Password == "" {
		stripped := c.Clone()
		for i, d := range stripped.Devices {
			stripped.Devices[i] = d.WithoutSecrets()
		}
		raw, err := json.Marshal(stripped)
		if err != nil {
			return nil, err
		}
		env.Config = raw
	} else {
		if model.UTF16Len(opts.Password) < MinPasswordLength {
			return nil, errors.New("Mot de passe trop court")
		}
		iterations := opts.Iterations
		if iterations == 0 {
			iterations = DefaultIterations
		}
		salt, iv := make([]byte, 16), make([]byte, 12)
		if _, err := rand.Read(salt); err != nil {
			return nil, err
		}
		if _, err := rand.Read(iv); err != nil {
			return nil, err
		}
		plain, err := Encode(c)
		if err != nil {
			return nil, err
		}
		if len(opts.Extra) > 0 {
			var root map[string]json.RawMessage
			if err := json.Unmarshal(plain, &root); err != nil {
				return nil, err
			}
			for k, v := range opts.Extra {
				if _, taken := root[k]; !taken {
					root[k] = v
				}
			}
			if plain, err = json.Marshal(root); err != nil {
				return nil, err
			}
		}
		aead, err := newAEAD(opts.Password, salt, iterations, len(iv))
		if err != nil {
			return nil, err
		}
		env.Encryption = &EncryptionInfo{
			KDF: KDF, Iterations: iterations, Cipher: Cipher,
			Salt: base64.StdEncoding.EncodeToString(salt), IV: base64.StdEncoding.EncodeToString(iv),
		}
		env.Data = base64.StdEncoding.EncodeToString(aead.Seal(nil, iv, plain, aad))
	}
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "    ")
	if err := encoder.Encode(env); err != nil {
		return nil, err
	}
	return bytes.TrimRight(out.Bytes(), "\n"), nil
}

// Inspect lit l'enveloppe sans la déchiffrer (pour savoir s'il faut demander un mot de passe).
func Inspect(data []byte) (Envelope, error) {
	var raw struct {
		Format     *string         `json:"format"`
		Version    *int            `json:"version"`
		ExportedAt *string         `json:"exportedAt"`
		App        string          `json:"app"`
		Config     json.RawMessage `json:"config"`
		Encryption *EncryptionInfo `json:"encryption"`
		Data       string          `json:"data"`
	}
	notBackup := fail(NotABackup, "Ce fichier n'est pas une sauvegarde Wake On LAN")
	if err := json.Unmarshal(data, &raw); err != nil || raw.Format == nil || raw.Version == nil || raw.ExportedAt == nil {
		return Envelope{}, notBackup
	}
	if *raw.Format != Format {
		return Envelope{}, notBackup
	}
	if *raw.Version > FormatVersion {
		return Envelope{}, fail(NewerVersion,
			"Sauvegarde créée par une version plus récente de l'application : mettez-la à jour.")
	}
	return Envelope{
		Format: *raw.Format, Version: *raw.Version, ExportedAt: *raw.ExportedAt, App: raw.App,
		Config: raw.Config, Encryption: raw.Encryption, Data: raw.Data,
	}, nil
}

// IsEncrypted indique si la sauvegarde est protégée par un mot de passe.
func IsEncrypted(data []byte) (bool, error) {
	env, err := Inspect(data)
	return env.Encryption != nil, err
}

// Import lit une sauvegarde (password vide si elle n'est pas chiffrée) et la valide entièrement.
func Import(data []byte, password string) (model.AppConfig, error) {
	c, _, err := ImportWithExtra(data, password)
	return c, err
}

// ImportWithExtra lit une sauvegarde et renvoie aussi les données ajoutées (voir ExportOptions.Extra),
// présentes seulement dans une sauvegarde chiffrée.
func ImportWithExtra(data []byte, password string) (model.AppConfig, map[string]json.RawMessage, error) {
	c, plain, err := importDecrypt(data, password)
	if err != nil || plain == nil {
		return c, nil, err
	}
	var root map[string]json.RawMessage
	if json.Unmarshal(plain, &root) != nil {
		return c, nil, nil
	}
	extra := map[string]json.RawMessage{}
	for _, k := range []string{"sharing"} {
		if v, ok := root[k]; ok {
			extra[k] = v
		}
	}
	return c, extra, nil
}

// importDecrypt renvoie la configuration et, pour une sauvegarde chiffrée, le JSON déchiffré.
func importDecrypt(data []byte, password string) (model.AppConfig, []byte, error) {
	env, err := Inspect(data)
	if err != nil {
		return model.AppConfig{}, nil, err
	}
	enc := env.Encryption
	if enc == nil {
		var root map[string]json.RawMessage
		if len(env.Config) == 0 || json.Unmarshal(env.Config, &root) != nil || root == nil {
			return model.AppConfig{}, nil, fail(InvalidData, "Sauvegarde vide")
		}
		c, err := fromJSON(root)
		return c, nil, err
	}
	if password == "" {
		return model.AppConfig{}, nil, fail(PasswordRequired, "Cette sauvegarde est protégée par un mot de passe")
	}
	if enc.KDF != KDF || enc.Cipher != Cipher || enc.Iterations < MinIterations || enc.Iterations > MaxIterations {
		return model.AppConfig{}, nil, fail(InvalidData, "Paramètres de chiffrement non pris en charge")
	}
	corrupted := fail(InvalidData, "Sauvegarde corrompue")
	salt, err1 := decodeStd(enc.Salt)
	iv, err2 := decodeStd(enc.IV)
	sealed, err3 := decodeStd(env.Data)
	if err1 != nil || err2 != nil || err3 != nil || len(iv) == 0 {
		return model.AppConfig{}, nil, corrupted
	}
	aead, err := newAEAD(password, salt, enc.Iterations, len(iv))
	if err != nil {
		return model.AppConfig{}, nil, corrupted
	}
	plain, err := aead.Open(nil, iv, sealed, aad)
	if err != nil {
		return model.AppConfig{}, nil, fail(WrongPassword, "Mot de passe incorrect ou fichier modifié")
	}
	c, err := Decode(plain)
	return c, plain, err
}

// newAEAD dérive la clé AES-256 du mot de passe (encodé en UTF-8, comme Java/Android).
func newAEAD(password string, salt []byte, iterations, nonceSize int) (cipher.AEAD, error) {
	key, err := pbkdf2.Key(sha256.New, password, salt, iterations, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	clear(key)
	if err != nil {
		return nil, err
	}
	if nonceSize == 12 {
		return cipher.NewGCM(block)
	}
	return cipher.NewGCMWithNonceSize(block, nonceSize)
}

// decodeStd décode du Base64 standard, avec ou sans remplissage (comme java.util.Base64).
func decodeStd(s string) ([]byte, error) {
	if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.RawStdEncoding.DecodeString(s)
}

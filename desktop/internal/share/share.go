// Package share implémente le partage chiffré de PC entre personnes (format « wakeonlan-share/1 »,
// décrit dans docs/PARTAGE.md et partagé avec l'application Android).
//
// Principe : la personne qui partage (le « propriétaire ») possède une clé de signature ECDSA P-256 ;
// chaque appareil qui reçoit un accès possède sa propre clé ECDH P-256, créée sur place et jamais
// transmise. Pour chaque appareil autorisé, le propriétaire publie un fichier d'accès :
//
//   - chiffré pour ce seul appareil (ECDH éphémère + HKDF-SHA256 + AES-256-GCM) ;
//   - signé par la clé du propriétaire, connue de l'appareil depuis l'invitation ;
//   - numéroté (révision croissante) pour refuser le retour à une version plus ancienne.
//
// Un fichier d'accès ne peut donc être lu que par l'appareil prévu et ne peut être fabriqué que
// par le propriétaire, même s'il est stocké dans un endroit public.
package share

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/slaynaw/wakeonlan/desktop/internal/config"
	"github.com/slaynaw/wakeonlan/desktop/internal/model"
)

const (
	// Format et Version identifient les fichiers d'accès.
	Format  = "wakeonlan-share"
	Version = 1

	// MaxNameLength est la longueur maximale d'un nom (propriétaire ou destinataire), en caractères.
	MaxNameLength = 40
	// MaxFileSize borne la taille d'un fichier d'accès (lecture comme écriture).
	MaxFileSize = 256 << 10
	// MaxDevices borne le nombre de PC dans un accès.
	MaxDevices = 100

	domain     = "wakeonlan-share/1"
	signPrefix = domain + "\n"
	hkdfInfo   = domain + " access"
	verifyInfo = domain + " verify"
	publicLen  = 65
	ivLen      = 12
)

// Raisons d'échec d'ouverture d'un fichier d'accès.
var (
	ErrInvalid   = errors.New("fichier d'accès invalide")
	ErrSignature = errors.New("fichier d'accès non signé par la personne qui partage")
	ErrRecipient = errors.New("fichier d'accès destiné à un autre appareil")
	ErrOlder     = errors.New("fichier d'accès plus ancien que celui déjà reçu")
)

var b64url = base64.RawURLEncoding

// --- Clés ---

// NewOwnerKey crée une clé de signature (personne qui partage).
func NewOwnerKey() (*ecdsa.PrivateKey, error) { return ecdsa.GenerateKey(elliptic.P256(), rand.Reader) }

// NewDeviceKey crée la clé de réception d'un appareil.
func NewDeviceKey() (*ecdh.PrivateKey, error) { return ecdh.P256().GenerateKey(rand.Reader) }

// EncodePrivate encode une clé privée (32 octets) pour le stockage local.
func EncodePrivate(raw []byte) string { return b64url.EncodeToString(raw) }

// ParseOwnerPrivate relit une clé de signature enregistrée par EncodePrivate.
func ParseOwnerPrivate(s string) (*ecdsa.PrivateKey, error) {
	raw, err := b64url.DecodeString(s)
	if err != nil {
		return nil, ErrInvalid
	}
	key, err := ecdsa.ParseRawPrivateKey(elliptic.P256(), raw)
	if err != nil {
		return nil, ErrInvalid
	}
	return key, nil
}

// OwnerPrivateBytes renvoie les 32 octets d'une clé de signature.
func OwnerPrivateBytes(key *ecdsa.PrivateKey) []byte {
	raw, err := key.Bytes()
	if err != nil {
		panic(err) // impossible pour une clé P-256 valide
	}
	return raw
}

// ParseDevicePrivate relit une clé de réception enregistrée par EncodePrivate.
func ParseDevicePrivate(s string) (*ecdh.PrivateKey, error) {
	raw, err := b64url.DecodeString(s)
	if err != nil {
		return nil, ErrInvalid
	}
	key, err := ecdh.P256().NewPrivateKey(raw)
	if err != nil {
		return nil, ErrInvalid
	}
	return key, nil
}

// OwnerPublic renvoie la clé publique de signature encodée (point non compressé, Base64 URL).
func OwnerPublic(key *ecdsa.PrivateKey) string {
	raw, err := key.PublicKey.Bytes()
	if err != nil {
		panic(err)
	}
	return b64url.EncodeToString(raw)
}

// DevicePublic renvoie la clé publique de réception encodée.
func DevicePublic(key *ecdh.PrivateKey) string {
	return b64url.EncodeToString(key.PublicKey().Bytes())
}

// parseOwnerPublic décode et vérifie une clé publique de signature.
func parseOwnerPublic(s string) (*ecdsa.PublicKey, []byte, error) {
	raw, err := b64url.DecodeString(s)
	if err != nil || len(raw) != publicLen {
		return nil, nil, ErrInvalid
	}
	key, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), raw)
	if err != nil {
		return nil, nil, ErrInvalid
	}
	return key, raw, nil
}

// parseDevicePublic décode et vérifie une clé publique de réception.
func parseDevicePublic(s string) (*ecdh.PublicKey, error) {
	raw, err := b64url.DecodeString(s)
	if err != nil || len(raw) != publicLen {
		return nil, ErrInvalid
	}
	key, err := ecdh.P256().NewPublicKey(raw)
	if err != nil {
		return nil, ErrInvalid
	}
	return key, nil
}

// ValidOwnerKey et ValidDeviceKey vérifient une clé publique reçue (lien, fichier).
func ValidOwnerKey(s string) bool {
	_, _, err := parseOwnerPublic(s)
	return err == nil
}

func ValidDeviceKey(s string) bool {
	_, err := parseDevicePublic(s)
	return err == nil
}

// KeyID est l'identifiant court d'une clé publique encodée : 32 caractères hexadécimaux
// (début du SHA-256 du point non compressé). Il nomme les fichiers d'accès.
func KeyID(public string) string {
	raw, err := b64url.DecodeString(public)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:16])
}

// FileName est le nom du fichier d'accès d'un appareil dans l'espace de stockage.
func FileName(devicePublic string) string { return "acces-" + KeyID(devicePublic) + ".json" }

// VerificationCode renvoie le code à 6 chiffres affiché des deux côtés lors d'une demande
// (« 123 456 ») : identique seulement si chacun a bien reçu la clé de l'autre.
func VerificationCode(ownerPublic, devicePublic string) string {
	owner, err1 := b64url.DecodeString(ownerPublic)
	device, err2 := b64url.DecodeString(devicePublic)
	if err1 != nil || err2 != nil {
		return ""
	}
	h := sha256.New()
	h.Write([]byte(verifyInfo))
	h.Write(owner)
	h.Write(device)
	n := binary.BigEndian.Uint32(h.Sum(nil)[:4]) % 1_000_000
	s := fmt.Sprintf("%06d", n)
	return s[:3] + " " + s[3:]
}

// ValidName vérifie un nom de personne (1 à MaxNameLength caractères, sans caractère de contrôle).
func ValidName(name string) bool {
	n := utf8.RuneCountInString(name)
	if strings.TrimSpace(name) != name || n == 0 || n > MaxNameLength || !utf8.ValidString(name) {
		return false
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// --- Fichier d'accès ---

// File est un fichier d'accès tel que stocké.
type File struct {
	Format  string `json:"format"`
	Version int    `json:"version"`
	// Payload est le contenu signé (JSON), en Base64.
	Payload string `json:"payload"`
	// Signature est la signature ECDSA P-256 / SHA-256 (DER, Base64) de "wakeonlan-share/1\n" + Payload décodé.
	Signature string `json:"signature"`
}

// payload est le contenu signé d'un fichier d'accès.
type payload struct {
	Owner    string `json:"owner"`
	Device   string `json:"device"`
	Revision int64  `json:"revision"`
	IssuedAt string `json:"issuedAt"`
	// Clé éphémère, IV et données chiffrées (AES-256-GCM, étiquette incluse).
	EPK  string `json:"epk"`
	IV   string `json:"iv"`
	Data string `json:"data"`
}

// Content est ce que reçoit l'appareil autorisé.
type Content struct {
	OwnerName     string         `json:"ownerName"`
	RecipientName string         `json:"recipientName"`
	Devices       []model.Device `json:"devices"`
}

// Opened est un fichier d'accès vérifié et déchiffré.
type Opened struct {
	Revision int64
	IssuedAt string
	Content  Content
}

// Seal fabrique le fichier d'accès d'un appareil. revision doit croître à chaque publication.
func Seal(owner *ecdsa.PrivateKey, devicePublic string, revision int64, issuedAt string, content Content) ([]byte, error) {
	plain, err := json.Marshal(content)
	if err != nil {
		return nil, err
	}
	eph, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	iv := make([]byte, ivLen)
	if _, err := rand.Read(iv); err != nil {
		return nil, err
	}
	return seal(owner, devicePublic, revision, issuedAt, plain, eph, iv)
}

// seal est la partie déterministe de Seal (hors signature), utilisée par les vecteurs de test.
func seal(owner *ecdsa.PrivateKey, devicePublic string, revision int64, issuedAt string, plain []byte, eph *ecdh.PrivateKey, iv []byte) ([]byte, error) {
	p, err := encrypt(OwnerPublic(owner), devicePublic, revision, issuedAt, plain, eph, iv)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(append([]byte(signPrefix), body...))
	sig, err := ecdsa.SignASN1(rand.Reader, owner, digest[:])
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(File{
		Format: Format, Version: Version,
		Payload:   base64.StdEncoding.EncodeToString(body),
		Signature: base64.StdEncoding.EncodeToString(sig),
	}, "", "  ")
}

func encrypt(ownerPublic, devicePublic string, revision int64, issuedAt string, plain []byte, eph *ecdh.PrivateKey, iv []byte) (payload, error) {
	device, err := parseDevicePublic(devicePublic)
	if err != nil {
		return payload{}, err
	}
	shared, err := eph.ECDH(device)
	if err != nil {
		return payload{}, err
	}
	gcm, err := newGCM(shared, eph.PublicKey().Bytes(), device.Bytes())
	if err != nil {
		return payload{}, err
	}
	aad, err := additionalData(ownerPublic, devicePublic)
	if err != nil {
		return payload{}, err
	}
	return payload{
		Owner: ownerPublic, Device: devicePublic, Revision: revision, IssuedAt: issuedAt,
		EPK:  b64url.EncodeToString(eph.PublicKey().Bytes()),
		IV:   base64.StdEncoding.EncodeToString(iv),
		Data: base64.StdEncoding.EncodeToString(gcm.Seal(nil, iv, plain, aad)),
	}, nil
}

// newGCM dérive la clé AES-256 : HKDF-SHA256(secret ECDH, sel = clé éphémère ‖ clé de l'appareil).
func newGCM(shared, ephPublic, devicePublic []byte) (cipher.AEAD, error) {
	salt := append(append([]byte{}, ephPublic...), devicePublic...)
	key, err := hkdf.Key(sha256.New, shared, salt, hkdfInfo, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// additionalData lie le chiffrement aux deux clés : "wakeonlan-share/1" ‖ clé du propriétaire ‖ clé de l'appareil.
func additionalData(ownerPublic, devicePublic string) ([]byte, error) {
	owner, err1 := b64url.DecodeString(ownerPublic)
	device, err2 := b64url.DecodeString(devicePublic)
	if err1 != nil || err2 != nil {
		return nil, ErrInvalid
	}
	return append(append([]byte(domain), owner...), device...), nil
}

// Open vérifie puis déchiffre un fichier d'accès :
//   - signature de la clé ownerPublic (celle de l'invitation) ;
//   - destiné à l'appareil device ;
//   - révision au moins égale à minRevision.
//
// La signature est vérifiée avant toute autre opération sur le contenu.
func Open(data []byte, ownerPublic string, device *ecdh.PrivateKey, minRevision int64) (Opened, error) {
	if len(data) > MaxFileSize {
		return Opened{}, ErrInvalid
	}
	var f File
	if err := json.Unmarshal(data, &f); err != nil || f.Format != Format {
		return Opened{}, ErrInvalid
	}
	if f.Version != Version {
		return Opened{}, fmt.Errorf("%w : version %d non prise en charge, mettez l'application à jour", ErrInvalid, f.Version)
	}
	body, err1 := base64.StdEncoding.DecodeString(f.Payload)
	sig, err2 := base64.StdEncoding.DecodeString(f.Signature)
	if err1 != nil || err2 != nil {
		return Opened{}, ErrInvalid
	}
	ownerKey, _, err := parseOwnerPublic(ownerPublic)
	if err != nil {
		return Opened{}, err
	}
	digest := sha256.Sum256(append([]byte(signPrefix), body...))
	if !ecdsa.VerifyASN1(ownerKey, digest[:], sig) {
		return Opened{}, ErrSignature
	}
	var p payload
	if err := json.Unmarshal(body, &p); err != nil {
		return Opened{}, ErrInvalid
	}
	if p.Owner != ownerPublic {
		return Opened{}, ErrSignature
	}
	mine := DevicePublic(device)
	if p.Device != mine {
		return Opened{}, ErrRecipient
	}
	if p.Revision < minRevision {
		return Opened{}, ErrOlder
	}
	eph, err := parseDevicePublic(p.EPK)
	if err != nil {
		return Opened{}, err
	}
	iv, err1 := base64.StdEncoding.DecodeString(p.IV)
	ct, err2 := base64.StdEncoding.DecodeString(p.Data)
	if err1 != nil || err2 != nil || len(iv) != ivLen {
		return Opened{}, ErrInvalid
	}
	shared, err := device.ECDH(eph)
	if err != nil {
		return Opened{}, ErrInvalid
	}
	gcm, err := newGCM(shared, eph.Bytes(), device.PublicKey().Bytes())
	if err != nil {
		return Opened{}, err
	}
	aad, err := additionalData(p.Owner, p.Device)
	if err != nil {
		return Opened{}, err
	}
	plain, err := gcm.Open(nil, iv, ct, aad)
	if err != nil {
		return Opened{}, ErrInvalid
	}
	var c Content
	dec := json.NewDecoder(bytes.NewReader(plain))
	if err := dec.Decode(&c); err != nil {
		return Opened{}, fmt.Errorf("%w : %v", ErrInvalid, err)
	}
	if !ValidName(c.OwnerName) || len(c.Devices) > MaxDevices {
		return Opened{}, ErrInvalid
	}
	// Mêmes contrôles qu'un import : ne jamais faire confiance au contenu reçu.
	checked, err := config.Sanitize(model.AppConfig{Devices: c.Devices})
	if err != nil {
		return Opened{}, fmt.Errorf("%w : %v", ErrInvalid, err)
	}
	c.Devices = checked.Devices
	return Opened{Revision: p.Revision, IssuedAt: p.IssuedAt, Content: c}, nil
}

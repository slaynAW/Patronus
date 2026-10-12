package protocol2

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"errors"
	"math/big"
	"time"
)

var (
	// ErrPin : empreinte mal écrite.
	ErrPin = errors.New("wolagent/2 : empreinte invalide")
	// ErrCertificate : certificat illisible.
	ErrCertificate = errors.New("wolagent/2 : certificat illisible")
	// ErrKeyType : la clé du certificat n'est pas une clé ECDSA P-256 sous sa forme standard.
	ErrKeyType = errors.New("wolagent/2 : clé autre que ECDSA P-256")
)

// Pin est l'empreinte d'une identité (§2.1) : SHA-256 de sa clé publique (SubjectPublicKeyInfo en DER),
// soit la valeur « pin-sha256 » de la RFC 7469.
type Pin [sha256.Size]byte

// p256SPKIPrefix commence tout SubjectPublicKeyInfo P-256 sous sa forme standard : id-ecPublicKey,
// courbe nommée prime256v1, point non compressé (0x04). Suivent les coordonnées X et Y (64 octets).
var p256SPKIPrefix = []byte{
	0x30, 0x59, 0x30, 0x13, 0x06, 0x07, 0x2a, 0x86, 0x48, 0xce, 0x3d, 0x02, 0x01,
	0x06, 0x08, 0x2a, 0x86, 0x48, 0xce, 0x3d, 0x03, 0x01, 0x07, 0x03, 0x42, 0x00, 0x04,
}

// p256SPKILength est la taille de ce SubjectPublicKeyInfo.
const p256SPKILength = 91

// PinOf calcule l'empreinte d'une clé publique encodée (SubjectPublicKeyInfo en DER).
func PinOf(spki []byte) Pin { return sha256.Sum256(spki) }

// ParsePin lit une empreinte écrite en B64 (43 caractères).
func ParsePin(s string) (Pin, error) {
	var p Pin
	raw, err := decodeB64(s, len(p))
	if err != nil {
		return p, ErrPin
	}
	copy(p[:], raw)
	return p, nil
}

// String écrit l'empreinte en B64, sa forme dans les messages, les liens et les fichiers.
func (p Pin) String() string { return b64.EncodeToString(p[:]) }

// MarshalText et UnmarshalText écrivent l'empreinte en B64 dans le JSON.
func (p Pin) MarshalText() ([]byte, error) { return []byte(p.String()), nil }

func (p *Pin) UnmarshalText(text []byte) error {
	parsed, err := ParsePin(string(text))
	if err != nil {
		return err
	}
	*p = parsed
	return nil
}

// Short renvoie l'empreinte courte (§2.1) : les 60 premiers bits en base32 de Crockford, par groupes de
// quatre (« 7KQ2-9MXP-4D1Z »). Elle sert à comparer à l'œil, jamais d'identifiant.
func (p Pin) Short() string {
	bits := binary.BigEndian.Uint64(p[:8]) >> 4
	out := make([]byte, 0, 14)
	for i := range 12 {
		if i == 4 || i == 8 {
			out = append(out, '-')
		}
		out = append(out, crockford[bits>>(55-5*i)&31])
	}
	return string(out)
}

// PinFromCertificate vérifie le certificat présenté dans TLS par l'autre côté et renvoie son empreinte.
// Seule la clé compte (§2.2) : ECDSA P-256 sous sa forme standard, sinon ErrKeyType. L'émetteur, les
// dates, les extensions et la signature du certificat sont ignorés ; la possession de la clé est
// prouvée par TLS.
func PinFromCertificate(der []byte) (Pin, error) {
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return Pin{}, ErrCertificate
	}
	pub, ok := cert.PublicKey.(*ecdsa.PublicKey)
	spki := cert.RawSubjectPublicKeyInfo
	if !ok || pub.Curve != elliptic.P256() || len(spki) != p256SPKILength || !bytes.HasPrefix(spki, p256SPKIPrefix) {
		return Pin{}, ErrKeyType
	}
	return PinOf(spki), nil
}

// NewCertificate crée le certificat autosigné qui présente la clé dans TLS (§2.2) : sujet CN=wolagent,
// numéro de série aléatoire de 16 octets, valable de la veille de now au 31/12/9999. Il peut être
// recréé à tout moment : l'empreinte ne dépend que de la clé.
func NewCertificate(key *ecdsa.PrivateKey, now time.Time) ([]byte, error) {
	if key == nil || key.Curve != elliptic.P256() {
		return nil, ErrKeyType
	}
	serial := make([]byte, 16)
	_, _ = rand.Read(serial)          // ne renvoie jamais d'erreur
	serial[0] = serial[0]&0x3f | 0x40 // positif, et toujours 16 octets une fois encodé
	template := &x509.Certificate{
		SerialNumber:       new(big.Int).SetBytes(serial),
		Subject:            pkix.Name{CommonName: "wolagent"},
		NotBefore:          now.UTC().Add(-24 * time.Hour).Truncate(time.Second),
		NotAfter:           time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC),
		KeyUsage:           x509.KeyUsageDigitalSignature,
		SignatureAlgorithm: x509.ECDSAWithSHA256,
	}
	return x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
}

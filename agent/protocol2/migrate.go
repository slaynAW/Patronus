package protocol2

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
)

var (
	// ErrNonce : nonce de migration mal écrit.
	ErrNonce = errors.New("wolagent/2 : nonce invalide")
	// ErrProof : preuve de migration mal écrite.
	ErrProof = errors.New("wolagent/2 : preuve invalide")
)

// Libellés des deux preuves de migration (§9). Distincts l'un de l'autre, ils empêchent de renvoyer une
// preuve comme l'autre ; et les messages signés par la clé en wolagent/1 commencent tous par
// « wolagent/1\n » : une signature d'un protocole ne vaut jamais preuve dans l'autre.
const (
	agentProofLabel  = "wolagent/2 migrate agent"
	deviceProofLabel = "wolagent/2 migrate device"
)

// Nonce est un nonce de migration : 32 octets aléatoires, écrits en B64.
type Nonce [NonceBytes]byte

// NewNonce tire un nonce de migration.
func NewNonce() Nonce {
	var n Nonce
	_, _ = rand.Read(n[:]) // ne renvoie jamais d'erreur
	return n
}

// ParseNonce lit un nonce écrit en B64 (43 caractères).
func ParseNonce(s string) (Nonce, error) {
	var n Nonce
	raw, err := decodeB64(s, len(n))
	if err != nil {
		return n, ErrNonce
	}
	copy(n[:], raw)
	return n, nil
}

func (n Nonce) String() string { return b64.EncodeToString(n[:]) }

// Proof est une preuve de migration (HMAC-SHA256), écrite en B64.
type Proof [sha256.Size]byte

// ParseProof lit une preuve écrite en B64 (43 caractères).
func ParseProof(s string) (Proof, error) {
	var p Proof
	raw, err := decodeB64(s, len(p))
	if err != nil {
		return p, ErrProof
	}
	copy(p[:], raw)
	return p, nil
}

func (p Proof) String() string { return b64.EncodeToString(p[:]) }

// Equal compare deux preuves en temps constant.
func (p Proof) Equal(other Proof) bool { return hmac.Equal(p[:], other[:]) }

// AgentProof calcule la preuve de l'agent, Pa, avec la clé wolagent/1 du PC. Chaque côté y met les
// empreintes telles que vues dans sa propre poignée de main TLS : un intermédiaire, qui présente sa propre
// identité de chaque côté, ne peut pas obtenir des preuves qui concordent.
func AgentProof(key []byte, agent, device Pin, nc, na Nonce) Proof {
	return migrationProof(key, agentProofLabel, agent, device, nc, na)
}

// DeviceProof calcule la preuve de l'appareil, Pd, envoyée une fois celle de l'agent vérifiée.
func DeviceProof(key []byte, agent, device Pin, nc, na Nonce) Proof {
	return migrationProof(key, deviceProofLabel, agent, device, nc, na)
}

// migrationProof : HMAC-SHA256(K, libellé ‖ 0x00 ‖ empreinte(agent) ‖ empreinte(appareil) ‖ Nc ‖ Na).
// Tous les champs après le libellé ont une taille fixe : l'écriture est sans ambiguïté.
func migrationProof(key []byte, label string, agent, device Pin, nc, na Nonce) Proof {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(label))
	mac.Write([]byte{0})
	mac.Write(agent[:])
	mac.Write(device[:])
	mac.Write(nc[:])
	mac.Write(na[:])
	var p Proof
	mac.Sum(p[:0])
	return p
}

// Package protocol2 est l'implémentation de référence des fonctions pures du protocole « wolagent/2 »
// (docs/PROTOCOLE-V2.md) : empreintes des identités, codes et liens d'appairage, rôles, preuves de
// migration et trames. Le transport TLS et le registre des appareils viendront avec l'agent.
//
// Les vecteurs de test protocol/v2-vectors.json sont produits par ce paquet
// (go test ./protocol2 -run TestVectors -update). Ils sont recalculés indépendamment par
// .github/scripts/check-v2-vectors.py (Python et OpenSSL), et le seront par l'application Android.
package protocol2

import (
	"encoding/base64"
	"errors"
)

const (
	// ALPN est le protocole applicatif annoncé dans TLS (§3.2).
	ALPN = "wolagent/2"
	// DefaultPort est le port de l'agent, partagé avec wolagent/1 (§3.1).
	DefaultPort = 9770
	// MaxRequestBytes borne une trame de requête (§4.1).
	MaxRequestBytes = 16 << 10
	// MaxResponseBytes borne une trame de réponse (§4.1).
	MaxResponseBytes = 4 << 20
	// MaxRequestID est le plus grand identifiant de requête (§4.2).
	MaxRequestID = 1<<31 - 1
	// NonceBytes est la taille des nonces de migration (§9).
	NonceBytes = 32
	// MaxPCNameLength borne le nom du PC lu dans un lien d'appairage, en unités UTF-16 comme les
	// applications (§7.1).
	MaxPCNameLength = 64
	// MaxDeviceNameLength borne le nom d'un appareil enregistré, comme « by » en wolagent/1 (§6.2).
	MaxDeviceNameLength = 40
	// DefaultInviteTTL, MinInviteTTL et MaxInviteTTL encadrent la validité d'une invitation, en
	// secondes (§6.2).
	DefaultInviteTTL = 600
	MinInviteTTL     = 60
	MaxInviteTTL     = 7 * 24 * 3600
	// MaxPendingInvites borne le nombre d'invitations en attente (§6.2).
	MaxPendingInvites = 10
)

// Codes de réponse (§4.4).
const (
	CodeOK               = "ok"
	CodeBadRequest       = "bad_request"
	CodeUnsupported      = "unsupported"
	CodeBusy             = "busy"
	CodeForbidden        = "forbidden"
	CodeInsufficientRole = "insufficient_role"
	CodeAlreadyPaired    = "already_paired"
	CodeLastAdmin        = "last_admin"
	CodeUnknownDevice    = "unknown_device"
	CodePairingFailed    = "pairing_failed"
	CodeMigrationFailed  = "migration_failed"
	CodeRateLimited      = "rate_limited"
)

// b64 est l'écriture « B64 » de la spécification : Base64 URL sans remplissage.
var b64 = base64.RawURLEncoding

var errB64 = errors.New("wolagent/2 : valeur B64 invalide")

// decodeB64 décode exactement n octets écrits en B64 sous leur forme canonique : une seule écriture
// est acceptée pour chaque valeur (ni remplissage, ni retour à la ligne, ni bits de fin non nuls).
func decodeB64(s string, n int) ([]byte, error) {
	if len(s) != b64.EncodedLen(n) {
		return nil, errB64
	}
	raw, err := b64.DecodeString(s)
	if err != nil || len(raw) != n || b64.EncodeToString(raw) != s {
		return nil, errB64
	}
	return raw, nil
}

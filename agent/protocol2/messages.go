package protocol2

import (
	"crypto/tls"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol"
)

const (
	// TLSHandshakeRecord est le premier octet d'un ClientHello TLS : il distingue wolagent/2 d'une
	// requête wolagent/1, qui commence par « { » (§3.1).
	TLSHandshakeRecord = 0x16
	// MaxHelloBytes borne la ligne de salut que l'agent envoie avant TLS (§3.1).
	MaxHelloBytes = 256
	// HandshakeTimeout borne l'établissement de TLS (§3.3).
	HandshakeTimeout = 10 * time.Second
	// IdleTimeout : l'agent ferme une connexion restée sans requête (§3.3).
	IdleTimeout = 30 * time.Second
	// MaxRequestsPerConnection borne les requêtes d'une connexion (§3.3).
	MaxRequestsPerConnection = 100
)

// Curves liste les échanges de clés de TLS, par ordre de préférence (§3.2) : hybride post-quantique
// quand les deux côtés le connaissent, sinon X25519 ou P-256.
var Curves = []tls.CurveID{tls.X25519MLKEM768, tls.X25519, tls.CurveP256}

// Request est une requête (§4.2). Les champs des commandes de wolagent/1 gardent leur nom ; « by »
// disparaît : le nom de l'appareil vient du registre de l'agent.
type Request struct {
	ID  int64  `json:"id"`
	Cmd string `json:"cmd"`
	// Commandes de wolagent/1. Force sert aussi, sur le PC seulement, à confirmer le retrait ou la
	// rétrogradation du dernier administrateur (§8).
	Delay *int    `json:"delay,omitempty"`
	Force *bool   `json:"force,omitempty"`
	Wakes []int64 `json:"wakes,omitempty"`
	Day   string  `json:"day,omitempty"`
	// Administration et appairage (§6).
	Device string `json:"device,omitempty"` // appareil visé (devices.*) ou seul autorisé (invite)
	Role   string `json:"role,omitempty"`   // rôle donné (devices.role, invite)
	Name   string `json:"name,omitempty"`   // nom de l'appareil (devices.rename, invite, pair, migrate)
	TTL    int64  `json:"ttl,omitempty"`    // validité de l'invitation, en secondes
	Invite string `json:"invite,omitempty"` // identifiant de l'invitation (invites.cancel)
	Code   string `json:"code,omitempty"`   // code d'appairage (pair)
	Nonce  string `json:"nonce,omitempty"`  // migrate, première étape
	Proof  string `json:"proof,omitempty"`  // migrate, seconde étape
}

// Response est la réponse à un appareil enregistré (§4.3) : celle de wolagent/1, plus l'identifiant
// de la requête et les champs de wolagent/2.
type Response struct {
	ID int64 `json:"id"`
	protocol.ResponseBody
	// Role est le rôle de l'appareil qui demande.
	Role string `json:"role,omitempty"`
	// Device est l'empreinte de l'appareil qui vient d'être enregistré (pair, migrate).
	Device  string       `json:"device,omitempty"`
	Devices []DeviceInfo `json:"devices,omitzero"`
	Invite  *InviteInfo  `json:"invite,omitempty"`
	Invites []InviteInfo `json:"invites,omitzero"`
	// Nonce et Proof : première étape de la migration (§9), envoyée sous la forme d'une Reply.
	Nonce string `json:"nonce,omitempty"`
	Proof string `json:"proof,omitempty"`
}

// Reply est la seule forme de réponse faite à une identité inconnue (§4.3) : ni nom du PC, ni
// système, ni version. Nonce et Proof portent la première étape de la migration (§9).
type Reply struct {
	ID      int64  `json:"id"`
	OK      bool   `json:"ok"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Nonce   string `json:"nonce,omitempty"`
	Proof   string `json:"proof,omitempty"`
}

// DeviceInfo décrit un appareil enregistré (commande devices).
type DeviceInfo struct {
	Device string `json:"device"`
	Name   string `json:"name"`
	Role   string `json:"role"`
	Added  int64  `json:"added"`
	Via    string `json:"via"` // pair, migrate ou local
	Seen   int64  `json:"seen,omitempty"`
	Self   bool   `json:"self,omitempty"`
}

// InviteInfo décrit une invitation. Code et Agent ne figurent que dans la réponse à invite : l'agent
// ne garde que le condensé du code.
type InviteInfo struct {
	ID      string `json:"id"`
	Code    string `json:"code,omitempty"`
	Role    string `json:"role"`
	Name    string `json:"name,omitempty"`
	Expires int64  `json:"expires"`
	Device  string `json:"device,omitempty"`
	Agent   string `json:"agent,omitempty"`
}

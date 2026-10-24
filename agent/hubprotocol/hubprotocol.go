// Package hubprotocol est l'implémentation de référence des fonctions pures de « patronus-hub/1 », le
// protocole entre les applications et le hub (docs/PROTOCOLE-HUB.md) : même transport que wolagent/2
// (salut, TLS 1.3 à authentification mutuelle, trames), avec son propre protocole applicatif, son port,
// ses rôles, ses commandes et son lien d'appairage.
//
// Les vecteurs de test protocol/hub-vectors.json sont produits par ce paquet
// (go test ./hubprotocol -run TestVectors -update).
package hubprotocol

import "github.com/slaynaw/wakeonlan/agent/protocol2"

const (
	// ALPN est le protocole applicatif annoncé dans TLS et dans le salut (§2).
	ALPN = "patronus-hub/1"
	// DefaultPort est le port du hub, à côté de celui de l'agent (9770) sur le même NAS.
	DefaultPort = 9771
	// LinkPrefix commence tout lien d'appairage du hub (casse indifférente, §7).
	LinkPrefix = "patronus://hub?"
	// MaxHubNameLength borne le nom du hub (« Hub NAS »), comme celui d'un PC dans un lien.
	MaxHubNameLength = protocol2.MaxPCNameLength
	// DefaultInviteRole est le rôle d'une invitation qui n'en précise pas : le plus faible (§5).
	DefaultInviteRole = protocol2.RoleView
)

// Hello est la ligne de salut du hub, envoyée avant TLS (§2) ; le blocage d'une adresse s'annonce comme
// chez l'agent : {"error":"rate_limited"}.
const Hello = `{"proto":"` + ALPN + `"}`

// LinkFormat est la famille des liens d'appairage du hub : paramètres et règles du lien v=2 d'un agent
// (docs/PROTOCOLE-V2.md, §7.1), début « patronus://hub? », version 1, port 9771 par défaut. « a » y
// porte l'empreinte du hub.
var LinkFormat = protocol2.LinkFormat{Prefix: LinkPrefix, Version: "1", DefaultPort: DefaultPort}

// ParseLink lit un lien d'appairage du hub ; un rôle annoncé doit être un rôle du hub.
func ParseLink(text string) (protocol2.Link, error) {
	l, err := protocol2.ParseLinkWith(text, LinkFormat)
	if err != nil {
		return protocol2.Link{}, err
	}
	if l.Role != 0 && !ValidRole(l.Role) {
		return protocol2.Link{}, &protocol2.LinkError{Field: "r"}
	}
	return l, nil
}

// Roles liste les rôles d'un appareil sur le hub (§5), du plus faible au plus fort. « control »
// n'existe pas : le hub ne relaie aucune commande vers les PC.
var Roles = []protocol2.Role{protocol2.RoleView, protocol2.RoleWake, protocol2.RoleAdmin}

// ValidRole indique un rôle du hub.
func ValidRole(r protocol2.Role) bool {
	return r == protocol2.RoleView || r == protocol2.RoleWake || r == protocol2.RoleAdmin
}

// ParseRole lit le nom d'un rôle du hub (« view », « wake », « admin »).
func ParseRole(name string) (protocol2.Role, bool) {
	r, ok := protocol2.ParseRole(name)
	return r, ok && ValidRole(r)
}

// Commandes propres au hub (§6). Celles des appareils autorisés et des invitations gardent leur nom de
// wolagent/2.
const (
	CmdStatus       = "hub.status"
	CmdEvents       = "hub.events"
	CmdHistory      = "hub.history"
	CmdMetrics      = "hub.metrics"
	CmdWake         = "hub.wake"
	CmdPCs          = "hub.pcs"
	CmdPCsAdd       = "hub.pcs.add"
	CmdPCsUpdate    = "hub.pcs.update"
	CmdPCsRemove    = "hub.pcs.remove"
	CmdSchedules    = "hub.schedules"
	CmdSchedulesSet = "hub.schedules.set"
	CmdSettings     = "hub.settings"
)

// Commands liste les commandes du protocole, appairage compris, dans l'ordre de la spécification (§6).
var Commands = []string{
	CmdStatus, CmdEvents, CmdHistory, CmdMetrics, CmdWake,
	CmdPCs, CmdPCsAdd, CmdPCsUpdate, CmdPCsRemove, CmdSchedules, CmdSchedulesSet, CmdSettings,
	protocol2.CmdDevices, protocol2.CmdDevicesRevoke, protocol2.CmdDevicesRole, protocol2.CmdDevicesRename,
	protocol2.CmdInvite, protocol2.CmdInvites, protocol2.CmdInvitesCancel,
	protocol2.CmdPair,
}

// minRoles donne le rôle minimal de chaque commande (§5) ; pair est réservée aux identités inconnues.
var minRoles = map[string]protocol2.Role{
	CmdStatus: protocol2.RoleView, CmdEvents: protocol2.RoleView, CmdHistory: protocol2.RoleView, CmdMetrics: protocol2.RoleView,
	CmdWake: protocol2.RoleWake,
	CmdPCs:  protocol2.RoleAdmin, CmdPCsAdd: protocol2.RoleAdmin, CmdPCsUpdate: protocol2.RoleAdmin, CmdPCsRemove: protocol2.RoleAdmin,
	CmdSchedules: protocol2.RoleAdmin, CmdSchedulesSet: protocol2.RoleAdmin, CmdSettings: protocol2.RoleAdmin,
	protocol2.CmdDevices: protocol2.RoleAdmin, protocol2.CmdDevicesRevoke: protocol2.RoleAdmin,
	protocol2.CmdDevicesRole: protocol2.RoleAdmin, protocol2.CmdDevicesRename: protocol2.RoleAdmin,
	protocol2.CmdInvite: protocol2.RoleAdmin, protocol2.CmdInvites: protocol2.RoleAdmin,
	protocol2.CmdInvitesCancel: protocol2.RoleAdmin,
}

// MinRole renvoie le rôle minimal d'une commande ; false pour une commande inconnue ou pour pair.
func MinRole(cmd string) (protocol2.Role, bool) {
	r, ok := minRoles[cmd]
	return r, ok
}

// Authorize applique les vérifications du hub (§5), dans l'ordre : identité enregistrée (sinon seule pair
// passe), commande connue, rôle suffisant. Elle renvoie protocol2.CodeOK, CodeUnknownDevice,
// CodeAlreadyPaired, CodeUnsupported ou CodeInsufficientRole.
func Authorize(registered bool, role protocol2.Role, cmd string) string {
	switch {
	case !registered && cmd == protocol2.CmdPair:
		return protocol2.CodeOK
	case !registered:
		return protocol2.CodeUnknownDevice
	case cmd == protocol2.CmdPair:
		return protocol2.CodeAlreadyPaired
	}
	minimum, known := minRoles[cmd]
	switch {
	case !known:
		return protocol2.CodeUnsupported
	case !ValidRole(role) || role < minimum:
		return protocol2.CodeInsufficientRole
	}
	return protocol2.CodeOK
}

// Request est une requête au hub (§4).
type Request struct {
	ID  int64  `json:"id"`
	Cmd string `json:"cmd"`
	// Appareils autorisés, invitations et appairage, comme en wolagent/2 (docs/PROTOCOLE-V2.md, §6.2).
	Device string `json:"device,omitempty"`
	Role   string `json:"role,omitempty"`
	Name   string `json:"name,omitempty"`
	TTL    int64  `json:"ttl,omitempty"`
	Invite string `json:"invite,omitempty"`
	Code   string `json:"code,omitempty"`
	// Force confirme, sur le NAS seulement, le retrait ou la rétrogradation du dernier administrateur.
	Force *bool `json:"force,omitempty"`
	// PC confiés au hub (hub.pcs.*) : PC visé (identifiant du hub), puis ce qui le décrit. Code porte
	// alors l'invitation de l'agent, réservée au hub.
	PC    string `json:"pc,omitempty"`
	Host  string `json:"host,omitempty"`
	Port  int    `json:"port,omitempty"`
	MAC   string `json:"mac,omitempty"`
	Agent string `json:"agent,omitempty"`
	Probe *Probe `json:"probe,omitempty"`
}

// Sondes des appareils sans agent (§6.2).
const (
	ProbeTCP   = "tcp"
	ProbeHTTP  = "http"
	ProbeHTTPS = "https"
)

// Probe est la sonde d'un appareil sans agent : connexion TCP à un port, ou page web (seule compte la
// réponse, jamais son contenu). Port vaut 80 ou 443 par défaut pour une page web.
type Probe struct {
	Kind string `json:"kind"`
	Port int    `json:"port,omitempty"`
}

// MaxPCs borne le nombre de PC et d'appareils confiés au hub.
const MaxPCs = 64

// PCInfo décrit un PC confié au hub, ou un appareil sans agent (Agent vide, Probe renseignée).
type PCInfo struct {
	ID    string `json:"id"` // identifiant propre au hub : 16 octets aléatoires en B64
	Name  string `json:"name"`
	Host  string `json:"host"`
	Port  int    `json:"port,omitempty"` // port de l'agent
	MAC   string `json:"mac,omitempty"`
	Agent string `json:"agent,omitempty"` // empreinte de l'agent
	// Role est le rôle du hub sur l'agent : view, wake, ou control pour une extinction programmée.
	Role  string `json:"role,omitempty"`
	Probe *Probe `json:"probe,omitempty"`
	Added int64  `json:"added"`
}

// Response est la réponse du hub à un appareil enregistré (§4). Une identité inconnue ne reçoit qu'une
// protocol2.Reply.
type Response struct {
	ID      int64  `json:"id"`
	OK      bool   `json:"ok"`
	Code    string `json:"code"`
	Message string `json:"message,omitempty"`
	// Role est le rôle de l'appareil qui demande.
	Role string `json:"role,omitempty"`
	// Device est l'empreinte de l'appareil qui vient d'être enregistré (pair).
	Device  string                 `json:"device,omitempty"`
	Devices []protocol2.DeviceInfo `json:"devices,omitzero"`
	Invite  *InviteInfo            `json:"invite,omitempty"`
	Invites []protocol2.InviteInfo `json:"invites,omitzero"`
	Hub     *Info                  `json:"hub,omitempty"`
	PCs     []PCInfo               `json:"pcs,omitzero"`
	PC      *PCInfo                `json:"pc,omitempty"`
}

// InviteInfo est l'invitation créée par invite : celle de wolagent/2, avec l'empreinte du hub (« hub »)
// au lieu de celle d'un agent.
type InviteInfo struct {
	ID      string `json:"id"`
	Code    string `json:"code,omitempty"`
	Role    string `json:"role"`
	Name    string `json:"name,omitempty"`
	Expires int64  `json:"expires"`
	Device  string `json:"device,omitempty"`
	Hub     string `json:"hub,omitempty"`
}

// Info décrit le hub (hub.status).
type Info struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Pin     string `json:"pin"`     // empreinte du hub (B64)
	Started int64  `json:"started"` // démarrage du hub, secondes Unix
}

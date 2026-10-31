// Package hubprotocol est l'implémentation de référence des fonctions pures de « patronus-hub/1 », le
// protocole entre les applications et le hub (docs/PROTOCOLE-HUB.md) : même transport que wolagent/2
// (salut, TLS 1.3 à authentification mutuelle, trames), avec son propre protocole applicatif, son port,
// ses rôles, ses commandes et son lien d'appairage.
//
// Les vecteurs de test protocol/hub-vectors.json sont produits par ce paquet
// (go test ./hubprotocol -run TestVectors -update).
package hubprotocol

import (
	"errors"
	"regexp"
	"slices"

	"github.com/slaynaw/wakeonlan/agent/protocol"
	"github.com/slaynaw/wakeonlan/agent/protocol2"
)

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
	// Archive (hub.history, hub.metrics) : début des évènements voulus (secondes Unix), jour des mesures.
	From int64  `json:"from,omitempty"`
	Day  string `json:"day,omitempty"`
	// Réglages (hub.settings) : durée de conservation, en jours, et fuseau horaire des programmations ;
	// absents, les réglages sont seulement lus.
	Retention int    `json:"retention,omitempty"`
	Timezone  string `json:"timezone,omitempty"`
	// Schedule : hub.schedules.set, programmation du PC (nil : plus aucune).
	Schedule *Schedule `json:"schedule,omitempty"`
	// Since : hub.events, début des évènements voulus (secondes Unix).
	Since int64 `json:"since,omitempty"`
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
	// States : hub.status, état de chaque PC confié et appareil, dans l'ordre de PCs.
	States []PCState `json:"states,omitzero"`
	// History : hub.history, évènements gardés du journal de l'agent (From : le plus ancien gardé) ; More
	// indique qu'il en reste après le dernier, à demander avec un « from » plus grand.
	History *protocol.History `json:"history,omitempty"`
	More    bool              `json:"more,omitempty"`
	// Metrics : hub.metrics, jours gardés (Days) ou mesures d'un jour (Day, Rows), comme « metrics ».
	Metrics *protocol.Metrics `json:"metrics,omitempty"`
	// Settings : hub.settings.
	Settings *Settings `json:"settings,omitempty"`
	// Events : hub.events, dans l'ordre (More : il en reste après le dernier).
	Events []Event `json:"events,omitzero"`
	// Schedules : hub.schedules, programmation de chaque PC qui en a une ; Schedule : hub.schedules.set.
	Schedules []PCSchedule `json:"schedules,omitzero"`
	Schedule  *PCSchedule  `json:"schedule,omitempty"`
}

// Slot est une programmation (§6.7) : jours ISO (1 = lundi … 7 = dimanche) et heure « HH:MM » dans le
// fuseau horaire du hub. Next, dans les réponses seulement, est la prochaine occurrence (secondes Unix).
type Slot struct {
	On   bool   `json:"on"`
	Days []int  `json:"days"`
	At   string `json:"at"`
	Next int64  `json:"next,omitempty"`
}

// Schedule est la programmation d'un PC : réveil, extinction si inactif (nil : aucune).
type Schedule struct {
	Wake     *Slot `json:"wake,omitempty"`
	Shutdown *Slot `json:"shutdown,omitempty"`
}

// PCSchedule est la programmation du PC d'identifiant PC.
type PCSchedule struct {
	PC       string `json:"pc"`
	Wake     *Slot  `json:"wake,omitempty"`
	Shutdown *Slot  `json:"shutdown,omitempty"`
}

var slotTime = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

// ValidSlot vérifie une programmation : au moins un jour de 1 à 7, sans doublon, et une heure « HH:MM ».
func ValidSlot(s Slot) error {
	if !slotTime.MatchString(s.At) {
		return errors.New("heure invalide : HH:MM attendu")
	}
	if len(s.Days) == 0 || len(s.Days) > 7 {
		return errors.New("jours invalides : de 1 (lundi) à 7 (dimanche), au moins un")
	}
	seen := map[int]bool{}
	for _, d := range s.Days {
		if d < 1 || d > 7 || seen[d] {
			return errors.New("jours invalides : de 1 (lundi) à 7 (dimanche), sans doublon")
		}
		seen[d] = true
	}
	return nil
}

// Évènements détectés par le hub (§6.5). Ce sont des codes : les applications rédigent elles-mêmes le
// texte, avec leurs propres noms de PC (T38).
const (
	EventOnline    = "online"     // de nouveau en ligne
	EventOffline   = "offline"    // plus rien ne répond
	EventAgentDown = "agent_down" // agent arrêté depuis plus de 2 minutes, PC allumé
	EventProblem   = "problem"    // refus d'un agent (X : revoked, pin_mismatch, rate_limited)
	EventLost      = "lost"       // arrêt inattendu noté par l'agent (X : cause, bsod, button, power, hardware)
	EventDisk      = "disk"       // disque à surveiller (X : health) ou lecteur plein à 90 % (X : full)
	EventMeasure   = "measure"    // mesure grave pendant plus de 5 minutes (X : identifiant de la mesure)
	EventWake      = "wake"       // paquet magique envoyé par le hub (I : appareil à l'origine, « local » ou « schedule »)
	EventShutdown  = "shutdown"   // extinction programmée (X : done, postponed, refused)
)

// Event est un évènement détecté par le hub.
type Event struct {
	T  int64  `json:"t"`           // heure, secondes Unix (horloge du hub)
	PC string `json:"pc"`          // identifiant du PC
	K  string `json:"k"`           // type (Event…)
	X  string `json:"x,omitempty"` // précision : un code ou un identifiant de mesure, jamais du texte libre
	I  string `json:"i,omitempty"` // empreinte courte de l'appareil à l'origine (réveil)
}

// Précisions des évènements (Event.X) que le hub donne aux applications.
const (
	LostBSOD     = "bsod"     // arrêt inattendu : plantage (écran bleu)
	LostButton   = "button"   // arrêt forcé au bouton d'alimentation
	LostPower    = "power"    // coupure de courant ou blocage
	LostHardware = "hardware" // erreur matérielle fatale
	DiskHealth   = "health"   // disque à surveiller
	DiskFull     = "full"     // lecteur plein à 90 %
	WakeLocal    = "local"    // Event.I d'un réveil : demandé sur le NAS
	WakeSchedule = "schedule" // Event.I d'un réveil : programmation (§6.7)
	// Extinction programmée (§6.7) : demandée à l'agent, renoncée (PC encore utilisé au bout de 2 heures),
	// refusée par l'agent (rôle control absent).
	ShutdownDone      = "done"
	ShutdownPostponed = "postponed"
	ShutdownRefused   = "refused"
)

var (
	measureID = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,23}$`)
	shortPin  = regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{4}-[0-9A-HJKMNP-TV-Z]{4}-[0-9A-HJKMNP-TV-Z]{4}$`)
	// eventDetails : précisions admises pour chaque type d'évènement ; nil : aucune.
	eventDetails = map[string][]string{
		EventOnline: nil, EventOffline: nil, EventAgentDown: nil, EventMeasure: nil, EventWake: nil,
		EventProblem:  {ProblemRevoked, ProblemPinMismatch, ProblemBlocked},
		EventLost:     {"", LostBSOD, LostButton, LostPower, LostHardware},
		EventDisk:     {DiskHealth, DiskFull},
		EventShutdown: {ShutdownDone, ShutdownPostponed, ShutdownRefused},
	}
)

// ValidEvent indique qu'un évènement n'est fait que de codes connus (§6.5). Les applications écartent les
// autres sans rien en afficher : un hub compromis pourrait y glisser du texte ou un lien (T38).
func ValidEvent(e Event) bool {
	allowed, known := eventDetails[e.K]
	switch {
	case !known || e.T <= 0 || e.PC == "":
		return false
	case e.K == EventMeasure:
		return measureID.MatchString(e.X) && e.I == ""
	case e.K == EventWake:
		return e.X == "" && (e.I == WakeLocal || e.I == WakeSchedule || shortPin.MatchString(e.I))
	case allowed == nil:
		return e.X == "" && e.I == ""
	}
	return slices.Contains(allowed, e.X) && e.I == ""
}

// Settings sont les réglages du hub.
type Settings struct {
	Retention int    `json:"retention"`          // durée de conservation de l'archive, en jours (90 à 1095)
	Timezone  string `json:"timezone,omitempty"` // fuseau horaire des programmations (nom IANA)
}

// États d'un PC vus par le hub (§6.2).
const (
	StateUnknown   = "unknown"    // pas encore relevé
	StateOnline    = "online"     // l'agent répond, ou l'appareil répond à sa sonde
	StateOffline   = "offline"    // rien ne répond
	StateAgentDown = "agent_down" // le PC refuse la connexion au port de l'agent : il est allumé, l'agent non
)

// Problèmes du hub avec un agent (PCState.Problem) : le PC est joint, mais pas relevé.
const (
	ProblemRevoked     = "revoked"      // l'agent ne connaît plus le hub (retiré de ses appareils autorisés)
	ProblemPinMismatch = "pin_mismatch" // l'agent présente une autre empreinte (réinstallé, ou intermédiaire)
	ProblemBlocked     = "rate_limited" // l'agent bloque l'adresse du hub
)

// PCState est l'état d'un PC confié au hub ou d'un appareil sans agent.
type PCState struct {
	ID      string `json:"id"`
	State   string `json:"state"`
	Since   int64  `json:"since,omitempty"`   // début de cet état, secondes Unix
	Checked int64  `json:"checked,omitempty"` // dernier relevé
	Latency int64  `json:"latency,omitempty"` // durée du dernier relevé réussi, en millisecondes
	Problem string `json:"problem,omitempty"`
	// Agent est la dernière réponse de l'agent à « status » (état, températures, disques, mesures), lue au
	// relevé Checked quand State vaut online.
	Agent *protocol.ResponseBody `json:"agent,omitempty"`
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

package protocol2

import "fmt"

// Role est le rôle d'un appareil enregistré (§5). Les rôles sont cumulatifs : chacun permet aussi tout
// ce que permettent les rôles inférieurs.
type Role uint8

const (
	RoleView    Role = iota + 1 // consulter : état, journal, mesures, fiche du PC
	RoleWake                    // démarrer : signaler les démarrages demandés
	RoleControl                 // contrôler : éteindre, redémarrer, mettre en veille
	RoleAdmin                   // administrer : appareils autorisés et invitations
)

var roleNames = [...]string{RoleView: "view", RoleWake: "wake", RoleControl: "control", RoleAdmin: "admin"}

// Roles liste les rôles, du plus faible au plus fort.
var Roles = []Role{RoleView, RoleWake, RoleControl, RoleAdmin}

// ParseRole lit le nom d'un rôle (« view », « wake », « control », « admin »).
func ParseRole(name string) (Role, bool) {
	for _, r := range Roles {
		if roleNames[r] == name {
			return r, true
		}
	}
	return 0, false
}

// String renvoie le nom du rôle, ou "" pour une valeur qui n'est pas un rôle.
func (r Role) String() string {
	if r < RoleView || r > RoleAdmin {
		return ""
	}
	return roleNames[r]
}

// MarshalText et UnmarshalText écrivent le rôle par son nom dans le JSON.
func (r Role) MarshalText() ([]byte, error) {
	if r.String() == "" {
		return nil, fmt.Errorf("wolagent/2 : rôle %d inconnu", r)
	}
	return []byte(r.String()), nil
}

func (r *Role) UnmarshalText(text []byte) error {
	parsed, ok := ParseRole(string(text))
	if !ok {
		return fmt.Errorf("wolagent/2 : rôle %q inconnu", text)
	}
	*r = parsed
	return nil
}

// Commandes ajoutées par wolagent/2 (§6). Celles de wolagent/1 gardent leur nom.
const (
	CmdDevices       = "devices"
	CmdDevicesRevoke = "devices.revoke"
	CmdDevicesRole   = "devices.role"
	CmdDevicesRename = "devices.rename"
	CmdInvite        = "invite"
	CmdInvites       = "invites"
	CmdInvitesCancel = "invites.cancel"
	CmdPair          = "pair"
	CmdMigrate       = "migrate"
)

// minRoles donne le rôle minimal de chaque commande (§5). pair et migrate n'y figurent pas : elles sont
// réservées aux identités inconnues.
var minRoles = map[string]Role{
	"status": RoleView, "history": RoleView, "metrics": RoleView, "specs": RoleView,
	"wakes":    RoleWake,
	"shutdown": RoleControl, "reboot": RoleControl, "sleep": RoleControl,
	CmdDevices: RoleAdmin, CmdDevicesRevoke: RoleAdmin, CmdDevicesRole: RoleAdmin, CmdDevicesRename: RoleAdmin,
	CmdInvite: RoleAdmin, CmdInvites: RoleAdmin, CmdInvitesCancel: RoleAdmin,
}

// Commands liste les commandes connues, appairage compris, dans l'ordre de la spécification (§5).
var Commands = []string{
	"status", "history", "metrics", "specs", "wakes", "shutdown", "reboot", "sleep",
	CmdDevices, CmdDevicesRevoke, CmdDevicesRole, CmdDevicesRename, CmdInvite, CmdInvites, CmdInvitesCancel,
	CmdPair, CmdMigrate,
}

// MinRole renvoie le rôle minimal d'une commande ; false pour une commande inconnue ou d'appairage.
func MinRole(cmd string) (Role, bool) {
	r, ok := minRoles[cmd]
	return r, ok
}

// IsEnrollment indique les commandes d'appairage (pair, migrate), réservées aux identités inconnues.
func IsEnrollment(cmd string) bool { return cmd == CmdPair || cmd == CmdMigrate }

// Authorize applique les vérifications 1 à 3 de l'agent (§5) à une commande reçue d'une identité
// enregistrée avec le rôle role, ou inconnue (registered faux, role ignoré). Elle renvoie CodeOK (la
// commande passe ensuite par la liste blanche du PC), CodeUnknownDevice, CodeAlreadyPaired,
// CodeUnsupported ou CodeInsufficientRole.
func Authorize(registered bool, role Role, cmd string) string {
	switch {
	case !registered && IsEnrollment(cmd):
		return CodeOK
	case !registered:
		return CodeUnknownDevice
	case IsEnrollment(cmd):
		return CodeAlreadyPaired
	}
	minimum, known := minRoles[cmd]
	switch {
	case !known:
		return CodeUnsupported
	case role < minimum || role > RoleAdmin:
		return CodeInsufficientRole
	}
	return CodeOK
}

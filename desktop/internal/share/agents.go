package share

import (
	"slices"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol2"
)

// Partage v2, côté propriétaire : ce que l'application, administratrice d'un PC, y fait pour les
// personnes autorisées (docs/PARTAGE.md, « Partage v2 ») : invitations réservées à leur appareil,
// alignement des rôles, retraits. Les décisions sont pures (PlanAgent) ; l'application les exécute
// sur une session d'administration, puis enregistre le résultat (ApplyAgent).
//
// L'application ne touche jamais à son propre enregistrement ni à celui d'un administrateur : ils
// ne relèvent pas du partage.

const (
	// InviteTTL est la validité des invitations du partage : 7 jours, le maximum de l'agent.
	InviteTTL = protocol2.MaxInviteTTL * time.Second
	// InviteRenewal : une invitation qui expire dans moins de ce délai est remplacée.
	InviteRenewal = 24 * time.Hour
)

// AllowedRight renvoie le droit effectivement accordé sur un PC : « démarrer et consulter » demande
// le partage v2 (demande avec empreinte, agent joint par la sienne) ; « démarrer et éteindre », le
// partage v2 ou la clé de l'agent (partage v1). À défaut : « démarrer ».
func AllowedRight(r Right, identity, pinned, hasKey bool) Right {
	v2 := identity && pinned
	switch {
	case r == RightView && v2, r == RightFull && (v2 || hasKey):
		return r
	}
	return RightWake
}

// V2 indique que le PC se partage avec cette personne en v2 (enregistrement par l'agent).
func (p Person) V2(pinned bool) bool { return p.Identity != "" && pinned }

// NeedsKey indique que la clé wolagent/1 du PC est transmise à quelqu'un (partage v1 avec le droit
// « démarrer et éteindre ») : l'application doit alors la garder.
func (o *Owner) NeedsKey(device string, pinned bool) bool {
	return slices.ContainsFunc(o.People, func(p Person) bool { return p.Rights[device] == RightFull && !p.V2(pinned) })
}

// want est l'enregistrement voulu d'une empreinte sur un PC : le rôle le plus fort parmi les
// personnes qui la portent (une même empreinte peut figurer dans deux demandes).
type want struct {
	identity string
	role     protocol2.Role
	people   []Person
}

// wanted renvoie les enregistrements voulus sur un PC, par empreinte, dans l'ordre des personnes.
func (o *Owner) wanted(device string) []want {
	var out []want
	for _, p := range o.People {
		role, ok := protocol2.ParseRole(p.Rights[device].AgentRole())
		if p.Identity == "" || !ok {
			continue
		}
		i := slices.IndexFunc(out, func(w want) bool { return w.identity == p.Identity })
		if i < 0 {
			out = append(out, want{identity: p.Identity})
			i = len(out) - 1
		}
		out[i].role = max(out[i].role, role)
		out[i].people = append(out[i].people, p)
	}
	return out
}

// pairingOf renvoie l'index de l'appairage d'une personne sur un PC (-1 sinon).
func (o *Owner) pairingOf(person, device string) int {
	return slices.IndexFunc(o.Pairings, func(x Pairing) bool { return x.Person == person && x.Device == device })
}

// AgentWork indique s'il y a quelque chose à faire sur un PC (joint en wolagent/2) : un retrait, une
// invitation à créer ou à renouveler. poll : des invitations attendent d'être utilisées ; l'état de
// l'agent mérite d'être relu de temps en temps pour le constater.
func (o *Owner) AgentWork(device string, now time.Time) (work, poll bool) {
	if slices.ContainsFunc(o.Revocations, func(r Revocation) bool { return r.Device == device }) {
		return true, false
	}
	for _, w := range o.wanted(device) {
		for _, p := range w.people {
			i := o.pairingOf(p.Device, device)
			if i < 0 {
				return true, false
			}
			x := o.Pairings[i]
			switch {
			case x.Identity != w.identity:
				return true, false
			case x.Registered:
				// Un administrateur garde son rôle : le partage ne le change pas.
				if x.Role != w.role.String() && x.Role != protocol2.RoleAdmin.String() {
					return true, false
				}
			case x.Code == "" || x.Role != w.role.String() || time.Unix(x.Expires, 0).Before(now.Add(InviteRenewal)):
				return true, false
			default:
				poll = true
			}
		}
	}
	return false, poll
}

// AgentView est l'état d'un agent vu par un administrateur.
type AgentView struct {
	Devices []protocol2.DeviceInfo
	Invites []protocol2.InviteInfo
}

// RoleChange associe une empreinte à un rôle : changement à demander à l'agent, ou enregistrement
// constaté.
type RoleChange struct {
	Identity string `json:"identity"`
	Role     string `json:"role"`
}

// InviteRequest est une invitation à créer, réservée à une empreinte.
type InviteRequest struct {
	Identity string `json:"identity"`
	Role     string `json:"role"`
	Name     string `json:"name"`
}

// AgentPlan est ce qu'il faut faire sur un PC, dans l'ordre : annulations d'invitations, retraits,
// changements de rôle, nouvelles invitations.
type AgentPlan struct {
	Cancel []string        `json:"cancel"`
	Revoke []string        `json:"revoke"`
	Roles  []RoleChange    `json:"roles"`
	Invite []InviteRequest `json:"invite"`
	// Registered : empreintes enregistrées par le PC, avec leur rôle une fois le plan exécuté.
	Registered []RoleChange `json:"registered"`
	// Done : retraits soldés une fois les annulations et les retraits faits.
	Done []Revocation `json:"done"`
	// Skipped : empreintes que le partage ne touche pas (cet appareil, un administrateur).
	Skipped []string `json:"skipped"`
}

// Empty indique qu'il n'y a rien à demander à l'agent (l'état peut tout de même changer).
func (p AgentPlan) Empty() bool {
	return len(p.Cancel) == 0 && len(p.Revoke) == 0 && len(p.Roles) == 0 && len(p.Invite) == 0
}

// PlanAgent décide des opérations sur un PC d'après l'état de son agent (view) :
//   - retraits en file : invitations réservées à l'empreinte annulées, enregistrement supprimé ;
//   - empreinte voulue et enregistrée : rôle aligné, invitations devenues inutiles annulées ;
//   - empreinte voulue, pas enregistrée : l'invitation connue est gardée si elle est encore en
//     attente, au bon rôle et valable assez longtemps ; sinon les invitations réservées à cette
//     empreinte sont annulées et une nouvelle est créée.
func (o *Owner) PlanAgent(device string, view AgentView, now time.Time) AgentPlan {
	plan := AgentPlan{Cancel: []string{}, Revoke: []string{}, Roles: []RoleChange{}, Invite: []InviteRequest{},
		Registered: []RoleChange{}, Done: []Revocation{}, Skipped: []string{}}
	registered := map[string]protocol2.DeviceInfo{}
	for _, d := range view.Devices {
		registered[d.Device] = d
	}
	protected := func(identity string) bool {
		d, ok := registered[identity]
		return ok && (d.Self || d.Role == protocol2.RoleAdmin.String())
	}
	cancel := func(identity string) {
		for _, inv := range view.Invites {
			if inv.Device == identity && !slices.Contains(plan.Cancel, inv.ID) {
				plan.Cancel = append(plan.Cancel, inv.ID)
			}
		}
	}
	for _, r := range o.Revocations {
		if r.Device != device || o.wants(device, r.Identity) {
			continue
		}
		cancel(r.Identity)
		switch _, ok := registered[r.Identity]; {
		case protected(r.Identity):
			plan.Skipped = append(plan.Skipped, r.Identity)
		case ok:
			plan.Revoke = append(plan.Revoke, r.Identity)
		}
		plan.Done = append(plan.Done, r)
	}
	for _, w := range o.wanted(device) {
		if d, ok := registered[w.identity]; ok {
			cancel(w.identity)
			role := d.Role
			if protected(w.identity) {
				plan.Skipped = append(plan.Skipped, w.identity)
			} else if role != w.role.String() {
				plan.Roles = append(plan.Roles, RoleChange{Identity: w.identity, Role: w.role.String()})
				role = w.role.String()
			}
			plan.Registered = append(plan.Registered, RoleChange{Identity: w.identity, Role: role})
			continue
		}
		if o.inviteValid(device, w, view, now) {
			continue
		}
		cancel(w.identity)
		plan.Invite = append(plan.Invite, InviteRequest{Identity: w.identity, Role: w.role.String(), Name: w.people[0].Name})
	}
	return plan
}

// inviteValid indique que l'invitation connue pour une empreinte peut servir encore : en attente sur
// l'agent, réservée à cette empreinte, au rôle voulu, valable au moins InviteRenewal, et connue de
// chaque personne qui porte l'empreinte.
func (o *Owner) inviteValid(device string, w want, view AgentView, now time.Time) bool {
	var id string
	for _, p := range w.people {
		i := o.pairingOf(p.Device, device)
		if i < 0 {
			return false
		}
		x := o.Pairings[i]
		if x.Identity != w.identity || x.Registered || x.Code == "" || x.Role != w.role.String() ||
			time.Unix(x.Expires, 0).Before(now.Add(InviteRenewal)) || (id != "" && x.Invite != id) {
			return false
		}
		id = x.Invite
	}
	return slices.ContainsFunc(view.Invites, func(inv protocol2.InviteInfo) bool { return inv.ID == id && inv.Device == w.identity })
}

// PlanResult est le résultat de l'exécution d'un plan.
type PlanResult struct {
	// Changed : annulations, retraits et changements de rôle tous faits.
	Changed bool `json:"changed"`
	// Created : invitations créées (code compris), par empreinte.
	Created map[string]protocol2.InviteInfo `json:"created"`
}

// ApplyAgent enregistre le résultat d'un plan exécuté sur un PC. L'état a pu changer pendant les
// échanges : seules les personnes qui veulent encore ces enregistrements sont mises à jour, et une
// invitation créée pour une empreinte qui n'est plus voulue part dans la file des retraits. Renvoie
// true si l'état a changé.
func (o *Owner) ApplyAgent(device string, plan AgentPlan, res PlanResult) bool {
	before, revocations := slices.Clone(o.Pairings), slices.Clone(o.Revocations)
	if res.Changed {
		o.Revocations = slices.DeleteFunc(o.Revocations, func(r Revocation) bool { return slices.Contains(plan.Done, r) })
		for _, r := range plan.Registered {
			o.setPairings(device, r.Identity, Pairing{Role: r.Role, Registered: true})
		}
	}
	for _, req := range plan.Invite {
		inv, ok := res.Created[req.Identity]
		code, err := protocol2.NormalizeCode(inv.Code)
		if !ok || err != nil {
			continue
		}
		if !o.wants(device, req.Identity) {
			o.revoke(device, req.Identity)
			continue
		}
		o.setPairings(device, req.Identity, Pairing{Role: inv.Role, Invite: inv.ID, Code: code, Expires: inv.Expires})
	}
	return !slices.Equal(revocations, o.Revocations) || !slices.Equal(before, o.Pairings)
}

// setPairings écrit l'appairage d'une empreinte sur un PC pour chaque personne qui la porte et veut
// joindre ce PC par l'agent.
func (o *Owner) setPairings(device, identity string, x Pairing) {
	for _, p := range o.People {
		if p.Identity != identity || p.Rights[device].AgentRole() == "" {
			continue
		}
		x.Person, x.Device, x.Identity = p.Device, device, identity
		if i := o.pairingOf(p.Device, device); i >= 0 {
			o.Pairings[i] = x
		} else {
			o.Pairings = append(o.Pairings, x)
		}
	}
}

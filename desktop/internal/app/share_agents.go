package app

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol2"
	"github.com/slaynaw/wakeonlan/desktop/internal/agentclient"
	"github.com/slaynaw/wakeonlan/desktop/internal/diag"
	"github.com/slaynaw/wakeonlan/desktop/internal/share"
	"github.com/slaynaw/wakeonlan/desktop/internal/status"
)

// Partage v2, côté propriétaire (docs/PARTAGE.md, « Partage v2 ») : quand cet ordinateur administre
// un PC partagé, il y crée les invitations réservées aux appareils des personnes autorisées, aligne
// leurs rôles et applique les retraits, puis republie les fichiers d'accès. Les décisions sont
// celles de share.Owner.PlanAgent ; ici, seulement leur exécution, au plus une session à la fois
// par PC. Les journaux ne contiennent ni code d'invitation ni empreinte complète.

const (
	// shareAgentTimeout borne une session (lecture de l'état, puis quelques commandes).
	shareAgentTimeout = 30 * time.Second
	// shareAgentPoll : relecture de l'état d'un PC allumé dont des invitations attendent d'être
	// utilisées (une session légère : la liste des appareils et des invitations).
	shareAgentPoll = 2 * time.Minute
	// Délais entre deux essais après un échec : doublés à chaque fois, d'une minute à une heure.
	shareAgentRetryMin = time.Minute
	shareAgentRetryMax = time.Hour
)

// agentSync suit les sessions du partage v2 par PC (verrou propre, jamais pris en tenant sh.mu).
type agentSync struct {
	mu       sync.Mutex
	running  map[string]bool
	last     map[string]time.Time // dernière session réussie
	next     map[string]time.Time // pas d'essai avant (après un échec)
	failures map[string]int
	// notAdmin : PC à joindre pour le partage v2 dont cet ordinateur n'est pas administrateur.
	notAdmin map[string]bool
}

func newAgentSync() *agentSync {
	return &agentSync{running: map[string]bool{}, last: map[string]time.Time{}, next: map[string]time.Time{},
		failures: map[string]int{}, notAdmin: map[string]bool{}}
}

// isRunning indique qu'une session est en cours sur un PC.
func (a *agentSync) isRunning(id string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.running[id]
}

// notAdminOf indique les PC où le partage v2 attend que cet ordinateur en devienne administrateur.
func (a *agentSync) notAdminOf(id string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.notAdmin[id]
}

// considerShareAgent décide, d'après le dernier état d'un PC, s'il faut y mener une session du
// partage v2 : PC à soi joint en wolagent/2, allumé, et quelque chose à faire (ou à vérifier).
func (s *Service) considerShareAgent(id string, st status.DeviceStatus) {
	sh := s.sharing
	if sh == nil {
		return
	}
	d, ok := s.ownDevice(id)
	if !ok || d.Agent == nil || !d.Agent.HasPin() || !d.HasHost() || st.State != status.Online || st.Agent == nil ||
		st.Agent.Proto != protocol2.ALPN {
		return
	}
	now := s.now()
	sh.mu.Lock()
	var work, poll bool
	if o := sh.state.Owner; o != nil {
		work, poll = o.AgentWork(id, now)
	}
	sh.mu.Unlock()
	a := sh.agents
	a.mu.Lock()
	notAdmin := (work || poll) && st.Agent.Role != protocol2.RoleAdmin.String()
	flipped := a.notAdmin[id] != notAdmin
	a.notAdmin[id] = notAdmin
	start := !notAdmin && (work || poll) && !a.running[id] && !now.Before(a.next[id]) && (work || now.Sub(a.last[id]) >= shareAgentPoll)
	if start {
		a.running[id] = true
	}
	a.mu.Unlock()
	// L'interface est prévenue à la fin du relevé (onStatusChange).
	if flipped && notAdmin {
		diag.Info(areaShare, "[%s] : partage v2 en attente, cet ordinateur n'est pas administrateur du PC (rôle %s)",
			shortID(id), st.Agent.Role)
	}
	if !start {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), shareAgentTimeout)
		defer cancel()
		err := s.syncShareAgent(ctx, id)
		a.mu.Lock()
		delete(a.running, id)
		var wait time.Duration
		switch {
		case err == nil:
			delete(a.failures, id)
			delete(a.next, id)
			a.last[id] = s.now()
		case agentclient.ReasonOf(err) == protocol2.CodeInsufficientRole:
			a.notAdmin[id] = true
		default:
			a.failures[id]++
			wait = min(shareAgentRetryMax, shareAgentRetryMin<<min(a.failures[id]-1, 6))
			a.next[id] = s.now().Add(wait)
		}
		a.mu.Unlock()
		if err != nil {
			diag.Warn(areaShare, "[%s] : partage v2 : %s %s (%v), nouvel essai dans %s", shortID(id), agentclient.CodeOf(err),
				agentclient.ReasonOf(err), err, wait)
			s.notify()
		}
	}()
}

// ignorable : commande sans objet, la cible ayant disparu entre la lecture de l'état et la commande
// (invitation utilisée ou expirée, appareil déjà retiré) ; l'état suivant le dira.
func ignorable(err error) bool {
	return agentclient.ReasonOf(err) == protocol2.CodeBadRequest
}

// syncShareAgent mène une session du partage v2 sur un PC : lecture de l'état de l'agent, plan,
// exécution, puis enregistrement du résultat (même partiel : une invitation créée n'est jamais
// perdue) et republication.
func (s *Service) syncShareAgent(ctx context.Context, id string) error {
	sh := s.sharing
	d, ok := s.ownDevice(id)
	if !ok || d.Agent == nil || !d.Agent.HasPin() || !d.HasHost() {
		return nil
	}
	var plan share.AgentPlan
	res := share.PlanResult{Created: map[string]protocol2.InviteInfo{}}
	err := s.agent.AdminSession(ctx, d.Host, *d.Agent, func(ctx context.Context, ops agentclient.AdminOps) error {
		state, err := ops.State(ctx)
		if err != nil {
			return err
		}
		sh.mu.Lock()
		o := sh.state.Owner
		if o != nil {
			plan = o.PlanAgent(id, share.AgentView{Devices: state.Devices, Invites: state.Invites}, s.now())
		}
		sh.mu.Unlock()
		if o == nil {
			return nil
		}
		for _, inv := range plan.Cancel {
			if err := ops.CancelInvite(ctx, inv); err != nil && !ignorable(err) {
				return err
			}
		}
		for _, identity := range plan.Revoke {
			if err := ops.Revoke(ctx, identity); err != nil && !ignorable(err) {
				return err
			}
		}
		for _, r := range plan.Roles {
			if err := ops.SetRole(ctx, r.Identity, r.Role); err != nil {
				return err
			}
		}
		res.Changed = true
		for _, req := range plan.Invite {
			inv, err := ops.Invite(ctx, req.Role, share.InviteTTL, req.Name, req.Identity)
			if err != nil {
				return err
			}
			res.Created[req.Identity] = inv
		}
		return nil
	})
	if errors.Is(err, context.Canceled) {
		return err
	}
	sh.mu.Lock()
	changed, revocations := false, 0
	if o := sh.state.Owner; o != nil && (res.Changed || len(res.Created) > 0) {
		revocations = len(o.Revocations)
		if changed = o.ApplyAgent(id, plan, res); changed {
			revocations -= len(o.Revocations)
			sh.dirty = true
			if serr := sh.saveLocked(); serr != nil {
				diag.Error(areaShare, "[%s] : partage v2 : état non enregistré : %v", shortID(id), serr)
			}
		}
	}
	sh.mu.Unlock()
	if res.Changed || len(res.Created) > 0 {
		diag.Info(areaShare, "« %s » : partage v2 : %d invitation(s) créée(s) sur %d, %d annulée(s), %d appareil(s) retiré(s), %d rôle(s) aligné(s), %d enregistré(s)%s",
			d.Name, len(res.Created), len(plan.Invite), len(plan.Cancel), len(plan.Revoke), len(plan.Roles), len(plan.Registered),
			pick(len(plan.Skipped) > 0, ", administrateurs laissés tels quels", ""))
	}
	if changed {
		if revocations != 0 {
			s.markBackupDirty()
		}
		sh.poke()
		s.notify()
	}
	if err == nil {
		s.agentAnswered(id)
	}
	return err
}

// shareAgentStates décrit, pour l'interface, où en est le partage v2 d'une personne sur chacun de ses
// PC (verrou sh.mu tenu) : « registered », « invited » (avec l'expiration), « pending » (invitation
// pas encore créée), « notAdmin » (cet ordinateur doit devenir administrateur du PC).
func (s *Service) shareAgentStatesLocked(o *share.Owner, p share.Person, pinned func(id string) bool) map[string]map[string]any {
	out := map[string]map[string]any{}
	for device, right := range p.Rights {
		if right.AgentRole() == "" || !p.V2(pinned(device)) {
			continue
		}
		st := map[string]any{"state": "pending"}
		i := slices.IndexFunc(o.Pairings, func(x share.Pairing) bool {
			return x.Person == p.Device && x.Device == device && x.Identity == p.Identity
		})
		switch {
		case i >= 0 && o.Pairings[i].Registered:
			st["state"] = "registered"
		case i >= 0 && o.Pairings[i].Code != "" && o.Pairings[i].Expires > s.now().Unix():
			st["state"], st["expires"] = "invited", o.Pairings[i].Expires
		case s.sharing.agents.notAdminOf(device):
			st["state"] = "notAdmin"
		}
		out[device] = st
	}
	return out
}

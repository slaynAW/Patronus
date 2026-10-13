package app

import (
	"context"
	"sync"
	"time"

	"github.com/slaynaw/wakeonlan/desktop/internal/agentclient"
	"github.com/slaynaw/wakeonlan/desktop/internal/diag"
	"github.com/slaynaw/wakeonlan/desktop/internal/model"
	"github.com/slaynaw/wakeonlan/desktop/internal/share"
	"github.com/slaynaw/wakeonlan/desktop/internal/status"
)

// Partage v2, côté destinataire (docs/PARTAGE.md, « Partage v2 ») : dès qu'un PC reçu répond, cet
// ordinateur utilise l'invitation qui lui est réservée (pair), une seule fois par invitation ; il
// joint ensuite ce PC en wolagent/2, comme ses propres PC. Retiré par le PC, il le traite de nouveau
// comme un PC « démarrer », jusqu'à une nouvelle invitation. Les journaux ne contiennent jamais le
// code d'une invitation.

// receivedTimeout borne un appairage avec une invitation reçue.
const receivedTimeout = 20 * time.Second

// receivedPairing suit les appairages en cours et les délais après une erreur réseau (verrou propre,
// jamais pris en tenant sh.mu).
type receivedPairing struct {
	mu       sync.Mutex
	running  map[string]bool
	failures map[string]int
	next     map[string]time.Time
}

func newReceivedPairing() *receivedPairing {
	return &receivedPairing{running: map[string]bool{}, failures: map[string]int{}, next: map[string]time.Time{}}
}

// isRunning indique qu'un appairage est en cours pour un PC reçu.
func (r *receivedPairing) isRunning(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running[id]
}

// receivedTarget est un PC reçu et ce qu'il faut y faire.
type receivedTarget struct {
	owner, device string
	pc            model.Device
	invite        share.AgentInvite
	pair, unpair  bool
}

// receivedTargetLocked retrouve le PC reçu d'identifiant local id (verrou sh.mu tenu).
func (s *Service) receivedTargetLocked(id, self string, st status.DeviceStatus) (receivedTarget, bool) {
	sh := s.sharing
	for _, a := range sh.state.Received {
		if !a.Active {
			continue
		}
		for _, d := range a.Devices {
			if share.SharedID(a.Owner, d.ID) != id {
				continue
			}
			t := receivedTarget{owner: a.Owner, device: d.ID, pc: d.Clone()}
			if d.Agent == nil || !d.Agent.HasPin() {
				return t, true
			}
			paired := a.Paired[d.ID] == d.Agent.Pin
			t.unpair = paired && st.AgentError == agentclient.Unregistered
			if !paired && st.State == status.Online {
				for _, inv := range a.Pending(self, s.now()) {
					if inv.Device == d.ID {
						t.invite, t.pair = inv, true
					}
				}
			}
			return t, true
		}
	}
	return receivedTarget{}, false
}

// considerReceived décide, d'après le dernier état d'un PC reçu, s'il faut utiliser son invitation
// ou oublier son enregistrement.
func (s *Service) considerReceived(id string, st status.DeviceStatus) {
	sh := s.sharing
	if sh == nil || !s.isShared(id) {
		return
	}
	self := s.selfIdentity()
	sh.mu.Lock()
	t, ok := s.receivedTargetLocked(id, self, st)
	if ok && t.unpair {
		if i := sh.state.Find(t.owner); i >= 0 {
			sh.state.Received[i].Unpair(t.device)
			if err := sh.saveLocked(); err != nil {
				diag.Error(areaShare, "PC reçu %s : état non enregistré : %v", shortID(id), err)
			}
		}
	}
	sh.mu.Unlock()
	if !ok {
		return
	}
	if t.unpair {
		diag.Info(areaShare, "PC reçu « %s » : cet ordinateur a été retiré du PC ; « démarrer » seulement", t.pc.Name)
		go s.sharedChanged()
		return
	}
	if !t.pair {
		return
	}
	r := sh.received
	r.mu.Lock()
	if r.running[id] || s.now().Before(r.next[id]) {
		r.mu.Unlock()
		return
	}
	r.running[id] = true
	r.mu.Unlock()
	go func() {
		err := s.pairReceived(id, t)
		r.mu.Lock()
		delete(r.running, id)
		if err == nil {
			delete(r.failures, id)
			delete(r.next, id)
		} else {
			r.failures[id]++
			r.next[id] = s.now().Add(min(upgradeRetryMax, upgradeRetryMin<<min(r.failures[id]-1, 6)))
		}
		r.mu.Unlock()
	}()
}

// pairReceived utilise l'invitation d'un PC reçu. Un refus de l'agent (code expiré, déjà utilisé,
// réservé à une autre identité) marque l'invitation comme essayée : elle n'est jamais retentée, pour
// ne pas compter d'échecs auprès de l'agent. Seule une erreur réseau est renvoyée (nouvel essai).
func (s *Service) pairReceived(id string, t receivedTarget) error {
	sh := s.sharing
	ctx, cancel := context.WithTimeout(context.Background(), receivedTimeout)
	defer cancel()
	agent := model.AgentSettings{Port: t.pc.Agent.Port, Pin: t.pc.Agent.Pin}
	res, err := s.agent.Pair(ctx, t.pc.Host, agent, t.invite.Code)
	refused := agentclient.CodeOf(err) == agentclient.PairingFailed || agentclient.CodeOf(err) == agentclient.Rejected
	if err != nil && !refused {
		diag.Warn(areaShare, "PC reçu « %s » : invitation non utilisée, PC injoignable (%s %v)", t.pc.Name, agentclient.CodeOf(err), err)
		return err
	}
	sh.mu.Lock()
	if i := sh.state.Find(t.owner); i >= 0 {
		a := &sh.state.Received[i]
		if err != nil {
			a.MarkTried(t.device, t.invite.Expires)
		} else {
			a.MarkPaired(t.device, t.pc.Agent.Pin)
		}
		if serr := sh.saveLocked(); serr != nil {
			diag.Error(areaShare, "PC reçu « %s » : état non enregistré : %v", t.pc.Name, serr)
		}
	}
	sh.mu.Unlock()
	if err != nil {
		diag.Warn(areaShare, "PC reçu « %s » : invitation refusée par le PC (%s) ; en attente d'une nouvelle invitation", t.pc.Name,
			agentclient.ReasonOf(err))
		return nil
	}
	diag.Info(areaShare, "PC reçu « %s » : cet ordinateur est enregistré par le PC (rôle %s%s)", t.pc.Name, res.Role,
		pick(res.Already, ", déjà enregistré", ""))
	s.agentAnswered(id)
	s.sharedChanged()
	return nil
}

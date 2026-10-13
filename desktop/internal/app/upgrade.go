package app

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol2"
	"github.com/slaynaw/wakeonlan/desktop/internal/agentclient"
	"github.com/slaynaw/wakeonlan/desktop/internal/diag"
	"github.com/slaynaw/wakeonlan/desktop/internal/model"
	"github.com/slaynaw/wakeonlan/desktop/internal/status"
)

// Passage des PC en wolagent/2 (docs/PROTOCOLE-V2.md, §9), sans rien demander à l'utilisateur, dès
// que l'agent du PC le permet :
//
//  1. l'agent annonce wolagent/2 et son empreinte dans une réponse wolagent/1, signée par la clé :
//     l'empreinte est donc authentique ;
//  2. une session wolagent/2 où cette empreinte est exigée dit si l'agent connaît déjà cet
//     ordinateur ; sinon, il l'enregistre (migration, prouvée par la clé de part et d'autre) ;
//  3. l'empreinte est alors enregistrée et la clé effacée, en une seule écriture : l'application ne
//     parle plus que wolagent/2 à ce PC.
//
// Une interruption à n'importe quelle étape laisse le PC en wolagent/1 ; l'essai suivant reprend
// (l'étape 2 reconnaît un enregistrement déjà fait). Un PC partagé en v1 avec le droit « démarrer
// et éteindre » (personne sans partage v2, §10) garde sa clé, que ce partage transmet, tant qu'il en a
// besoin. Les PC reçus d'une autre personne ne sont pas concernés.

// upgradeTimeout borne un passage complet (trois échanges au plus).
const upgradeTimeout = 30 * time.Second

// Délais entre deux essais après un échec : doublés à chaque fois, d'une minute à une heure.
const (
	upgradeRetryMin = time.Minute
	upgradeRetryMax = time.Hour
)

// upgrader suit les passages en cours et les délais après un échec (verrou propre).
type upgrader struct {
	mu       sync.Mutex
	running  map[string]bool
	failures map[string]int
	next     map[string]time.Time
}

func newUpgrader() *upgrader {
	return &upgrader{running: map[string]bool{}, failures: map[string]int{}, next: map[string]time.Time{}}
}

// considerUpgrade décide, d'après le dernier état d'un PC, s'il faut avancer son passage.
func (s *Service) considerUpgrade(id string, st status.DeviceStatus) {
	d, ok := s.ownDevice(id)
	if !ok || d.Agent == nil || !d.Agent.HasKey() || !d.HasHost() {
		return
	}
	v2 := st.Agent != nil && st.Agent.Proto == protocol2.ALPN
	switch {
	case !d.Agent.HasPin():
		// wolagent/1 : l'agent doit annoncer wolagent/2 et son empreinte.
		if st.Agent == nil || st.Agent.Pin == "" || !slices.Contains(st.Agent.Protocols, protocol2.ALPN) {
			return
		}
	case v2 && s.sharedWithControl(id):
		return // enregistré, clé gardée pour le partage : rien à faire
	case !v2 && st.AgentError != agentclient.Unregistered:
		return // PC éteint, injoignable ou autre empreinte : rien à tenter
	}
	s.startUpgrade(id)
}

// startUpgrade lance le passage d'un PC (un seul à la fois par PC, jamais avant le délai d'attente).
func (s *Service) startUpgrade(id string) {
	u := s.upgrades
	u.mu.Lock()
	if u.running[id] || s.now().Before(u.next[id]) {
		u.mu.Unlock()
		return
	}
	u.running[id] = true
	u.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), upgradeTimeout)
		defer cancel()
		err := s.upgradeAgent(ctx, id)
		u.mu.Lock()
		delete(u.running, id)
		var wait time.Duration
		if err == nil {
			delete(u.failures, id)
			delete(u.next, id)
		} else {
			u.failures[id]++
			wait = min(upgradeRetryMax, upgradeRetryMin<<min(u.failures[id]-1, 6))
			u.next[id] = s.now().Add(wait)
		}
		u.mu.Unlock()
		if err != nil {
			diag.Warn(areaAgent, "passage en wolagent/2 de [%s] : %s (%v), nouvel essai dans %s", shortID(id),
				agentclient.CodeOf(err), err, wait)
		}
	}()
}

// upgradeAgent mène le passage d'un PC en wolagent/2 (voir plus haut).
func (s *Service) upgradeAgent(ctx context.Context, id string) error {
	d, ok := s.ownDevice(id)
	if !ok || d.Agent == nil || !d.Agent.HasKey() || !d.HasHost() {
		return nil
	}
	before := *d.Agent
	next := before
	if !next.HasPin() {
		// Réponse wolagent/1 signée par la clé : l'empreinte annoncée est celle du vrai agent.
		st, err := s.agent.Status(ctx, d.Host, next)
		if err != nil {
			return err
		}
		if _, err := protocol2.ParsePin(st.Pin); err != nil || !slices.Contains(st.Protocols, protocol2.ALPN) {
			return nil
		}
		next.Pin = st.Pin
	}
	_, err := s.agent.Status(ctx, d.Host, next)
	if agentclient.CodeOf(err) == agentclient.Unregistered {
		var m agentclient.Migration
		if m, err = s.agent.Migrate(ctx, d.Host, next); err == nil {
			diag.Info(areaAgent, "« %s » : cet ordinateur est enregistré par l'agent (empreinte %s, rôle %s)",
				d.Name, next.ShortPin(), m.Role)
		}
	}
	if err != nil {
		return err
	}
	s.agentAnswered(id)
	kept := s.sharedWithControl(id)
	if !kept {
		next.Key = ""
	}
	if next == before {
		return nil
	}
	if !s.updateAgent(id, before, next) {
		return nil // modifié entre-temps (fiche du PC, import) : l'état suivant décidera
	}
	if kept {
		diag.Info(areaAgent, "« %s » : passé en wolagent/2 ; clé gardée pour le partage « démarrer et éteindre »", d.Name)
	} else {
		diag.Info(areaAgent, "« %s » : passé en wolagent/2, clé wolagent/1 effacée", d.Name)
	}
	return nil
}

// agentAnswered signale que l'agent d'un PC accepte cet ordinateur (enregistrement, test ou action
// réussis) : la sonde le recontacte sans attendre la fin d'un délai après un refus.
func (s *Service) agentAnswered(id string) {
	if p, ok := s.prober.(interface{ Forget(id string) }); ok {
		p.Forget(id)
	}
}

// updateAgent remplace les réglages de l'agent d'un PC s'ils valent toujours expected ; renvoie
// true si c'est fait.
func (s *Service) updateAgent(id string, expected, next model.AgentSettings) bool {
	if d, ok := s.ownDevice(id); !ok || d.Agent == nil || *d.Agent != expected {
		return false
	}
	applied := false
	err := s.mutate(func(c model.AppConfig) model.AppConfig {
		d, ok := c.Device(id)
		if !ok || d.Agent == nil || *d.Agent != expected {
			return c
		}
		d.Agent = &next
		applied = true
		return c.Upsert(d)
	})
	if err != nil {
		diag.Error(areaData, "agent de [%s] : configuration non enregistrée : %v", shortID(id), err)
		return false
	}
	return applied
}

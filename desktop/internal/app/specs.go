package app

import (
	"context"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol"
	"github.com/slaynaw/wakeonlan/desktop/internal/agentclient"
	"github.com/slaynaw/wakeonlan/desktop/internal/diag"
	"github.com/slaynaw/wakeonlan/desktop/internal/pcspecs"
	"github.com/slaynaw/wakeonlan/desktop/internal/status"
)

// Fiche des PC (commande « specs » de l'agent 1.10.0) : lue quand le PC répond, gardée sur le disque.
const (
	// specsMaxAge : une fiche plus ancienne est relue (le matériel a pu changer).
	specsMaxAge = 24 * time.Hour
	// specsRetry : délai avant une nouvelle tentative après un échec ; specsBusyRetry quand l'agent
	// finissait sa première lecture.
	specsRetry     = 5 * time.Minute
	specsBusyRetry = 15 * time.Second
	specsTimeout   = 8 * time.Second
)

type specsFetchState struct {
	running bool
	last    time.Time
	wait    time.Duration
	// unsupported : version de l'agent qui ne connaît pas la commande (relue s'il est mis à jour).
	unsupported string
}

// SpecsView est la fiche affichée (date de lecture en ms).
type SpecsView struct {
	Specs   protocol.Specs `json:"specs"`
	Fetched int64          `json:"fetched"`
}

// loadSpecs lit les fiches enregistrées (au démarrage).
func (s *Service) loadSpecs(store *pcspecs.Store) {
	s.specsStore = store
	s.specsFetch = map[string]*specsFetchState{}
	var err error
	if s.specs, err = store.Load(); err != nil {
		diag.Warn(areaData, "%v", err)
	}
}

// refreshSpecs lit la fiche d'un PC qui vient de répondre par son agent, si elle manque ou a vieilli.
func (s *Service) refreshSpecs(id string, st status.DeviceStatus) {
	if st.State != status.Online || st.Agent == nil {
		return
	}
	d, ok := s.device(id)
	if !ok || d.Agent == nil || !d.Agent.HasKey() || !d.HasHost() {
		return
	}
	version := st.Agent.Version
	now := s.now()
	s.specsMu.Lock()
	entry, known := s.specs[id]
	fetch := s.specsFetch[id]
	if fetch == nil {
		fetch = &specsFetchState{}
		s.specsFetch[id] = fetch
	}
	fresh := known && entry.Agent == version && now.Sub(time.UnixMilli(entry.Fetched)) < specsMaxAge
	if fetch.running || fresh || fetch.unsupported == version || (!fetch.last.IsZero() && now.Sub(fetch.last) < fetch.wait) {
		s.specsMu.Unlock()
		return
	}
	fetch.running, fetch.last, fetch.wait = true, now, specsRetry
	s.specsMu.Unlock()

	host, settings := d.Host, *d.Agent
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), specsTimeout)
		defer cancel()
		specs, err := s.agent.Specs(ctx, host, settings)
		_, stillThere := s.device(id)
		s.specsMu.Lock()
		fetch.running = false
		switch {
		case !stillThere || s.specsFetch[id] != fetch:
		case err == nil:
			s.specs[id] = pcspecs.Entry{Specs: specs, Fetched: s.now().UnixMilli(), Agent: version}
			if err := s.specsStore.Save(s.specs); err != nil {
				diag.Warn(areaData, "fiches non enregistrées : %v", err)
			}
		case agentclient.ReasonOf(err) == "busy":
			fetch.wait = specsBusyRetry
		case agentclient.CodeOf(err) == agentclient.Rejected:
			// Redemandée dès que l'agent change de version (mis à jour), sans attendre.
			fetch.unsupported, fetch.wait = version, 0
			diag.Info(areaAgent, "fiche de « %s » : agent %s trop ancien", d.Name, version)
		default:
			diag.Info(areaAgent, "fiche de « %s » illisible : %s (%v)", d.Name, agentclient.CodeOf(err), err)
		}
		s.specsMu.Unlock()
		if err == nil {
			s.notify()
		}
	}()
}

// specsView renvoie la fiche d'un PC (nil si elle n'a jamais été lue).
func (s *Service) specsView(id string) *SpecsView {
	s.specsMu.Lock()
	defer s.specsMu.Unlock()
	entry, ok := s.specs[id]
	if !ok {
		return nil
	}
	return &SpecsView{Specs: entry.Specs, Fetched: entry.Fetched}
}

// keepSpecs oublie la fiche des PC retirés.
func (s *Service) keepSpecs(ids map[string]bool) {
	s.specsMu.Lock()
	defer s.specsMu.Unlock()
	changed := false
	for id := range s.specs {
		if !ids[id] {
			delete(s.specs, id)
			changed = true
		}
	}
	for id := range s.specsFetch {
		if !ids[id] {
			delete(s.specsFetch, id)
		}
	}
	if changed {
		if err := s.specsStore.Save(s.specs); err != nil {
			diag.Warn(areaData, "fiches non enregistrées : %v", err)
		}
	}
}

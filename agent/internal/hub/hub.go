// Package hub assemble le hub sur le NAS (docs/HUB.md) : stockage chiffré, registre des appareils, PC
// confiés, archive, réglages, surveillance et serveur patronus-hub/1. La commande patronus-hub et le
// hub des tests (paquet testhub) l'assemblent de la même façon.
package hub

import (
	"context"
	"errors"
	"log"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/slaynaw/wakeonlan/agent/internal/devices"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/agents"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/archive"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/fleet"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/monitor"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/schedule"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/server"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/settings"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/store"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/wake"
	"github.com/slaynaw/wakeonlan/agent/internal/identity"
	"github.com/slaynaw/wakeonlan/agent/protocol2"
)

// RegistryFile est le registre des appareils autorisés, dans le stockage chiffré.
const RegistryFile = "devices.db"

// pruneEvery est l'intervalle entre deux nettoyages de l'archive (après celui du démarrage).
const pruneEvery = 6 * time.Hour

// Config décrit un hub à ouvrir.
type Config struct {
	Data, Keys    string // données chiffrées (/data) ; identité et clé des données (/keys)
	Name, Version string
	Logger        *log.Logger
	// Networks sont les réseaux où le hub joint les agents et les appareils : agents.TargetNetworks()
	// si vide (réseaux privés, sans l'adresse de bouclage).
	Networks []netip.Prefix
}

// Hub est un hub assemblé, prêt à servir.
type Hub struct {
	Identity *identity.Identity
	Registry *devices.Registry
	Server   *server.Server
	// KeyCreated, IdentityCreated : la clé des données ou l'identité vient d'être créée.
	KeyCreated, IdentityCreated bool

	logger   *log.Logger
	archive  *archive.Archive
	settings *settings.Settings
	monitor  *monitor.Monitor
	schedule *schedule.Scheduler
}

// Open ouvre (ou crée au premier démarrage) le hub décrit par cfg.
func Open(cfg Config) (*Hub, error) {
	if cfg.Logger == nil {
		return nil, errors.New("hub : journal manquant")
	}
	key, keyCreated, err := store.LoadOrCreateKey(cfg.Keys)
	if err != nil {
		return nil, err
	}
	id, idCreated, err := identity.LoadOrCreate(cfg.Keys, time.Now())
	if err != nil {
		return nil, err
	}
	st, err := store.Open(cfg.Data, key)
	if err != nil {
		return nil, err
	}
	registry, err := devices.OpenStorage(st.File(RegistryFile), RegistryFile+" (chiffré)", time.Now)
	if err != nil {
		return nil, err
	}
	pcs, err := fleet.Open(st.File(fleet.FileName), time.Now)
	if err != nil {
		return nil, err
	}
	prefs, err := settings.Open(st.File(settings.FileName))
	if err != nil {
		return nil, err
	}
	networks := cfg.Networks
	if len(networks) == 0 {
		networks = agents.TargetNetworks()
	}
	dialer := &agents.Dialer{Identity: id, Networks: networks}
	arch := archive.New(st, time.Now)
	watch := monitor.New(pcs, dialer, arch, cfg.Logger)
	srv, err := server.New(server.Config{
		Name: cfg.Name, Version: cfg.Version, Identity: id, Registry: registry, Fleet: pcs, Agents: dialer,
		States: watch.States, Archive: arch, Settings: prefs, NoteWake: watch.NoteWake, Logger: cfg.Logger,
	})
	if err != nil {
		return nil, err
	}
	plans := schedule.New(schedule.Config{
		Fleet: pcs, Settings: prefs, Dialer: dialer, Archive: arch, States: watch.States,
		Wake: wake.Send, NoteWake: watch.NoteWake, Hub: id.Pin, Logger: cfg.Logger,
	})
	return &Hub{
		Identity: id, Registry: registry, Server: srv, KeyCreated: keyCreated, IdentityCreated: idCreated,
		logger: cfg.Logger, archive: arch, settings: prefs, monitor: watch, schedule: plans,
	}, nil
}

// HasAdmin indique si un administrateur est enregistré.
func (h *Hub) HasAdmin() bool {
	for _, d := range h.Registry.Devices() {
		if d.Role == protocol2.RoleAdmin {
			return true
		}
	}
	return false
}

// Run surveille les PC, nettoie l'archive et sert les applications sur ln jusqu'à l'annulation de ctx,
// qui ferme ln ; il rend la main une fois la surveillance et le nettoyage arrêtés.
func (h *Hub) Run(ctx context.Context, ln net.Listener) error {
	ctx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Go(func() { h.monitor.Run(ctx) })
	wg.Go(func() { h.prune(ctx) })
	wg.Go(func() { h.schedule.Run(ctx) })
	stop := context.AfterFunc(ctx, func() { _ = ln.Close() })
	err := h.Server.Serve(ln)
	stop()
	cancel()
	wg.Wait()
	return err
}

// prune retire de l'archive, au démarrage puis toutes les 6 heures, ce qui dépasse la durée de
// conservation.
func (h *Hub) prune(ctx context.Context) {
	for {
		if n, err := h.archive.Prune(h.settings.Get().RetentionDays); err != nil {
			h.logger.Printf("archive : %v", err)
		} else if n > 0 {
			h.logger.Printf("archive : %d mois au-delà de la durée de conservation retirés", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(pruneEvery):
		}
	}
}

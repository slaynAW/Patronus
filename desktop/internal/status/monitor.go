package status

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/slaynaw/wakeonlan/desktop/internal/agentclient"
	"github.com/slaynaw/wakeonlan/desktop/internal/model"
)

// FastInterval est l'intervalle de vérification pendant un réveil / une extinction.
const FastInterval = time.Second

// Availability indique si l'ordinateur peut vérifier l'état des PC en ce moment.
type Availability struct {
	CanProbe bool
	Reason   UnknownReason
}

type update struct {
	cfg    model.AppConfig
	avail  Availability
	active bool
}

// Monitor surveille en continu l'état de tous les PC : une boucle par PC, toutes les
// pollIntervalSeconds, et toutes les secondes pendant une transition. Comme sur Android, la
// surveillance ne tourne que lorsque la fenêtre est visible (active).
type Monitor struct {
	prober   Prober
	clock    func() int64
	onChange func()

	mu       sync.Mutex
	trackers map[string]*Tracker
	signals  map[string]chan struct{}
	statuses map[string]DeviceStatus
	pending  *update
	wake     chan struct{}
}

// NewMonitor crée la surveillance ; onChange est appelé (hors verrou) à chaque changement d'état.
func NewMonitor(prober Prober, clock func() int64, onChange func()) *Monitor {
	if clock == nil {
		clock = func() int64 { return time.Now().UnixMilli() }
	}
	if onChange == nil {
		onChange = func() {}
	}
	return &Monitor{
		prober: prober, clock: clock, onChange: onChange,
		trackers: map[string]*Tracker{}, signals: map[string]chan struct{}{}, statuses: map[string]DeviceStatus{},
		wake: make(chan struct{}, 1),
	}
}

// Update transmet la configuration, la disponibilité du réseau et la visibilité de la fenêtre.
// Les boucles sont relancées si quelque chose a changé (non bloquant).
func (m *Monitor) Update(cfg model.AppConfig, avail Availability, active bool) {
	m.mu.Lock()
	m.pending = &update{cfg: cfg.Clone(), avail: avail, active: active}
	m.mu.Unlock()
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

// Run exécute la surveillance jusqu'à l'annulation de ctx.
func (m *Monitor) Run(ctx context.Context) {
	cancel := context.CancelFunc(func() {})
	var wg sync.WaitGroup
	var last *update
	defer func() {
		cancel()
		wg.Wait()
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.wake:
		}
		m.mu.Lock()
		u := m.pending
		m.pending = nil
		m.mu.Unlock()
		if u == nil || (last != nil && reflect.DeepEqual(*last, *u)) {
			continue
		}
		// Comme collectLatest côté Android : on arrête les boucles précédentes avant de relancer.
		cancel()
		wg.Wait()
		last = u
		m.prune(u.cfg.Devices)
		if !u.active {
			continue
		}
		gen, c := context.WithCancel(ctx)
		cancel = c
		for _, d := range u.cfg.Devices {
			wg.Add(1)
			go func() {
				defer wg.Done()
				m.pollLoop(gen, d, u.cfg.Settings, u.avail)
			}()
		}
	}
}

// Statuses renvoie une copie de l'état de chaque PC, par identifiant.
func (m *Monitor) Statuses() map[string]DeviceStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]DeviceStatus, len(m.statuses))
	for k, v := range m.statuses {
		out[k] = v
	}
	return out
}

// Refresh demande une vérification immédiate (tous les PC si id est vide).
func (m *Monitor) Refresh(id string) {
	m.mu.Lock()
	var targets []chan struct{}
	for k, ch := range m.signals {
		if id == "" || k == id {
			targets = append(targets, ch)
		}
	}
	m.mu.Unlock()
	for _, ch := range targets {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// OnWakeSent signale l'envoi d'un paquet magique.
func (m *Monitor) OnWakeSent(id string) {
	m.act(id, true, (*Tracker).OnWakeSent)
}

// OnPowerActionSent signale une demande d'extinction, de redémarrage ou de mise en veille.
func (m *Monitor) OnPowerActionSent(id string, action agentclient.Action) {
	if action == agentclient.Reboot {
		m.act(id, true, (*Tracker).OnRestartSent)
	} else {
		m.act(id, true, (*Tracker).OnShutdownSent)
	}
}

// ClearNotice efface la notification d'un PC.
func (m *Monitor) ClearNotice(id string) {
	m.act(id, false, (*Tracker).ClearNotice)
}

func (m *Monitor) act(id string, refresh bool, f func(*Tracker) DeviceStatus) {
	m.apply(id, f)
	if refresh {
		m.Refresh(id)
	}
}

func (m *Monitor) apply(id string, f func(*Tracker) DeviceStatus) DeviceStatus {
	m.mu.Lock()
	t := m.tracker(id)
	st := f(t)
	m.statuses[id] = st
	m.mu.Unlock()
	m.onChange()
	return st
}

// tracker renvoie (ou crée) la machine à états d'un PC ; appelé verrou pris.
func (m *Monitor) tracker(id string) *Tracker {
	t, ok := m.trackers[id]
	if !ok {
		t = NewTracker(m.clock)
		m.trackers[id] = t
	}
	return t
}

func (m *Monitor) signal(id string) chan struct{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch, ok := m.signals[id]
	if !ok {
		ch = make(chan struct{}, 1)
		m.signals[id] = ch
	}
	return ch
}

func (m *Monitor) prune(devices []model.Device) {
	keep := map[string]bool{}
	for _, d := range devices {
		keep[d.ID] = true
	}
	m.mu.Lock()
	changed := false
	for id := range m.trackers {
		if !keep[id] {
			delete(m.trackers, id)
		}
	}
	for id := range m.signals {
		if !keep[id] {
			delete(m.signals, id)
		}
	}
	for id := range m.statuses {
		if !keep[id] {
			delete(m.statuses, id)
			changed = true
		}
	}
	m.mu.Unlock()
	if changed {
		m.onChange()
	}
}

func (m *Monitor) pollLoop(ctx context.Context, d model.Device, settings model.AppSettings, avail Availability) {
	m.mu.Lock()
	m.tracker(d.ID).WakeTimeoutMs = int64(settings.WakeTimeoutSeconds) * 1000
	m.mu.Unlock()
	signal := m.signal(d.ID)
	interval := time.Duration(settings.Clamped().PollIntervalSeconds) * time.Second
	for {
		var st DeviceStatus
		switch {
		case !d.HasHost():
			st = m.apply(d.ID, func(t *Tracker) DeviceStatus { return t.OnUnavailable(NoHost) })
		case !avail.CanProbe:
			reason := avail.Reason
			if reason == "" {
				reason = NoNetwork
			}
			st = m.apply(d.ID, func(t *Tracker) DeviceStatus { return t.OnUnavailable(reason) })
		default:
			result := m.prober.Probe(ctx, d)
			if ctx.Err() != nil {
				return
			}
			st = m.apply(d.ID, func(t *Tracker) DeviceStatus { return t.OnProbe(result) })
		}
		wait := interval
		if st.State.IsTransitional() {
			wait = min(interval, FastInterval)
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-signal:
			timer.Stop()
		case <-timer.C:
		}
	}
}

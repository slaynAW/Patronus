package schedule

import (
	"context"
	"io"
	"log"
	"net/netip"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/hubprotocol"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/agents"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/archive"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/fleet"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/hubtest"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/settings"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/store"
	"github.com/slaynaw/wakeonlan/agent/internal/identity"
	"github.com/slaynaw/wakeonlan/agent/internal/power"
	"github.com/slaynaw/wakeonlan/agent/protocol"
	"github.com/slaynaw/wakeonlan/agent/protocol2"
)

func paris(t *testing.T) *time.Location {
	t.Helper()
	loc, err := settings.Location("Europe/Paris")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

// TestOccurrences : jours ISO, heure locale du hub, changements d'heure compris.
func TestOccurrences(t *testing.T) {
	loc := paris(t)
	at := func(s string) time.Time {
		v, err := time.ParseInLocation("2006-01-02 15:04", s, loc)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	weekdays := hubprotocol.Slot{On: true, Days: []int{1, 2, 3, 4, 5}, At: "07:30"}
	// Vendredi 30 octobre 2026, 08:00 : dernière occurrence ce matin, prochaine lundi.
	now := at("2026-10-30 08:00")
	if p, ok := Previous(weekdays, loc, now); !ok || !p.Equal(at("2026-10-30 07:30")) {
		t.Errorf("précédente : %v %v", p, ok)
	}
	if n, ok := Next(weekdays, loc, now); !ok || !n.Equal(at("2026-11-02 07:30")) {
		t.Errorf("suivante : %v %v", n, ok)
	}
	// Dimanche seulement, à l'heure pile : l'occurrence est celle-ci.
	sunday := hubprotocol.Slot{On: true, Days: []int{7}, At: "23:00"}
	if p, ok := Previous(sunday, loc, at("2026-11-01 23:00")); !ok || !p.Equal(at("2026-11-01 23:00")) {
		t.Errorf("dimanche : %v %v", p, ok)
	}
	// Passage à l'heure d'hiver (25 octobre 2026, 03:00 → 02:00) : 07:30 reste 07:30, à 1 h d'UTC de plus.
	daily := hubprotocol.Slot{On: true, Days: []int{1, 2, 3, 4, 5, 6, 7}, At: "07:30"}
	before, _ := Previous(daily, loc, at("2026-10-24 12:00"))
	after, _ := Previous(daily, loc, at("2026-10-25 12:00"))
	if before.UTC().Hour() != 5 || after.UTC().Hour() != 6 || after.Sub(before) != 25*time.Hour {
		t.Errorf("heure d'hiver : %v puis %v", before.UTC(), after.UTC())
	}
	// 02:30 n'existe pas le 29 mars 2026 (heure d'été) : décalée d'une heure, une seule fois.
	night := hubprotocol.Slot{On: true, Days: []int{7}, At: "02:30"}
	gap, ok := Next(night, loc, at("2026-03-28 12:00"))
	if !ok || gap.Sub(at("2026-03-29 00:00")) < 2*time.Hour || gap.Sub(at("2026-03-29 00:00")) > 4*time.Hour {
		t.Errorf("heure d'été : %v %v", gap, ok)
	}
	if again, _ := Next(night, loc, gap); again.Sub(gap) < 6*24*time.Hour {
		t.Errorf("deux fois la même nuit : %v puis %v", gap, again)
	}
	// Autre fuseau.
	ny, _ := settings.Location("America/New_York")
	if p, _ := Previous(daily, ny, at("2026-10-30 20:00")); p.In(ny).Hour() != 7 || p.In(ny).Minute() != 30 {
		t.Errorf("New York : %v", p.In(ny))
	}
	if _, ok := Previous(hubprotocol.Slot{Days: []int{1}, At: "7h30"}, loc, now); ok {
		t.Error("heure invalide acceptée")
	}
}

func TestBusy(t *testing.T) {
	now := time.Date(2026, 10, 30, 23, 0, 0, 0, time.UTC)
	load := func(v float64) *float64 { return &v }
	rows := func(n int, cpu float64) []protocol.MetricsRow {
		var out []protocol.MetricsRow
		for i := range n {
			out = append(out, protocol.MetricsRow{T: now.Add(-time.Duration(i+1) * time.Minute).Unix(), N: 6, CPULoad: load(cpu)})
		}
		return out
	}
	hub := "7KQ2-9MXP-4D1Z"
	if why := Busy(rows(15, 3), nil, hub, now); why != "" {
		t.Errorf("inactif : %q", why)
	}
	if why := Busy(append(rows(14, 3), protocol.MetricsRow{T: now.Add(-30 * time.Second).Unix(), CPULoad: load(40)}), nil, hub, now); why == "" {
		t.Error("processeur utilisé : accepté")
	}
	if why := Busy(rows(11, 3), nil, hub, now); why == "" {
		t.Error("trop peu de minutes relevées : accepté")
	}
	// Une minute ancienne, au-delà des 15 dernières, ne compte pas.
	old := append(rows(15, 3), protocol.MetricsRow{T: now.Add(-20 * time.Minute).Unix(), CPULoad: load(90)})
	if why := Busy(old, nil, hub, now); why != "" {
		t.Errorf("minute ancienne : %q", why)
	}
	recent := now.Add(-30 * time.Minute).Unix()
	for _, e := range []protocol.HistoryEvent{
		{T: recent, K: protocol.HistoryCommand, A: "reboot", I: "AAAA-BBBB-CCCC"},
		{T: recent, K: protocol.HistoryWake, B: "Pixel 8"},
	} {
		if why := Busy(rows(15, 3), []protocol.HistoryEvent{e}, hub, now); why == "" {
			t.Errorf("commande récente %+v : acceptée", e)
		}
	}
	for _, e := range []protocol.HistoryEvent{
		{T: recent, K: protocol.HistoryWake, I: hub},                                 // réveil du hub lui-même
		{T: now.Add(-2 * time.Hour).Unix(), K: protocol.HistoryCommand, A: "reboot"}, // il y a plus d'une heure
		{T: recent, K: protocol.HistoryBoot},
	} {
		if why := Busy(rows(15, 3), []protocol.HistoryEvent{e}, hub, now); why != "" {
			t.Errorf("évènement %+v : %q", e, why)
		}
	}
	if got := Days(time.Date(2026, 10, 31, 0, 5, 0, 0, time.UTC), 15); len(got) != 2 || got[0] != "2026-10-30" || got[1] != "2026-10-31" {
		t.Errorf("jours à minuit : %v", got)
	}
}

type memStorage struct{ data []byte }

func (m *memStorage) Load() ([]byte, error) {
	if m.data == nil {
		return nil, os.ErrNotExist
	}
	return m.data, nil
}

func (m *memStorage) Save(data []byte) error { m.data = append([]byte(nil), data...); return nil }

type env struct {
	s     *Scheduler
	fleet *fleet.Fleet
	arch  *archive.Archive
	agent *hubtest.Agent
	hub   *identity.Identity
	pc    fleet.PC
	mu    sync.Mutex
	now   time.Time
	wakes []string
}

func (e *env) clock() time.Time {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.now
}

func (e *env) set(t time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.now = t
}

// run fait un passage du planificateur et attend ses essais.
func (e *env) run() {
	var wg sync.WaitGroup
	e.s.tick(context.Background(), &wg)
	wg.Wait()
}

func (e *env) events(t *testing.T) []hubprotocol.Event {
	t.Helper()
	ev, _, err := e.arch.HubEvents(0)
	if err != nil {
		t.Fatal(err)
	}
	return ev
}

// start confie au hub un vrai agent de test (rôle wake) avec la programmation sched, à l'heure now.
func start(t *testing.T, now time.Time, sched hubprotocol.Schedule) *env {
	t.Helper()
	hub, _, err := identity.LoadOrCreate(t.TempDir(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	dialer := &agents.Dialer{Identity: hub, Networks: append(agents.TargetNetworks(), netip.MustParsePrefix("127.0.0.0/8"))}
	a := hubtest.StartAgent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	role, err := dialer.Pair(ctx, "127.0.0.1", a.Port, a.ID.Pin, a.Invite(t, protocol2.RoleWake, hub.Pin), "Hub NAS")
	if err != nil {
		t.Fatal(err)
	}
	list, err := fleet.Open(&memStorage{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	pin := a.ID.Pin
	pc, err := list.Add(fleet.PC{Name: "PC Bureau", Host: "127.0.0.1", Port: a.Port, MAC: "AA:BB:CC:DD:EE:01", Agent: &pin, Role: role, Schedule: &sched})
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(t.TempDir(), store.Key{4})
	if err != nil {
		t.Fatal(err)
	}
	set, err := settings.Open(&memStorage{})
	if err != nil {
		t.Fatal(err)
	}
	e := &env{fleet: list, agent: a, hub: hub, pc: pc, now: now}
	e.arch = archive.New(st, time.Now)
	e.s = New(Config{
		Fleet: list, Settings: set, Dialer: dialer, Archive: e.arch, Hub: hub.Pin,
		States: func() []hubprotocol.PCState {
			return []hubprotocol.PCState{{ID: pc.ID, State: hubprotocol.StateOnline}}
		},
		Wake: func(_ context.Context, mac string) (int, error) {
			e.mu.Lock()
			defer e.mu.Unlock()
			e.wakes = append(e.wakes, mac)
			return 3, nil
		},
		Logger: log.New(io.Discard, "", 0),
		Now:    e.clock,
	})
	return e
}

// idle enregistre sur l'agent 15 minutes d'utilisation du processeur à cpu %, jusqu'à now.
func (e *env) idle(now time.Time, cpu float64) {
	byDay := map[string][]protocol.MetricsRow{}
	for i := range 15 {
		v := cpu
		r := protocol.MetricsRow{T: now.Add(-time.Duration(i+1) * time.Minute).Unix(), N: 6, CPULoad: &v}
		day := time.Unix(r.T, 0).UTC().Format(time.DateOnly)
		byDay[day] = append(byDay[day], r)
	}
	for day, rows := range byDay {
		e.agent.Metrics.Set(day, rows)
	}
}

// TestShutdown : extinction programmée refusée sans le rôle control, puis demandée (jamais forcée), et
// reportée quand le PC reste utilisé.
func TestShutdown(t *testing.T) {
	now := time.Now().Truncate(time.Minute)
	loc := paris(t)
	slot := &hubprotocol.Slot{On: true, Days: []int{1, 2, 3, 4, 5, 6, 7}, At: now.In(loc).Format("15:04")}
	e := start(t, now.Add(30*time.Second), hubprotocol.Schedule{Shutdown: slot})
	e.idle(now, 3)

	// Rôle wake : l'agent refuse.
	e.run()
	if calls := e.agent.Power.Calls(); len(calls) != 0 {
		t.Fatalf("extinction sans le rôle control : %+v", calls)
	}
	if ev := e.events(t); len(ev) != 1 || ev[0].K != hubprotocol.EventShutdown || ev[0].X != hubprotocol.ShutdownRefused || !hubprotocol.ValidEvent(ev[0]) {
		t.Fatalf("évènements : %+v", ev)
	}
	// Une occurrence n'est menée qu'une fois.
	e.run()
	if ev := e.events(t); len(ev) != 1 {
		t.Fatalf("occurrence menée deux fois : %+v", ev)
	}

	// Rôle control donné par l'application : extinction normale.
	if _, err := e.agent.Registry.SetRole(e.hub.Pin, protocol2.RoleControl, false); err != nil {
		t.Fatal(err)
	}
	e.s.fired = map[string]time.Time{}
	e.run()
	// L'agent éteint 1,5 s après sa réponse.
	deadline := time.Now().Add(10 * time.Second)
	for len(e.agent.Power.Calls()) == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if calls := e.agent.Power.Calls(); len(calls) != 1 || calls[0].Action != power.Shutdown || calls[0].Force {
		t.Fatalf("extinction : %+v", calls)
	}
	if ev := e.events(t); len(ev) != 2 || ev[1].X != hubprotocol.ShutdownDone {
		t.Fatalf("évènements : %+v", ev)
	}

	// PC utilisé : nouvel essai toutes les 15 minutes, puis renoncement au bout de 2 heures.
	e.idle(now, 60)
	e.s.fired = map[string]time.Time{}
	e.run()
	if a := e.s.attempts[e.pc.ID]; a == nil || a.next.Sub(e.clock()) != RetryEvery {
		t.Fatalf("essai suivant : %+v", a)
	}
	e.set(now.Add(GiveUpAfter + time.Minute))
	e.run()
	if calls := e.agent.Power.Calls(); len(calls) != 1 {
		t.Fatalf("PC utilisé éteint : %+v", calls)
	}
	if ev := e.events(t); len(ev) != 3 || ev[2].X != hubprotocol.ShutdownPostponed {
		t.Fatalf("évènements : %+v", ev)
	}
	if _, running := e.s.attempts[e.pc.ID]; running {
		t.Fatal("essai toujours en cours")
	}
}

// TestWake : réveil programmé, une fois par occurrence ; occurrence manquée au-delà de 10 minutes sautée.
func TestWake(t *testing.T) {
	now := time.Now().Truncate(time.Minute)
	loc := paris(t)
	slot := &hubprotocol.Slot{On: true, Days: []int{1, 2, 3, 4, 5, 6, 7}, At: now.In(loc).Format("15:04")}
	e := start(t, now.Add(time.Minute), hubprotocol.Schedule{Wake: slot})
	e.run()
	e.run()
	if len(e.wakes) != 1 || e.wakes[0] != "AA:BB:CC:DD:EE:01" {
		t.Fatalf("réveils : %v", e.wakes)
	}
	if ev := e.events(t); len(ev) != 1 || ev[0].K != hubprotocol.EventWake || ev[0].I != hubprotocol.WakeSchedule || !hubprotocol.ValidEvent(ev[0]) {
		t.Fatalf("évènements : %+v", ev)
	}
	// Hub arrêté pendant l'occurrence et reparti 11 minutes après : sautée.
	late := start(t, now.Add(Grace+time.Minute), hubprotocol.Schedule{Wake: slot})
	late.run()
	if len(late.wakes) != 0 {
		t.Fatalf("occurrence trop ancienne menée : %v", late.wakes)
	}
	// Désactivée : rien.
	off := *slot
	off.On = false
	idle := start(t, now.Add(time.Minute), hubprotocol.Schedule{Wake: &off})
	idle.run()
	if len(idle.wakes) != 0 {
		t.Fatalf("programmation désactivée menée : %v", idle.wakes)
	}
}

// Package schedule mène les programmations du hub (docs/PROTOCOLE-HUB.md, §6.7) : réveil à l'heure dite,
// et extinction d'un PC seulement s'il est inactif, jamais forcée, dans le fuseau horaire réglé sur le hub.
package schedule

import (
	"context"
	"log"
	"slices"
	"sync"
	"time"

	"github.com/slaynaw/wakeonlan/agent/hubprotocol"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/agents"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/archive"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/fleet"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/settings"
	"github.com/slaynaw/wakeonlan/agent/protocol"
	"github.com/slaynaw/wakeonlan/agent/protocol2"
)

// Règles des programmations (§6.7).
const (
	// Grace : une occurrence manquée (hub arrêté) est rattrapée si le hub repart dans ce délai.
	Grace = 10 * time.Minute
	// RetryEvery et GiveUpAfter : un PC encore utilisé est revu toutes les 15 minutes, pendant 2 heures.
	RetryEvery  = 15 * time.Minute
	GiveUpAfter = 2 * time.Hour
	// Inactif : utilisation moyenne du processeur sous IdleLoad % chaque minute des IdleMinutes dernières,
	// au moins IdleMinRows minutes relevées, et aucune commande d'un autre appareil depuis Quiet.
	IdleLoad     = 10.0
	IdleMinutes  = 15
	IdleMinRows  = 12
	Quiet        = time.Hour
	tickEvery    = 20 * time.Second
	agentTimeout = 15 * time.Second
)

// Config relie le planificateur au reste du hub.
type Config struct {
	Fleet    *fleet.Fleet
	Settings *settings.Settings
	Dialer   *agents.Dialer
	Archive  *archive.Archive
	// States donne l'état relevé de chaque PC (monitor.Monitor.States).
	States func() []hubprotocol.PCState
	// Wake envoie le paquet magique (wake.Send) ; NoteWake le signale au journal de l'agent.
	Wake     func(ctx context.Context, mac string) (int, error)
	NoteWake func(pc string, t int64)
	// Hub est l'empreinte du hub : ses propres commandes, notées par l'agent, ne comptent pas.
	Hub    protocol2.Pin
	Logger *log.Logger
	Now    func() time.Time
}

// Scheduler mène les programmations.
type Scheduler struct {
	cfg Config

	mu       sync.Mutex
	fired    map[string]time.Time // dernière occurrence menée, par PC et type (« id|wake »)
	attempts map[string]*attempt  // extinctions en cours, par PC
}

// attempt est une extinction programmée en cours : occurrence, prochain essai, essai en cours.
type attempt struct {
	since, next time.Time
	busy        bool
}

// New prépare le planificateur.
func New(cfg Config) *Scheduler {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Scheduler{cfg: cfg, fired: map[string]time.Time{}, attempts: map[string]*attempt{}}
}

// Run mène les programmations jusqu'à l'annulation de ctx.
func (s *Scheduler) Run(ctx context.Context) {
	t := time.NewTicker(tickEvery)
	defer t.Stop()
	var wg sync.WaitGroup
	defer wg.Wait()
	for {
		s.tick(ctx, &wg)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Location renvoie le fuseau horaire des programmations.
func (s *Scheduler) Location() *time.Location {
	loc, err := settings.Location(s.cfg.Settings.Get().Timezone)
	if err != nil {
		loc, _ = settings.Location(settings.DefaultTimezone)
	}
	return loc
}

// tick mène les occurrences arrivées et les extinctions en cours.
func (s *Scheduler) tick(ctx context.Context, wg *sync.WaitGroup) {
	now := s.cfg.Now()
	loc := s.Location()
	pcs := s.cfg.Fleet.List()
	online := map[string]bool{}
	for _, st := range s.cfg.States() {
		online[st.ID] = st.State == hubprotocol.StateOnline
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	known := map[string]bool{}
	for _, p := range pcs {
		known[p.ID] = true
		if p.Schedule == nil {
			delete(s.attempts, p.ID)
			continue
		}
		if w := p.Schedule.Wake; w != nil && w.On && p.MAC != "" {
			if occ, ok := s.due(p.ID+"|wake", *w, loc, now); ok {
				wg.Go(func() { s.wake(ctx, p, occ) })
			}
		}
		sd := p.Schedule.Shutdown
		if sd == nil || !sd.On || p.Agent == nil {
			delete(s.attempts, p.ID)
			continue
		}
		if occ, ok := s.due(p.ID+"|shutdown", *sd, loc, now); ok && online[p.ID] {
			// Un PC éteint à l'heure dite n'est pas concerné.
			if _, running := s.attempts[p.ID]; !running {
				s.attempts[p.ID] = &attempt{since: occ, next: now}
			}
		}
		a := s.attempts[p.ID]
		switch {
		case a == nil || a.busy || now.Before(a.next):
		case !online[p.ID]:
			delete(s.attempts, p.ID) // éteint entre-temps : rien de plus à faire
		case now.Sub(a.since) >= GiveUpAfter:
			delete(s.attempts, p.ID)
			s.event(p, hubprotocol.ShutdownPostponed, now)
			s.cfg.Logger.Printf("extinction programmée de « %s » reportée : PC encore utilisé depuis 2 heures", p.Name)
		default:
			a.busy = true
			wg.Go(func() { s.shutdown(ctx, p, a) })
		}
	}
	for id := range s.attempts {
		if !known[id] {
			delete(s.attempts, id) // PC retiré du hub
		}
	}
}

// due indique qu'une occurrence de slot est arrivée et pas encore menée (verrou mu).
func (s *Scheduler) due(key string, slot hubprotocol.Slot, loc *time.Location, now time.Time) (time.Time, bool) {
	occ, ok := Previous(slot, loc, now)
	if !ok || now.Sub(occ) > Grace || !s.fired[key].Before(occ) {
		return time.Time{}, false
	}
	s.fired[key] = occ
	return occ, true
}

// wake envoie le paquet magique d'un réveil programmé.
func (s *Scheduler) wake(ctx context.Context, p fleet.PC, occ time.Time) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	sent, err := s.cfg.Wake(ctx, p.MAC)
	if err != nil {
		s.cfg.Logger.Printf("réveil programmé de « %s » impossible : %v", p.Name, err)
		return
	}
	now := s.cfg.Now().Unix()
	if err := s.cfg.Archive.AddHubEvents(hubprotocol.Event{T: now, PC: p.ID, K: hubprotocol.EventWake, I: hubprotocol.WakeSchedule}); err != nil {
		s.cfg.Logger.Printf("évènements du hub : %v", err)
	}
	if p.Agent != nil && s.cfg.NoteWake != nil {
		s.cfg.NoteWake(p.ID, now)
	}
	s.cfg.Logger.Printf("réveil programmé de « %s » (%s, %d paquets)", p.Name, occ.Format("Mon 15:04 MST"), sent)
}

// shutdown fait un essai d'extinction programmée : éteint le PC s'il est inactif, sinon prévoit l'essai
// suivant.
func (s *Scheduler) shutdown(ctx context.Context, p fleet.PC, a *attempt) {
	done, refused := s.try(ctx, p)
	s.mu.Lock()
	defer s.mu.Unlock()
	a.busy = false
	if !done && !refused {
		a.next = s.cfg.Now().Add(RetryEvery)
		return
	}
	if s.attempts[p.ID] == a {
		delete(s.attempts, p.ID)
	}
	x := hubprotocol.ShutdownDone
	if refused {
		x = hubprotocol.ShutdownRefused
	}
	s.event(p, x, s.cfg.Now())
}

// try vérifie que le PC est inactif et, s'il l'est, lui demande de s'éteindre (sans forcer). done : demande
// acceptée ; refused : l'agent refuse au hub le droit de l'éteindre (rôle control absent).
func (s *Scheduler) try(parent context.Context, p fleet.PC) (done, refused bool) {
	ctx, cancel := context.WithTimeout(parent, agentTimeout)
	defer cancel()
	sess, err := s.cfg.Dialer.Dial(ctx, p.Host, p.Port, *p.Agent)
	if err != nil {
		s.cfg.Logger.Printf("extinction programmée de « %s » : agent injoignable (%v), nouvel essai dans 15 minutes", p.Name, err)
		return false, false
	}
	defer sess.Close()
	now := s.cfg.Now()
	var rows []protocol.MetricsRow
	for _, day := range Days(now, IdleMinutes) {
		resp, err := sess.Do(ctx, protocol2.Request{Cmd: "metrics", Day: day})
		if err != nil || !resp.OK || resp.Metrics == nil {
			s.cfg.Logger.Printf("extinction programmée de « %s » : mesures illisibles, nouvel essai dans 15 minutes", p.Name)
			return false, false
		}
		rows = append(rows, resp.Metrics.Rows...)
	}
	resp, err := sess.Do(ctx, protocol2.Request{Cmd: "history"})
	if err != nil || !resp.OK || resp.History == nil {
		s.cfg.Logger.Printf("extinction programmée de « %s » : journal illisible, nouvel essai dans 15 minutes", p.Name)
		return false, false
	}
	if why := Busy(rows, resp.History.Events, s.cfg.Hub.Short(), now); why != "" {
		s.cfg.Logger.Printf("extinction programmée de « %s » : %s, nouvel essai dans 15 minutes", p.Name, why)
		return false, false
	}
	resp, err = sess.Do(ctx, protocol2.Request{Cmd: "shutdown"})
	switch {
	case err != nil:
		s.cfg.Logger.Printf("extinction programmée de « %s » : %v", p.Name, err)
		return false, false
	case resp.Code == protocol2.CodeInsufficientRole:
		s.cfg.Logger.Printf("extinction programmée de « %s » refusée : le hub n'a pas le rôle control sur ce PC", p.Name)
		return false, true
	case !resp.OK:
		s.cfg.Logger.Printf("extinction programmée de « %s » refusée par l'agent : %s", p.Name, resp.Code)
		return false, true
	}
	s.cfg.Logger.Printf("extinction programmée de « %s » demandée (PC inactif)", p.Name)
	return true, false
}

// event note un évènement d'extinction programmée.
func (s *Scheduler) event(p fleet.PC, x string, now time.Time) {
	if err := s.cfg.Archive.AddHubEvents(hubprotocol.Event{T: now.Unix(), PC: p.ID, K: hubprotocol.EventShutdown, X: x}); err != nil {
		s.cfg.Logger.Printf("évènements du hub : %v", err)
	}
}

// Busy dit pourquoi un PC n'est pas inactif (vide : il l'est), d'après ses mesures et son journal : chaque
// minute des IdleMinutes dernières sous IdleLoad % (au moins IdleMinRows relevées), aucune commande ni
// demande de démarrage d'un autre appareil que le hub (hub : son empreinte courte) depuis Quiet.
func Busy(rows []protocol.MetricsRow, events []protocol.HistoryEvent, hub string, now time.Time) string {
	from := now.Add(-IdleMinutes * time.Minute).Unix()
	seen := 0
	for _, r := range rows {
		if r.T < from || r.T > now.Unix() {
			continue
		}
		if r.CPULoad == nil {
			continue
		}
		if *r.CPULoad >= IdleLoad {
			return "processeur utilisé"
		}
		seen++
	}
	if seen < IdleMinRows {
		return "utilisation du processeur inconnue"
	}
	quiet := now.Add(-Quiet).Unix()
	for _, e := range events {
		if e.T >= quiet && (e.K == protocol.HistoryCommand || e.K == protocol.HistoryWake) && e.I != hub {
			return "commande d'une application depuis moins d'une heure"
		}
	}
	return ""
}

// Days renvoie les jours de mesures (UTC, « 2026-10-03 ») qui couvrent les minutes dernières minutes.
func Days(now time.Time, minutes int) []string {
	first := now.UTC().Add(-time.Duration(minutes) * time.Minute).Format(time.DateOnly)
	last := now.UTC().Format(time.DateOnly)
	if first == last {
		return []string{last}
	}
	return []string{first, last}
}

// Previous renvoie la dernière occurrence de slot au plus tard à now, dans le fuseau loc (false : aucune
// dans la semaine écoulée, programmation invalide).
func Previous(slot hubprotocol.Slot, loc *time.Location, now time.Time) (time.Time, bool) {
	hh, mm, ok := clock(slot.At)
	if !ok {
		return time.Time{}, false
	}
	local := now.In(loc)
	for back := 0; back <= 7; back++ {
		day := local.AddDate(0, 0, -back)
		t := time.Date(day.Year(), day.Month(), day.Day(), hh, mm, 0, 0, loc)
		if slices.Contains(slot.Days, isoWeekday(t)) && !t.After(now) {
			return t, true
		}
	}
	return time.Time{}, false
}

// Next renvoie la prochaine occurrence de slot après now, dans le fuseau loc.
func Next(slot hubprotocol.Slot, loc *time.Location, now time.Time) (time.Time, bool) {
	hh, mm, ok := clock(slot.At)
	if !ok {
		return time.Time{}, false
	}
	local := now.In(loc)
	for ahead := 0; ahead <= 8; ahead++ {
		day := local.AddDate(0, 0, ahead)
		t := time.Date(day.Year(), day.Month(), day.Day(), hh, mm, 0, 0, loc)
		if slices.Contains(slot.Days, isoWeekday(t)) && t.After(now) {
			return t, true
		}
	}
	return time.Time{}, false
}

// isoWeekday renvoie le jour ISO de t : 1 (lundi) à 7 (dimanche).
func isoWeekday(t time.Time) int {
	return (int(t.Weekday())+6)%7 + 1
}

// clock lit une heure « HH:MM ».
func clock(at string) (hh, mm int, ok bool) {
	if hubprotocol.ValidSlot(hubprotocol.Slot{Days: []int{1}, At: at}) != nil {
		return 0, 0, false
	}
	return int(at[0]-'0')*10 + int(at[1]-'0'), int(at[3]-'0')*10 + int(at[4]-'0'), true
}

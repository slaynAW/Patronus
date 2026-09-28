package history

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol"
)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newClock() *clock { return &clock{t: time.Date(2026, 9, 28, 8, 0, 0, 0, time.UTC)} }

func kinds(h protocol.History) []string {
	var out []string
	for _, e := range h.Events {
		out = append(out, e.K)
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestBootShutdownCycleAndReload(t *testing.T) {
	c := newClock()
	path := filepath.Join(t.TempDir(), "history.json")
	l, err := Open(path, c.Now)
	if err != nil {
		t.Fatal(err)
	}
	boot1 := Boot{ID: "a", At: c.Now().Add(-3 * time.Minute)}
	if err := l.Started(boot1, time.Time{}); err != nil {
		t.Fatal(err)
	}
	c.Advance(time.Hour)
	_ = l.Add(protocol.HistoryEvent{T: c.Now().Unix(), K: protocol.HistoryCommand, A: "shutdown", C: "192.168.1.37"})
	if err := l.Stopping(); err != nil {
		t.Fatal(err)
	}

	// Rechargement depuis le disque puis nouveau démarrage le lendemain.
	c.Advance(24 * time.Hour)
	l2, err := Open(path, c.Now)
	if err != nil {
		t.Fatal(err)
	}
	if err := l2.Started(Boot{ID: "b", At: c.Now().Add(-time.Minute)}, c.Now().Add(-24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	h := l2.Snapshot()
	if want := []string{"boot", "cmd", "shutdown", "boot"}; !equal(kinds(h), want) {
		t.Fatalf("évènements %v, attendu %v", kinds(h), want)
	}
	if h.Events[0].T != boot1.At.Unix() || h.From > boot1.At.Unix() {
		t.Errorf("heure de démarrage ou début de couverture incorrects : %+v", h)
	}
	if h.Events[1].C != "192.168.1.37" || h.Events[1].A != "shutdown" {
		t.Errorf("commande mal enregistrée : %+v", h.Events[1])
	}
}

func TestAgentRestartIsNotAShutdown(t *testing.T) {
	c := newClock()
	l, _ := Open(filepath.Join(t.TempDir(), "h.json"), c.Now)
	boot := Boot{ID: "same", At: c.Now().Add(-time.Hour)}
	_ = l.Started(boot, time.Time{})
	c.Advance(10 * time.Minute)
	_ = l.Stopping() // mise à jour de l'agent
	c.Advance(5 * time.Second)
	_ = l.Started(boot, c.Now().Add(-10*time.Second))
	if got := kinds(l.Snapshot()); !equal(got, []string{"boot"}) {
		t.Fatalf("un redémarrage du service ne doit rien laisser : %v", got)
	}
	// Sans identifiant de démarrage : même démarrage si l'heure concorde (tolérance d'horloge).
	l2, _ := Open(filepath.Join(t.TempDir(), "h2.json"), c.Now)
	_ = l2.Started(Boot{At: c.Now().Add(-time.Hour)}, time.Time{})
	_ = l2.Stopping()
	_ = l2.Started(Boot{At: c.Now().Add(-time.Hour + 3*time.Minute)}, time.Time{})
	if got := kinds(l2.Snapshot()); !equal(got, []string{"boot"}) {
		t.Fatalf("même démarrage sans identifiant : %v", got)
	}
}

func TestUnrecordedShutdownIsLost(t *testing.T) {
	c := newClock()
	l, _ := Open(filepath.Join(t.TempDir(), "h.json"), c.Now)
	_ = l.Started(Boot{ID: "1", At: c.Now().Add(-time.Minute)}, time.Time{})
	lastAlive := c.Now().Add(2 * time.Hour)
	c.Advance(10 * time.Hour) // coupure de courant pendant la nuit
	_ = l.Started(Boot{ID: "2", At: c.Now().Add(-time.Minute)}, lastAlive)
	h := l.Snapshot()
	if !equal(kinds(h), []string{"boot", "lost", "boot"}) || h.Events[1].T != lastAlive.Unix() {
		t.Fatalf("arrêt non enregistré attendu à l'heure du dernier signe de vie : %+v", h.Events)
	}

	// Extinction demandée puis arrêt trop rapide pour être enregistré : c'est un arrêt, pas une coupure.
	c.Advance(time.Hour)
	_ = l.Add(protocol.HistoryEvent{T: c.Now().Unix(), K: protocol.HistoryCommand, A: "reboot"})
	alive := c.Now().Add(time.Second)
	c.Advance(2 * time.Minute)
	_ = l.Started(Boot{ID: "3", At: c.Now().Add(-30 * time.Second)}, alive)
	got := kinds(l.Snapshot())
	if got[len(got)-2] != "shutdown" || got[len(got)-1] != "boot" {
		t.Fatalf("arrêt après commande : %v", got)
	}

	// Après une mise en veille, un nouveau démarrage (batterie vide...) n'ajoute pas de coupure.
	_ = l.Add(protocol.HistoryEvent{T: c.Now().Unix(), K: protocol.HistorySleep})
	alive = c.Now().Add(-time.Second)
	c.Advance(5 * time.Hour)
	_ = l.Started(Boot{ID: "4", At: c.Now().Add(-time.Minute)}, alive)
	got = kinds(l.Snapshot())
	if got[len(got)-2] != "sleep" || got[len(got)-1] != "boot" {
		t.Fatalf("démarrage après veille : %v", got)
	}
}

func TestRetentionAndCapacity(t *testing.T) {
	c := newClock()
	l, _ := Open(filepath.Join(t.TempDir(), "h.json"), c.Now)
	_ = l.Add(protocol.HistoryEvent{T: c.Now().Unix(), K: protocol.HistoryBoot})
	c.Advance(31 * 24 * time.Hour)
	_ = l.Add(protocol.HistoryEvent{T: c.Now().Unix(), K: protocol.HistoryShutdown})
	h := l.Snapshot()
	if !equal(kinds(h), []string{"shutdown"}) || h.From != c.Now().Add(-Retention).Unix() {
		t.Fatalf("rétention de 30 jours : %+v", h)
	}
	for i := 0; i < MaxEvents+10; i++ {
		_ = l.Add(protocol.HistoryEvent{T: c.Now().Unix() + int64(i), K: protocol.HistoryResume})
	}
	h = l.Snapshot()
	if len(h.Events) != MaxEvents || h.From != h.Events[0].T {
		t.Fatalf("capacité : %d évènements, début %d / %d", len(h.Events), h.From, h.Events[0].T)
	}
	// Les évènements restent triés même ajoutés dans le désordre.
	_ = l.Add(protocol.HistoryEvent{T: h.Events[0].T + 1, K: protocol.HistoryBoot})
	h = l.Snapshot()
	for i := 1; i < len(h.Events); i++ {
		if h.Events[i].T < h.Events[i-1].T {
			t.Fatal("journal non trié")
		}
	}
}

func TestCorruptFileIsReplaced(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "h.json")
	if err := WriteAlive(path, time.Now()); err != nil { // contenu inattendu
		t.Fatal(err)
	}
	l, err := Open(path, time.Now)
	if err != nil || len(l.Snapshot().Events) != 0 {
		t.Fatalf("fichier inattendu : %v", err)
	}
	alive := filepath.Join(dir, "alive")
	now := time.Unix(1_800_000_000, 0)
	if err := WriteAlive(alive, now); err != nil || !ReadAlive(alive).Equal(now) || !ReadAlive(filepath.Join(dir, "x")).IsZero() {
		t.Fatal("signe de vie mal relu")
	}
}

func TestWatcherDetectsSleep(t *testing.T) {
	c := newClock()
	dir := t.TempDir()
	l, _ := Open(filepath.Join(dir, "h.json"), c.Now)
	var mu sync.Mutex
	suspended := 5 * time.Second
	w := &Watcher{Log: l, AlivePath: filepath.Join(dir, "alive"), Interval: 5 * time.Millisecond, Now: c.Now,
		Suspended: func() (time.Duration, bool) {
			mu.Lock()
			defer mu.Unlock()
			return suspended, true
		}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { w.Run(ctx); close(done) }()
	time.Sleep(30 * time.Millisecond)

	// Deux heures de veille entre deux passages.
	mu.Lock()
	suspended += 2 * time.Hour
	mu.Unlock()
	c.Advance(2 * time.Hour)
	deadline := time.Now().Add(2 * time.Second)
	for len(l.Snapshot().Events) < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done
	h := l.Snapshot()
	if !equal(kinds(h), []string{"sleep", "resume"}) || h.Events[1].T-h.Events[0].T != int64((2*time.Hour).Seconds()) {
		t.Fatalf("veille non détectée : %+v", h.Events)
	}
	if ReadAlive(w.AlivePath).IsZero() {
		t.Error("signe de vie non écrit")
	}
}

func TestWatcherFallsBackOnWallClockGap(t *testing.T) {
	c := newClock()
	dir := t.TempDir()
	l, _ := Open(filepath.Join(dir, "h.json"), c.Now)
	w := &Watcher{Log: l, AlivePath: filepath.Join(dir, "alive"), Interval: 5 * time.Millisecond, Now: c.Now,
		Suspended: func() (time.Duration, bool) { return 0, false }}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { w.Run(ctx); close(done) }()
	time.Sleep(20 * time.Millisecond)
	c.Advance(45 * time.Minute)
	deadline := time.Now().Add(2 * time.Second)
	for len(l.Snapshot().Events) < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done
	if got := kinds(l.Snapshot()); !equal(got, []string{"sleep", "resume"}) {
		t.Fatalf("veille déduite de l'horloge : %v", got)
	}
}

func TestCurrentBoot(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 400_000_000, time.UTC)
	b := CurrentBoot(90*time.Minute, now)
	if !b.At.Equal(time.Date(2026, 9, 28, 10, 30, 0, 0, time.UTC)) {
		t.Errorf("heure de démarrage : %v", b.At)
	}
	t.Logf("identifiant de démarrage de ce système : %q", BootID())
	if total, ok := SuspendedTotal(); ok && total < 0 {
		t.Errorf("temps de veille négatif : %v", total)
	}
}

func TestAddWakes(t *testing.T) {
	c := newClock()
	path := filepath.Join(t.TempDir(), "history.json")
	l, err := Open(path, c.Now)
	if err != nil {
		t.Fatal(err)
	}
	bootAt := c.Now().Add(-time.Hour)
	if err := l.Started(Boot{ID: "a", At: bootAt}, time.Time{}); err != nil {
		t.Fatal(err)
	}
	wake := bootAt.Add(-40 * time.Second).Unix()
	now := c.Now().Unix()
	added, err := l.AddWakes([]int64{
		wake,
		wake + 20,        // même démarrage (signalé deux fois, ou par deux appareils)
		now + 3600,       // dans le futur : ignoré
		now - 40*24*3600, // hors de la période couverte : ignoré
		now - 10*60,      // autre démarrage demandé, PC déjà allumé
	}, "192.168.1.37", "Pixel 8")
	if err != nil || added != 2 {
		t.Fatalf("ajouts : %d, %v", added, err)
	}
	h := l.Snapshot()
	if !equal(kinds(h), []string{protocol.HistoryWake, protocol.HistoryBoot, protocol.HistoryWake}) {
		t.Fatalf("ordre chronologique : %v", kinds(h))
	}
	if e := h.Events[0]; e.T != wake || e.C != "192.168.1.37" || e.B != "Pixel 8" {
		t.Errorf("démarrage noté : %+v", e)
	}
	// Déjà connus : rien à ajouter, rien à écrire.
	if added, err := l.AddWakes([]int64{wake}, "192.168.1.37", "Pixel 8"); added != 0 || err != nil {
		t.Errorf("doublon : %d, %v", added, err)
	}
	// Conservé au redémarrage de l'agent.
	l2, err := Open(path, c.Now)
	if err != nil || len(l2.Snapshot().Events) != 3 {
		t.Fatalf("relecture : %v", err)
	}
}

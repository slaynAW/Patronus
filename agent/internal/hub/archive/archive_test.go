package archive

import (
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/internal/hub/store"
	"github.com/slaynaw/wakeonlan/agent/protocol"
)

const pc = "q2Xh0bE9mGk4J1tYw7RfZA"

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func open(t *testing.T) (*Archive, *store.Store, *clock) {
	t.Helper()
	st, err := store.Open(t.TempDir(), store.Key{7})
	if err != nil {
		t.Fatal(err)
	}
	c := &clock{t: time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)}
	return New(st, c.now), st, c
}

func at(day string) int64 {
	t, _ := time.Parse("2006-01-02 15:04", day)
	return t.Unix()
}

func TestEvents(t *testing.T) {
	a, st, _ := open(t)
	journal := []protocol.HistoryEvent{
		{T: at("2026-09-30 22:00"), K: protocol.HistoryShutdown},
		{T: at("2026-10-01 08:00"), K: protocol.HistoryBoot},
		{T: at("2026-10-02 09:00"), K: protocol.HistoryLost},
		{T: at("2026-10-02 09:05"), K: protocol.HistoryCommand, A: "shutdown", C: "192.168.1.37", B: "Pixel 8"},
	}
	added, err := a.AddEvents(pc, journal)
	if err != nil || len(added) != 4 {
		t.Fatalf("ajout : %d, %v", len(added), err)
	}
	// Le même journal relu dix minutes plus tard : rien de nouveau ; puis la cause de l'arrêt anormal est
	// précisée par l'agent : seule la précision s'ajoute.
	if added, err := a.AddEvents(pc, journal); err != nil || len(added) != 0 {
		t.Fatalf("même journal : %d, %v", len(added), err)
	}
	precise := journal[2]
	precise.R, precise.D = protocol.LostBSOD, "0x7E SYSTEM_THREAD_EXCEPTION_NOT_HANDLED"
	if added, err := a.AddEvents(pc, append(journal, precise)); err != nil || len(added) != 1 || added[0].R != protocol.LostBSOD {
		t.Fatalf("précision : %+v, %v", added, err)
	}
	// Une nouvelle archive (redémarrage du hub) relit les mêmes évènements, la précision comprise.
	again := New(st, time.Now)
	events, more, first, err := again.Events(pc, 0)
	if err != nil || more || len(events) != 4 || first != journal[0].T {
		t.Fatalf("relecture : %d, %v, %d, %v", len(events), more, first, err)
	}
	if events[2].R != protocol.LostBSOD || events[2].D == "" {
		t.Fatalf("cause perdue : %+v", events[2])
	}
	for i := 1; i < len(events); i++ {
		if events[i].T < events[i-1].T {
			t.Fatal("évènements dans le désordre")
		}
	}
	if added, err := again.AddEvents(pc, journal); err != nil || len(added) != 0 {
		t.Fatalf("après redémarrage : %d, %v", len(added), err)
	}
	// À partir d'une date.
	if events, _, _, _ := again.Events(pc, at("2026-10-02 00:00")); len(events) != 2 {
		t.Fatalf("à partir du 2 octobre : %d", len(events))
	}
	// Deux fichiers : septembre et octobre.
	if names, _ := st.List("pc/" + pc); len(names) != 2 {
		t.Fatalf("fichiers : %v", names)
	}
	if _, err := a.AddEvents("../etc", journal); err != ErrBadPC {
		t.Fatalf("identifiant hors format : %v", err)
	}
}

func TestEventsLimit(t *testing.T) {
	a, _, _ := open(t)
	var journal []protocol.HistoryEvent
	for i := range MaxEvents + 10 {
		journal = append(journal, protocol.HistoryEvent{T: at("2026-10-01 00:00") + int64(i*60), K: protocol.HistoryBoot})
	}
	if _, err := a.AddEvents(pc, journal); err != nil {
		t.Fatal(err)
	}
	events, more, _, err := a.Events(pc, 0)
	if err != nil || !more || len(events) != MaxEvents {
		t.Fatalf("première page : %d, %v, %v", len(events), more, err)
	}
	rest, more, _, _ := a.Events(pc, events[len(events)-1].T+1)
	if more || len(rest) != 10 {
		t.Fatalf("page suivante : %d, %v", len(rest), more)
	}
}

func TestMetrics(t *testing.T) {
	a, st, _ := open(t)
	v := 41.5
	rows := []protocol.MetricsRow{{T: at("2026-10-03 10:00"), N: 6, CPUTemp: &v}, {T: at("2026-10-03 10:01"), N: 6}}
	if err := a.AddDay(pc, "2026-10-03", rows); err != nil {
		t.Fatal(err)
	}
	if err := a.AddDay(pc, "2026-10-03", rows[:1]); err != nil {
		t.Fatal(err)
	}
	if err := a.AddDay(pc, "2026-09-30", rows[:1]); err != nil {
		t.Fatal(err)
	}
	again := New(st, time.Now)
	if ok, err := again.HasDay(pc, "2026-10-03"); !ok || err != nil {
		t.Fatalf("jour gardé : %v, %v", ok, err)
	}
	days, err := again.Days(pc)
	if err != nil || len(days) != 2 || days[0] != "2026-09-30" || days[1] != "2026-10-03" {
		t.Fatalf("jours : %v, %v", days, err)
	}
	got, err := again.Day(pc, "2026-10-03")
	if err != nil || len(got) != 2 || *got[0].CPUTemp != 41.5 {
		t.Fatalf("mesures : %+v, %v", got, err)
	}
	if got, err := again.Day(pc, "2026-10-04"); err != nil || got != nil {
		t.Fatalf("jour absent : %v, %v", got, err)
	}
	for _, bad := range []string{"2026-10", "../x", "2026-10-03/../../a"} {
		if err := a.AddDay(pc, bad, rows); err == nil {
			t.Errorf("jour %q accepté", bad)
		}
	}
}

func TestPruneAndRemove(t *testing.T) {
	a, st, c := open(t)
	other := "AAAAAAAAAAAAAAAAAAAAAA"
	for _, day := range []string{"2025-09-15 10:00", "2025-10-15 10:00", "2026-10-01 10:00"} {
		if _, err := a.AddEvents(pc, []protocol.HistoryEvent{{T: at(day), K: protocol.HistoryBoot}}); err != nil {
			t.Fatal(err)
		}
		if err := a.AddDay(pc, day[:10], nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.AddEvents(other, []protocol.HistoryEvent{{T: at("2025-01-01 10:00"), K: protocol.HistoryBoot}}); err != nil {
		t.Fatal(err)
	}
	// Un an au 24/10/2026 : septembre 2025 et avant sont retirés, octobre 2025 reste.
	removed, err := a.Prune(DefaultRetentionDays)
	if err != nil || removed != 3 {
		t.Fatalf("retirés : %d, %v", removed, err)
	}
	events, _, first, _ := a.Events(pc, 0)
	if len(events) != 2 || first != at("2025-10-15 10:00") {
		t.Fatalf("après la purge : %d évènements, premier %d", len(events), first)
	}
	if days, _ := a.Days(pc); len(days) != 2 {
		t.Fatalf("jours après la purge : %v", days)
	}
	c.t = c.t.AddDate(2, 0, 0)
	if removed, _ := a.Prune(MaxRetentionDays); removed != 0 {
		t.Fatalf("trois ans : %d retirés", removed)
	}
	if err := a.RemovePC(pc); err != nil {
		t.Fatal(err)
	}
	if names, _ := st.List("pc/" + pc); len(names) != 0 {
		t.Fatalf("archive du PC retiré : %v", names)
	}
	if events, _, _, _ := a.Events(pc, 0); len(events) != 0 {
		t.Fatalf("évènements du PC retiré : %d", len(events))
	}
}

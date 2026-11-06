package schedule

import (
	"slices"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/hubprotocol"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/settings"
)

// Fuzzing : go test ./internal/hub/schedule -run '^$' -fuzz FuzzNext. Sans -fuzz, seuls les exemples de
// départ sont rejoués.

// fuzzZones mêle des fuseaux ordinaires et ceux où les changements d'heure sont les plus piégeux :
// changement à minuit (Santiago, Asunción), décalage d'une demi-heure (Lord Howe), quart d'heure
// (Katmandou, Chatham), heure d'été retirée pendant le ramadan (Casablanca).
var fuzzZones = []string{
	"Europe/Paris", "UTC", "America/New_York", "America/Santiago", "America/Asuncion", "Australia/Lord_Howe",
	"Asia/Kathmandu", "Pacific/Chatham", "Africa/Casablanca", "America/Havana", "Asia/Tehran", "Pacific/Apia",
}

// FuzzNext : pour toute programmation valide, à toute date et dans tout fuseau, la prochaine occurrence
// existe, tombe un jour programmé, après maintenant, au plus 8 jours plus tard, et à l'heure dite sauf
// quand cette heure n'existe pas ce jour-là (passage à l'heure d'été).
func FuzzNext(f *testing.F) {
	for _, seed := range []struct {
		zone  uint8
		days  uint8
		hh    uint8
		mm    uint8
		epoch int64
	}{
		{0, 0b1111111, 7, 30, 1793936400},  // tous les jours, Paris
		{0, 0b0000001, 2, 30, 1774746000},  // lundi 2 h 30, fin mars (heure d'été)
		{3, 0b1000000, 0, 30, 1788656400},  // dimanche 0 h 30, Santiago (changement à minuit)
		{4, 0b0000010, 0, 0, 1791763200},   // mardi minuit, Asunción
		{5, 0b0010000, 2, 15, 1791000000},  // vendredi 2 h 15, Lord Howe
		{8, 0b0100000, 2, 0, 1772000000},   // samedi 2 h, Casablanca
		{1, 0b1111111, 23, 59, 4102444799}, // fin 2099
		// Heure d'été à minuit : « 0 h 30 » n'existe pas le jour du changement, et Go la ramène à la veille.
		{3, 0b0100000, 0, 30, 1788660000}, // samedi 0 h 30, Santiago, samedi 5 septembre 2026 à 22 h
		{3, 0b1000000, 0, 30, 1788066000}, // dimanche 0 h 30, Santiago, une semaine avant le changement
		{9, 0b0100000, 0, 30, 1772942400}, // samedi 0 h 30, La Havane, samedi 7 mars 2026 à 23 h
	} {
		f.Add(seed.zone, seed.days, seed.hh, seed.mm, seed.epoch)
	}
	f.Fuzz(func(t *testing.T, zone, days, hh, mm uint8, epoch int64) {
		loc, err := settings.Location(fuzzZones[int(zone)%len(fuzzZones)])
		if err != nil {
			t.Fatal(err)
		}
		var slot hubprotocol.Slot
		for d := 1; d <= 7; d++ {
			if days&(1<<(d-1)) != 0 {
				slot.Days = append(slot.Days, d)
			}
		}
		if len(slot.Days) == 0 {
			slot.Days = []int{1 + int(days>>7)}
		}
		slot.On = true
		slot.At = time.Date(2000, 1, 1, int(hh)%24, int(mm)%60, 0, 0, time.UTC).Format("15:04")
		if err := hubprotocol.ValidSlot(slot); err != nil {
			t.Fatalf("programmation de départ refusée : %+v (%v)", slot, err)
		}
		// De 2000 à 2100 : les fuseaux sont connus, et l'arithmétique des dates reste exacte.
		const from, to = 946684800, 4102444800
		if epoch < from || epoch >= to {
			epoch = from + (epoch%(to-from)+(to-from))%(to-from)
		}
		now := time.Unix(epoch, 0)

		next, ok := Next(slot, loc, now)
		if !ok {
			t.Fatalf("aucune occurrence : %+v, %s, maintenant %s", slot, loc, now.In(loc))
		}
		local := next.In(loc)
		if !next.After(now) || next.Sub(now) > 8*24*time.Hour+2*time.Hour {
			t.Fatalf("occurrence %s hors de la semaine qui suit %s", local, now.In(loc))
		}
		// Le jour visé : celui dont « HH:MM » donne exactement cette occurrence (la veille, le jour même ou
		// le lendemain du jour affiché, quand l'heure dite n'existe pas ce jour-là). Il doit être programmé.
		day, found := intendedDay(next, loc, int(hh)%24, int(mm)%60)
		if !found {
			t.Fatalf("occurrence %s : ce n'est l'heure %s d'aucun jour", local, slot.At)
		}
		if !slices.Contains(slot.Days, isoWeekday(day)) {
			t.Fatalf("occurrence %s pour le %s, jour non programmé %v (maintenant %s)", local, day.Format("Mon 2006-01-02"), slot.Days, now.In(loc))
		}
		// Un instant après l'occurrence, la suivante est strictement plus tard.
		if again, ok := Next(slot, loc, next); !ok || !again.After(next) {
			t.Fatalf("occurrence suivante %s après %s", again.In(loc), local)
		}

		// Previous, lu par le planificateur : la dernière occurrence échue, mêmes règles, et cohérente avec Next.
		prev, ok := Previous(slot, loc, now)
		if !ok || prev.After(now) || now.Sub(prev) > 8*24*time.Hour+2*time.Hour {
			t.Fatalf("occurrence précédente %s (%t), maintenant %s", prev.In(loc), ok, now.In(loc))
		}
		if day, found := intendedDay(prev, loc, int(hh)%24, int(mm)%60); !found || !slices.Contains(slot.Days, isoWeekday(day)) {
			t.Fatalf("occurrence précédente %s pour le %s, jour non programmé %v", prev.In(loc), day.Format("Mon 2006-01-02"), slot.Days)
		}
		if p, ok := Previous(slot, loc, next); !ok || !p.Equal(next) {
			t.Fatalf("Previous(%s) = %s au lieu de l'occurrence elle-même", local, p.In(loc))
		}
		if n, ok := Next(slot, loc, prev); !ok || !n.After(prev) || n.After(next) {
			t.Fatalf("Next(%s) = %s, attendu au plus tard %s", prev.In(loc), n.In(loc), local)
		}
		if !prev.Before(next) {
			t.Fatalf("occurrence précédente %s pas avant la suivante %s", prev.In(loc), local)
		}
	})
}

// intendedDay renvoie le jour (à midi, heure locale) dont l'heure hh:mm donne exactement l'instant t.
func intendedDay(t time.Time, loc *time.Location, hh, mm int) (time.Time, bool) {
	local := t.In(loc)
	for _, delta := range []int{0, 1, -1} {
		d := time.Date(local.Year(), local.Month(), local.Day()+delta, 12, 0, 0, 0, loc)
		if time.Date(d.Year(), d.Month(), d.Day(), hh, mm, 0, 0, loc).Equal(t) {
			return d, true
		}
	}
	return time.Time{}, false
}

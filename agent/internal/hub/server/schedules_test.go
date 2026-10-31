package server

import (
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/hubprotocol"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/settings"
	"github.com/slaynaw/wakeonlan/agent/protocol2"
)

// TestSchedules : programmations lues et changées par un administrateur (docs/PROTOCOLE-HUB.md, §6.7),
// fuseau horaire du hub, règles du réveil (adresse MAC) et de l'extinction (PC avec agent).
func TestSchedules(t *testing.T) {
	e, a := start(t), startAgent(t)
	local := e.local(t)
	pc := e.addPC(t, a, a.invite(t, protocol2.RoleWake, e.hub.Pin)).PC
	box := call(t, local, hubprotocol.Request{Cmd: hubprotocol.CmdPCsAdd, Name: "Box", Host: "192.168.1.1", Probe: &hubprotocol.Probe{Kind: "http"}}).PC
	if pc == nil || box == nil {
		t.Fatal("PC non ajoutés")
	}
	weekdays := &hubprotocol.Slot{On: true, Days: []int{5, 1, 3, 2, 4}, At: "07:30"}
	night := &hubprotocol.Slot{On: true, Days: []int{1, 2, 3, 4, 5, 6, 7}, At: "23:00"}
	set := call(t, local, hubprotocol.Request{Cmd: hubprotocol.CmdSchedulesSet, PC: pc.ID, Schedule: &hubprotocol.Schedule{Wake: weekdays, Shutdown: night}})
	if !set.OK || set.Schedule == nil || set.Schedule.PC != pc.ID || set.Schedule.Wake == nil || set.Schedule.Shutdown == nil {
		t.Fatalf("programmation : %+v", set)
	}
	if w := set.Schedule.Wake; w.Days[0] != 1 || w.Days[4] != 5 || w.At != "07:30" || w.Next <= time.Now().Unix() || w.Next > time.Now().Add(4*24*time.Hour).Unix() {
		t.Fatalf("réveil : %+v", w)
	}
	loc, _ := settings.Location(settings.DefaultTimezone)
	if next := time.Unix(set.Schedule.Shutdown.Next, 0).In(loc); next.Hour() != 23 || next.Minute() != 0 {
		t.Fatalf("extinction : %v", next)
	}
	list := call(t, local, hubprotocol.Request{Cmd: hubprotocol.CmdSchedules})
	if !list.OK || len(list.Schedules) != 1 || list.Schedules[0].PC != pc.ID || list.Settings == nil || list.Settings.Timezone != settings.DefaultTimezone {
		t.Fatalf("programmations : %+v", list)
	}

	// Refus : extinction d'un appareil sans agent, réveil sans adresse MAC, jours ou heure invalides, PC inconnu.
	for _, req := range []hubprotocol.Request{
		{Cmd: hubprotocol.CmdSchedulesSet, PC: box.ID, Schedule: &hubprotocol.Schedule{Shutdown: night}},
		{Cmd: hubprotocol.CmdSchedulesSet, PC: box.ID, Schedule: &hubprotocol.Schedule{Wake: weekdays}},
		{Cmd: hubprotocol.CmdSchedulesSet, PC: pc.ID, Schedule: &hubprotocol.Schedule{Wake: &hubprotocol.Slot{On: true, Days: []int{8}, At: "07:30"}}},
		{Cmd: hubprotocol.CmdSchedulesSet, PC: pc.ID, Schedule: &hubprotocol.Schedule{Wake: &hubprotocol.Slot{On: true, Days: []int{1, 1}, At: "07:30"}}},
		{Cmd: hubprotocol.CmdSchedulesSet, PC: pc.ID, Schedule: &hubprotocol.Schedule{Wake: &hubprotocol.Slot{On: true, Days: []int{1}, At: "24:00"}}},
		{Cmd: hubprotocol.CmdSchedulesSet, PC: "inconnu", Schedule: &hubprotocol.Schedule{Wake: weekdays}},
	} {
		if r := call(t, local, req); r.Code != protocol2.CodeBadRequest {
			t.Errorf("%+v : %+v", req.Schedule, r)
		}
	}
	// La programmation refusée n'a rien changé.
	if list := call(t, local, hubprotocol.Request{Cmd: hubprotocol.CmdSchedules}); len(list.Schedules) != 1 || list.Schedules[0].Wake.Days[0] != 1 {
		t.Fatalf("après les refus : %+v", list.Schedules)
	}

	// Fuseau horaire : changé, puis refusé s'il est inconnu ; la prochaine occurrence le suit.
	if r := call(t, local, hubprotocol.Request{Cmd: hubprotocol.CmdSettings, Timezone: "America/New_York"}); !r.OK || r.Settings.Timezone != "America/New_York" || r.Settings.Retention != 365 {
		t.Fatalf("fuseau horaire : %+v", r)
	}
	if r := call(t, local, hubprotocol.Request{Cmd: hubprotocol.CmdSettings, Timezone: "Lune/Tranquillité"}); r.Code != protocol2.CodeBadRequest {
		t.Errorf("fuseau inconnu : %+v", r)
	}
	ny, _ := settings.Location("America/New_York")
	list = call(t, local, hubprotocol.Request{Cmd: hubprotocol.CmdSchedules})
	if next := time.Unix(list.Schedules[0].Shutdown.Next, 0).In(ny); next.Hour() != 23 {
		t.Fatalf("extinction à New York : %v", next)
	}

	// Désactivée : gardée, sans prochaine occurrence ; retirée ; retirée avec le PC.
	off := *weekdays
	off.On = false
	if r := call(t, local, hubprotocol.Request{Cmd: hubprotocol.CmdSchedulesSet, PC: pc.ID, Schedule: &hubprotocol.Schedule{Wake: &off}}); !r.OK || r.Schedule.Wake.Next != 0 || r.Schedule.Shutdown != nil {
		t.Fatalf("désactivée : %+v", r.Schedule)
	}
	if r := call(t, local, hubprotocol.Request{Cmd: hubprotocol.CmdSchedulesSet, PC: pc.ID}); !r.OK {
		t.Fatalf("retrait : %+v", r)
	}
	if list := call(t, local, hubprotocol.Request{Cmd: hubprotocol.CmdSchedules}); len(list.Schedules) != 0 {
		t.Fatalf("après le retrait : %+v", list.Schedules)
	}
	call(t, local, hubprotocol.Request{Cmd: hubprotocol.CmdSchedulesSet, PC: pc.ID, Schedule: &hubprotocol.Schedule{Wake: weekdays}})
	if r := call(t, local, hubprotocol.Request{Cmd: hubprotocol.CmdPCsRemove, PC: pc.ID}); !r.OK {
		t.Fatalf("retrait du PC : %+v", r)
	}
	if list := call(t, local, hubprotocol.Request{Cmd: hubprotocol.CmdSchedules}); len(list.Schedules) != 0 {
		t.Fatalf("PC retiré : %+v", list.Schedules)
	}
}

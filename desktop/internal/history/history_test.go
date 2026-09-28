package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/slaynaw/wakeonlan/agent/protocol"
	"github.com/slaynaw/wakeonlan/desktop/internal/agentclient"
	"github.com/slaynaw/wakeonlan/desktop/internal/status"
)

type vectors struct {
	Now    int64   `json:"now"`
	Events []Event `json:"events"`
	Agent  struct {
		Device    string           `json:"device"`
		FetchedAt int64            `json:"fetchedAt"`
		History   protocol.History `json:"history"`
	} `json:"agent"`
	Expected map[string][][]any `json:"expected"`
}

func loadVectors(t *testing.T) vectors {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "protocol", "history-vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var v vectors
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func summary(events []Event) [][]any {
	out := [][]any{}
	for _, e := range events {
		out = append(out, []any{string(e.Kind), string(e.Source), float64(e.Time)})
	}
	return out
}

// Scénario partagé avec les tests Kotlin (Android) : même résultat attendu.
func TestSharedHistoryVectors(t *testing.T) {
	v := loadVectors(t)
	d := New()
	for _, e := range v.Events {
		d = d.Add(e, v.Now)
	}
	d = d.ReplaceAgent(v.Agent.Device, v.Agent.History, v.Agent.FetchedAt, v.Now)
	for _, device := range []string{"pc1", "all"} {
		id := device
		if id == "all" {
			id = ""
		}
		got := summary(d.View(id, v.Now))
		if !reflect.DeepEqual(got, v.Expected[device]) {
			t.Errorf("%s :\nobtenu  %v\nattendu %v", device, got, v.Expected[device])
		}
	}
	// La commande d'un autre appareil garde son origine.
	for _, e := range d.View("pc1", v.Now) {
		if e.Kind == SleepSent && e.Client != "192.168.1.50" {
			t.Errorf("client perdu : %+v", e)
		}
	}
	// Nouvelle lecture du journal : l'ancienne version de l'agent est remplacée, pas dupliquée.
	again := d.ReplaceAgent(v.Agent.Device, v.Agent.History, v.Agent.FetchedAt, v.Now)
	if len(again.View("", v.Now)) != len(v.Expected["all"]) {
		t.Error("journal de l'agent dupliqué")
	}
	// PC supprimé : son historique disparaît.
	kept := d.Keep(map[string]bool{"pc2": true})
	if len(kept.View("", v.Now)) != 1 || len(kept.Coverage) != 0 {
		t.Errorf("suppression : %+v", kept)
	}
}

func TestTiesShowLatestRecordedFirst(t *testing.T) {
	now := int64(1_790_600_000_000)
	d := New().Add(Event{Device: "a", Time: now, Kind: ShutdownSent, Source: App}, now)
	d = d.Add(Event{Device: "a", Time: now, Kind: Off, Source: App, Approx: true}, now)
	if v := d.View("a", now); v[0].Kind != Off || v[1].Kind != ShutdownSent {
		t.Fatalf("ordre à heure égale : %+v", v)
	}
}

func TestRetentionAndCapacity(t *testing.T) {
	now := int64(1_790_600_000_000)
	d := New().Add(Event{Device: "a", Time: now - Retention.Milliseconds() - 1, Kind: On, Source: App}, now)
	if len(d.Events) != 0 {
		t.Fatal("évènement de plus de 30 jours conservé")
	}
	for i := 0; i < MaxEvents+5; i++ {
		d.Events = append(d.Events, Event{Device: "a", Time: now - int64(i) - 1, Kind: On, Source: App})
	}
	d = d.Add(Event{Device: "a", Time: now, Kind: Off, Source: App}, now)
	if len(d.Events) != MaxEvents || d.View("a", now)[0].Kind != Off {
		t.Fatalf("capacité : %d", len(d.Events))
	}
}

func ptr(v int64) *int64 { return &v }

func TestTransitions(t *testing.T) {
	now := int64(1_790_600_000_000)
	prev := map[string]status.DeviceStatus{
		"booted":   {State: status.Offline, Since: now - 3_600_000},
		"resumed":  {State: status.Offline, Since: now - 600_000},
		"noagent":  {State: status.Waking, Since: now - 60_000},
		"off":      {State: status.Online, LastSeen: ptr(now - 4_000)},
		"timeout":  {State: status.Waking},
		"unknown":  {State: status.Unknown},
		"same":     {State: status.Online},
		"shutdown": {State: status.ShuttingDown, LastSeen: ptr(now - 9_000)},
		"request":  {State: status.ShuttingDown, Since: now - 2_000, LastSeen: ptr(now - 5_000)},
	}
	cur := map[string]status.DeviceStatus{
		"booted":   {State: status.Online, Agent: &agentclient.Status{Uptime: 120}},
		"resumed":  {State: status.Online, Agent: &agentclient.Status{Uptime: 86_400}},
		"noagent":  {State: status.Online},
		"off":      {State: status.Offline},
		"timeout":  {State: status.Offline, Notice: status.WakeTimeout},
		"unknown":  {State: status.Online},
		"same":     {State: status.Online},
		"shutdown": {State: status.Offline},
		"request":  {State: status.Offline},
		"new":      {State: status.Online},
	}
	got := map[string]Event{}
	for _, e := range Transitions(prev, cur, now) {
		got[e.Device] = e
	}
	want := map[string]Event{
		"booted":   {Device: "booted", Time: now - 120_000, Kind: On, Source: App},
		"resumed":  {Device: "resumed", Time: now, Kind: On, Source: App, Approx: true},
		"noagent":  {Device: "noagent", Time: now, Kind: On, Source: App, Approx: true},
		"off":      {Device: "off", Time: now - 4_000, Kind: Off, Source: App, Approx: true},
		"timeout":  {Device: "timeout", Time: now, Kind: WakeTimeout, Source: App},
		"shutdown": {Device: "shutdown", Time: now - 9_000, Kind: Off, Source: App, Approx: true},
		"request":  {Device: "request", Time: now - 2_000, Kind: Off, Source: App, Approx: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("transitions :\nobtenu  %+v\nattendu %+v", got, want)
	}
}

func TestStore(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if d := s.Load(); len(d.Events) != 0 || d.Coverage == nil {
		t.Fatalf("historique initial : %+v", d)
	}
	now := int64(1_790_600_000_000)
	d := New().Add(Event{Device: "a", Time: now, Kind: WakeSent, Source: App}, now)
	d = d.ReplaceAgent("a", protocol.History{From: now / 1000, Events: []protocol.HistoryEvent{{T: now / 1000, K: "boot"}}}, now, now)
	if err := s.Save(d); err != nil {
		t.Fatal(err)
	}
	if back := s.Load(); !reflect.DeepEqual(back, d) {
		t.Fatalf("relecture :\n%+v\n%+v", back, d)
	}
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte("abîmé"), 0o600); err != nil {
		t.Fatal(err)
	}
	if back := s.Load(); len(back.Events) != 0 {
		t.Fatal("fichier abîmé non ignoré")
	}
}

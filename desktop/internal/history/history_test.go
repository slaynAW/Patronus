package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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
	Expected   map[string][][]any `json:"expected"`
	Clients    map[string]string  `json:"clients"`
	Unreported map[string][]int64 `json:"unreported"`
	Import     Data               `json:"import"`
	Merged     struct {
		PC3 [][]any `json:"pc3"`
		All int     `json:"all"`
	} `json:"merged"`
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
	// Les demandes des autres appareils gardent leur origine (nom, à défaut adresse).
	for _, e := range d.View("pc1", v.Now) {
		if want, ok := v.Clients[string(e.Kind)]; ok && e.Source == Agent && e.Client != want {
			t.Errorf("appareil de %s : %q, attendu %q", e.Kind, e.Client, want)
		}
	}
	// Démarrages demandés ici, inconnus du journal de l'agent : à lui signaler.
	for device, want := range v.Unreported {
		h := protocol.History{}
		if device == v.Agent.Device {
			h = v.Agent.History
		}
		if got := d.UnreportedWakes(device, h); !reflect.DeepEqual(got, want) && (len(got) != 0 || len(want) != 0) {
			t.Errorf("démarrages à signaler (%s) : %v, attendu %v", device, got, want)
		}
	}
	// Sauvegarde importée : fusionnée sans doublon, deux fois de suite sans effet.
	merged := d.Merge(v.Import, v.Now)
	if got := summary(merged.View("pc3", v.Now)); !reflect.DeepEqual(got, v.Merged.PC3) {
		t.Errorf("pc3 importé :\nobtenu  %v\nattendu %v", got, v.Merged.PC3)
	}
	if n := len(merged.View("", v.Now)); n != v.Merged.All || len(merged.Merge(v.Import, v.Now).View("", v.Now)) != n {
		t.Errorf("fusion : %d évènements affichés, attendu %d", n, v.Merged.All)
	}
	if len(d.View("", v.Now)) != len(v.Expected["all"]) {
		t.Error("la fusion a modifié l'historique d'origine")
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
	if d, err := s.Load(); err != nil || len(d.Events) != 0 || d.Coverage == nil {
		t.Fatalf("historique initial : %+v %v", d, err)
	}
	now := int64(1_790_600_000_000)
	d := New().Add(Event{Device: "a", Time: now, Kind: WakeSent, Source: App}, now)
	d = d.ReplaceAgent("a", protocol.History{From: now / 1000, Events: []protocol.HistoryEvent{{T: now / 1000, K: "boot"}}}, now, now)
	if err := s.Save(d); err != nil {
		t.Fatal(err)
	}
	if back, err := s.Load(); err != nil || !reflect.DeepEqual(back, d) {
		t.Fatalf("relecture :\n%+v\n%+v\n%v", back, d, err)
	}
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte("abîmé"), 0o600); err != nil {
		t.Fatal(err)
	}
	back, err := s.Load()
	if len(back.Events) != 0 || err == nil || !strings.Contains(err.Error(), "mis de côté") {
		t.Fatalf("fichier abîmé non ignoré : %v", err)
	}
	// Mis de côté, jamais supprimé ; le lancement suivant repart d'un historique vide sans erreur.
	if kept, _ := filepath.Glob(filepath.Join(dir, fileName+".illisible-*")); len(kept) != 1 {
		t.Errorf("copie du fichier abîmé : %v", kept)
	}
	if _, err := s.Load(); err != nil {
		t.Errorf("après mise de côté : %v", err)
	}
}

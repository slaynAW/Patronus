package measures

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol"
)

// Ce que l'agent produit passe la validation des applications (protocol.ParseMeasures) à l'identique.
func valid(t *testing.T, m *protocol.Measure) protocol.Measure {
	t.Helper()
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	back := protocol.ParseMeasures([]json.RawMessage{raw})
	if len(back) != 1 {
		t.Fatalf("mesure refusée : %s", raw)
	}
	return back[0]
}

func TestMemory(t *testing.T) {
	m := valid(t, memoryMeasure(16<<30, 9<<30+644245094))
	if m.ID != "mem" || m.Value != 40 || m.Unit != "%" || m.Text != "6,4 Go sur 16 Go" || m.Level() != protocol.LevelNone {
		t.Errorf("%+v", m)
	}
	if m := valid(t, memoryMeasure(2<<30, 50<<20)); m.Value != 97.6 || m.Level() != protocol.LevelCrit || m.Text != "2 Go sur 2 Go" {
		t.Errorf("mémoire presque pleine : %+v", m)
	}
	if memoryMeasure(0, 0) != nil || memoryMeasure(1<<30, 2<<30) != nil {
		t.Error("relevé incohérent accepté")
	}
}

func TestBattery(t *testing.T) {
	m := valid(t, batteryMeasure(18, discharging))
	if m.ID != "battery" || m.Value != 18 || !m.Low || m.Text != "sur batterie" || m.Level() != protocol.LevelWarn {
		t.Errorf("%+v", m)
	}
	if m := valid(t, batteryMeasure(100, full)); m.Level() != protocol.LevelNone {
		t.Errorf("batterie pleine : %+v", m)
	}
	if batteryMeasure(-1, "") != nil || batteryMeasure(101, "") != nil {
		t.Error("pourcentage impossible accepté")
	}
}

func TestGigabytes(t *testing.T) {
	for b, want := range map[uint64]string{16 << 30: "16 Go", 6871947674: "6,4 Go", 512 << 20: "0,5 Go", 0: "0 Go"} {
		if got := gigabytes(b); got != want {
			t.Errorf("%d : %q, attendu %q", b, got, want)
		}
	}
}

func TestParseMeminfo(t *testing.T) {
	text := "MemTotal:        2007644 kB\nMemFree:          120000 kB\nMemAvailable:     980000 kB\nBuffers:           10 kB\n"
	total, available, ok := parseMeminfo(text)
	if !ok || total != 2007644*1024 || available != 980000*1024 {
		t.Errorf("%d %d %v", total, available, ok)
	}
	// Noyau très ancien sans MemAvailable : pas de mesure plutôt qu'une mesure fausse.
	if _, _, ok := parseMeminfo("MemTotal: 2007644 kB\nMemFree: 120000 kB\n"); ok {
		t.Error("MemAvailable absent accepté")
	}
}

func TestCache(t *testing.T) {
	calls := 0
	now := time.Unix(1000, 0)
	c := NewCache(func() []protocol.Measure { calls++; return nil })
	c.now = func() time.Time { return now }
	c.Get()
	c.Get()
	now = now.Add(refreshEvery)
	c.Get()
	if calls != 2 {
		t.Errorf("%d lectures", calls)
	}
}

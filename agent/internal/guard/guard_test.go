package guard

import (
	"testing"
	"time"
)

func TestLimiter(t *testing.T) {
	now := time.Date(2026, 10, 12, 15, 0, 0, 0, time.UTC)
	l := NewLimiter(5, time.Minute, 5*time.Minute)
	l.now = func() time.Time { return now }
	for i := range 4 {
		if l.Fail("192.168.1.9") {
			t.Fatalf("bloqué après %d échecs", i+1)
		}
	}
	if l.Blocked("192.168.1.9") {
		t.Fatal("bloqué avant le cinquième échec")
	}
	if !l.Fail("192.168.1.9") || !l.Blocked("192.168.1.9") {
		t.Fatal("pas bloqué après 5 échecs en une minute")
	}
	if l.Blocked("192.168.1.10") {
		t.Error("une autre adresse est bloquée")
	}
	now = now.Add(5*time.Minute + time.Second)
	if l.Blocked("192.168.1.9") {
		t.Error("toujours bloqué après 5 minutes")
	}
	// Échecs espacés de plus d'une minute, ou effacés par une réussite : jamais de blocage.
	for range 10 {
		if l.Fail("192.168.1.11") {
			t.Fatal("bloqué par des échecs espacés")
		}
		now = now.Add(61 * time.Second)
	}
	for range 10 {
		l.Fail("192.168.1.12")
		l.Success("192.168.1.12")
	}
	if l.Blocked("192.168.1.12") {
		t.Error("bloqué malgré les réussites")
	}
}

func TestPairing(t *testing.T) {
	now := time.Date(2026, 10, 12, 15, 0, 0, 0, time.UTC)
	g := NewPairing(func() time.Time { return now })
	for i := range PairingMaxFailures {
		if g.Fail() {
			t.Fatalf("suspendu après %d échecs", i+1)
		}
		now = now.Add(10 * time.Second)
	}
	if !g.Fail() || !g.Suspended() {
		t.Fatal("pas suspendu après 21 échecs en 10 minutes")
	}
	now = now.Add(PairingPause)
	if g.Suspended() {
		t.Error("toujours suspendu après 10 minutes")
	}
	// Échecs espacés de plus de 10 minutes : jamais de suspension.
	for range 3 * PairingMaxFailures {
		if g.Fail() {
			t.Fatal("suspendu par des échecs espacés")
		}
		now = now.Add(PairingWindow / PairingMaxFailures)
	}
}

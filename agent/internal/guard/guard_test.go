package guard

import (
	"fmt"
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

// TestLimiterMemory : échecs venus de milliers d'adresses, puis oubliés ; la mémoire ne garde que ce qui
// sert encore (fenêtre d'échecs, blocages en cours), et jamais plus de maxTracked adresses en échec.
func TestLimiterMemory(t *testing.T) {
	now := time.Date(2026, 11, 6, 3, 0, 0, 0, time.UTC)
	l := NewLimiter(5, time.Minute, 5*time.Minute)
	l.now = func() time.Time { return now }
	for i := range 1000 {
		l.Fail(fmt.Sprintf("fd00::%x", i))
	}
	for range 5 {
		l.Fail("192.168.1.9")
	}
	if len(l.failures) < 1000 || !l.Blocked("192.168.1.9") {
		t.Fatalf("échecs récents : %d adresses suivies", len(l.failures))
	}
	// Une fenêtre plus tard, la purge suivante oublie les échecs ; le blocage, encore en cours, reste.
	now = now.Add(time.Minute + time.Second)
	for range l.sweepEvery {
		l.Fail("192.168.1.30")
		l.Success("192.168.1.30")
	}
	if len(l.failures) > 1 || !l.Blocked("192.168.1.9") {
		t.Fatalf("après la fenêtre : %d adresses suivies, blocage %t", len(l.failures), l.Blocked("192.168.1.9"))
	}
	// Blocage expiré : purgé aussi, même si l'adresse ne revient jamais.
	now = now.Add(5 * time.Minute)
	for range l.sweepEvery {
		l.Fail("192.168.1.30")
		l.Success("192.168.1.30")
	}
	if len(l.blocked) != 0 {
		t.Fatalf("blocages expirés gardés : %v", l.blocked)
	}

	// Plafond : au-delà de maxTracked adresses en échec dans la fenêtre, les compteurs repartent de zéro.
	l.maxTracked = 100
	for i := range 1000 {
		l.Fail(fmt.Sprintf("fd00::1:%x", i))
		if len(l.failures) > l.maxTracked {
			t.Fatalf("%d adresses suivies (plafond %d)", len(l.failures), l.maxTracked)
		}
	}
	// Le blocage d'une adresse insistante fonctionne toujours.
	for range 5 {
		l.Fail("192.168.1.66")
	}
	if !l.Blocked("192.168.1.66") {
		t.Error("blocage perdu sous le plafond")
	}
}

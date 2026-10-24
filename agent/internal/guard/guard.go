// Package guard limite les tentatives d'authentification et d'appairage (docs/PROTOCOLE-V2.md, §3.3),
// pour l'agent comme pour le hub : blocage d'une adresse après trop d'échecs, et suspension de
// l'appairage après trop de codes refusés, toutes adresses confondues.
package guard

import (
	"sync"
	"time"
)

// Limiter bloque temporairement une adresse IP après trop d'échecs d'authentification, ce qui rend
// toute attaque par force brute irréaliste.
type Limiter struct {
	mu        sync.Mutex
	failures  map[string][]time.Time
	blocked   map[string]time.Time
	maxFails  int
	window    time.Duration
	blockTime time.Duration
	now       func() time.Time
}

// NewLimiter bloque pendant blockTime une adresse qui a échoué maxFails fois en window.
func NewLimiter(maxFails int, window, blockTime time.Duration) *Limiter {
	return &Limiter{
		failures:  map[string][]time.Time{},
		blocked:   map[string]time.Time{},
		maxFails:  maxFails,
		window:    window,
		blockTime: blockTime,
		now:       time.Now,
	}
}

// Blocked indique si l'adresse est actuellement bloquée.
func (r *Limiter) Blocked(ip string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	until, ok := r.blocked[ip]
	if !ok {
		return false
	}
	if r.now().After(until) {
		delete(r.blocked, ip)
		return false
	}
	return true
}

// Fail enregistre un échec ; renvoie vrai si l'adresse vient d'être bloquée.
func (r *Limiter) Fail(ip string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	recent := r.failures[ip][:0]
	for _, t := range r.failures[ip] {
		if now.Sub(t) < r.window {
			recent = append(recent, t)
		}
	}
	recent = append(recent, now)
	if len(recent) >= r.maxFails {
		delete(r.failures, ip)
		r.blocked[ip] = now.Add(r.blockTime)
		return true
	}
	r.failures[ip] = recent
	return false
}

// Success efface l'historique d'échecs d'une adresse.
func (r *Limiter) Success(ip string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.failures, ip)
}

// Suspension de l'appairage (§3.3) : plus de PairingMaxFailures codes refusés en PairingWindow, toutes
// adresses confondues.
const (
	PairingMaxFailures = 20
	PairingWindow      = 10 * time.Minute
	PairingPause       = 10 * time.Minute
)

// Pairing suspend l'appairage après trop de codes refusés.
type Pairing struct {
	mu       sync.Mutex
	failures []time.Time
	until    time.Time
	now      func() time.Time
}

// NewPairing prépare la garde de l'appairage, avec l'horloge now.
func NewPairing(now func() time.Time) *Pairing { return &Pairing{now: now} }

// Suspended indique si l'appairage est suspendu.
func (g *Pairing) Suspended() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.now().Before(g.until)
}

// Fail note un code refusé ; vrai si l'appairage vient d'être suspendu.
func (g *Pairing) Fail() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	recent := g.failures[:0]
	for _, t := range g.failures {
		if now.Sub(t) < PairingWindow {
			recent = append(recent, t)
		}
	}
	g.failures = append(recent, now)
	if len(g.failures) > PairingMaxFailures {
		g.failures = nil
		g.until = now.Add(PairingPause)
		return true
	}
	return false
}

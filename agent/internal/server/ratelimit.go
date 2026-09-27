package server

import (
	"sync"
	"time"
)

// rateLimiter bloque temporairement une adresse IP après trop d'échecs d'authentification,
// ce qui rend toute attaque par force brute sur la clé irréaliste (en plus de ses 256 bits).
type rateLimiter struct {
	mu        sync.Mutex
	failures  map[string][]time.Time
	blocked   map[string]time.Time
	maxFails  int
	window    time.Duration
	blockTime time.Duration
	now       func() time.Time
}

func newRateLimiter(maxFails int, window, blockTime time.Duration) *rateLimiter {
	return &rateLimiter{
		failures:  map[string][]time.Time{},
		blocked:   map[string]time.Time{},
		maxFails:  maxFails,
		window:    window,
		blockTime: blockTime,
		now:       time.Now,
	}
}

// Blocked indique si l'adresse est actuellement bloquée.
func (r *rateLimiter) Blocked(ip string) bool {
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
func (r *rateLimiter) Fail(ip string) bool {
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
func (r *rateLimiter) Success(ip string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.failures, ip)
}

// Package guard limite les tentatives d'authentification et d'appairage (docs/PROTOCOLE-V2.md, §3.3),
// pour l'agent comme pour le hub : blocage d'une adresse après trop d'échecs, et suspension de
// l'appairage après trop de codes refusés, toutes adresses confondues.
package guard

import (
	"errors"
	"io"
	"net"
	"sync"
	"syscall"
	"time"
)

// Limiter bloque temporairement une adresse IP après trop d'échecs d'authentification, ce qui rend
// toute attaque par force brute irréaliste.
//
// Sa mémoire est bornée, même face à des échecs venus d'un très grand nombre d'adresses (IPv6) :
// adresses en échec et blocages expirés sont purgés régulièrement, et au-delà de maxTracked adresses en
// échec à la fois, leurs compteurs repartent de zéro (les blocages en cours sont gardés).
type Limiter struct {
	mu        sync.Mutex
	failures  map[string][]time.Time
	blocked   map[string]time.Time
	maxFails  int
	window    time.Duration
	blockTime time.Duration
	now       func() time.Time

	calls      int // échecs notés, pour la purge régulière
	sweepEvery int
	maxTracked int
}

// Mémoire du Limiter : purge tous les defaultSweepEvery échecs, defaultMaxTracked adresses en échec au plus.
const (
	defaultSweepEvery = 256
	defaultMaxTracked = 1 << 16
)

// NewLimiter bloque pendant blockTime une adresse qui a échoué maxFails fois en window.
func NewLimiter(maxFails int, window, blockTime time.Duration) *Limiter {
	return &Limiter{
		failures:   map[string][]time.Time{},
		blocked:    map[string]time.Time{},
		maxFails:   maxFails,
		window:     window,
		blockTime:  blockTime,
		now:        time.Now,
		sweepEvery: defaultSweepEvery,
		maxTracked: defaultMaxTracked,
	}
}

// sweep retire les adresses dont le dernier échec sort de la fenêtre et les blocages expirés ; au-delà
// de maxTracked adresses encore en échec, il les oublie toutes. Appelé verrou pris.
func (r *Limiter) sweep(now time.Time) {
	for ip, times := range r.failures {
		if len(times) == 0 || now.Sub(times[len(times)-1]) >= r.window {
			delete(r.failures, ip)
		}
	}
	for ip, until := range r.blocked {
		if now.After(until) {
			delete(r.blocked, ip)
		}
	}
	if len(r.failures) >= r.maxTracked {
		r.failures = map[string][]time.Time{}
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
	if r.calls++; r.calls%r.sweepEvery == 0 || len(r.failures) >= r.maxTracked {
		r.sweep(now)
	}
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

// Refused distingue un refus de TLS (version, ALPN, certificat), compté comme un échec de l'adresse,
// d'une connexion simplement interrompue (sonde TCP, délai dépassé) ou refusée par le client lui-même.
func Refused(err error) bool {
	var ne net.Error
	var op *net.OpError
	switch {
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, net.ErrClosed),
		errors.Is(err, syscall.ECONNRESET), errors.As(err, &ne) && ne.Timeout():
		return false
	case errors.As(err, &op) && op.Op == "remote error": // alerte du client : son refus, pas le nôtre
		return false
	}
	return true
}

// Package wol construit et envoie le « paquet magique » Wake-on-LAN.
package wol

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"time"

	"github.com/slaynaw/wakeonlan/desktop/internal/model"
)

const (
	headerSize  = 6
	repetitions = 16
	// PacketSize est la taille d'un paquet sans mot de passe SecureOn.
	PacketSize = headerSize + repetitions*model.MACLength
	// DefaultRepeat : chaque destination reçoit le paquet plusieurs fois (l'UDP ne garantit rien et
	// certaines cartes ratent le premier paquet).
	DefaultRepeat = 3
	// DefaultInterval sépare deux envois successifs.
	DefaultInterval = 120 * time.Millisecond
)

// Limited est l'adresse de diffusion limitée.
var Limited = netip.AddrFrom4([4]byte{255, 255, 255, 255})

// BuildPacket construit le paquet : 6 octets 0xFF, 16 fois l'adresse MAC, puis le mot de passe
// SecureOn (6 octets) éventuel.
func BuildPacket(mac model.MAC, secureOn []byte) ([]byte, error) {
	if secureOn != nil && len(secureOn) != 6 {
		return nil, errors.New("Le mot de passe SecureOn fait 6 octets")
	}
	packet := make([]byte, 0, PacketSize+len(secureOn))
	for range headerSize {
		packet = append(packet, 0xFF)
	}
	for range repetitions {
		packet = append(packet, mac[:]...)
	}
	return append(packet, secureOn...), nil
}

// Directed renvoie l'adresse de diffusion d'un sous-réseau (192.168.1.37/24 → 192.168.1.255),
// ou false pour les préfixes /31 et /32 qui n'en ont pas.
func Directed(prefix netip.Prefix) (netip.Addr, bool) {
	if !prefix.Addr().Is4() || prefix.Bits() < 0 || prefix.Bits() >= 31 {
		return netip.Addr{}, false
	}
	ip := prefix.Addr().As4()
	host := uint32(0xFFFFFFFF) >> prefix.Bits()
	value := (uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])) | host
	return netip.AddrFrom4([4]byte{byte(value >> 24), byte(value >> 16), byte(value >> 8), byte(value)}), true
}

// Route est une carte réseau locale depuis laquelle émettre.
type Route struct {
	// Source est l'adresse IPv4 de la carte (la socket y est attachée pour sortir par cette carte).
	Source netip.Prefix
}

// Result décrit un envoi.
type Result struct {
	PacketsSent  int      `json:"packetsSent"`
	Destinations []string `json:"destinations"`
	Errors       []string `json:"errors"`
}

// Success indique qu'au moins un paquet est parti.
func (r Result) Success() bool { return r.PacketsSent > 0 }

// Request décrit un réveil.
type Request struct {
	Packet []byte
	// Override est l'adresse de diffusion forcée par l'utilisateur (invalide = aucune).
	Override netip.Addr
	Port     int
	// Routes sont les cartes réseau locales ; vide = laisser le système choisir.
	Routes   []Route
	Repeat   int
	Interval time.Duration
}

type target struct {
	source netip.Addr // invalide = socket non attachée
	dest   netip.Addr
}

// Targets calcule les couples (carte, destination) : diffusion forcée par le système, puis pour
// chaque carte sa diffusion dirigée et la diffusion limitée 255.255.255.255 (émise par CETTE carte :
// sous Windows, un PC avec plusieurs cartes n'émet sinon la diffusion limitée que sur l'une d'elles).
func Targets(override netip.Addr, routes []Route) []target {
	var out []target
	seen := map[target]bool{}
	add := func(t target) {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	if override.IsValid() {
		add(target{dest: override})
	}
	for _, r := range routes {
		if directed, ok := Directed(r.Source); ok {
			add(target{source: r.Source.Addr(), dest: directed})
		}
		add(target{source: r.Source.Addr(), dest: Limited})
	}
	if len(routes) == 0 {
		add(target{dest: Limited})
	}
	return out
}

// Send envoie le paquet vers toutes les destinations. Une destination en échec n'empêche pas les autres.
func Send(ctx context.Context, req Request) Result {
	if req.Repeat < 1 {
		req.Repeat = DefaultRepeat
	}
	if req.Interval <= 0 {
		req.Interval = DefaultInterval
	}
	targets := Targets(req.Override, req.Routes)
	result := Result{Destinations: []string{}, Errors: []string{}}
	conns := map[netip.Addr]*net.UDPConn{}
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()
	errSeen := map[string]bool{}
	addErr := func(msg string) {
		if !errSeen[msg] {
			errSeen[msg] = true
			result.Errors = append(result.Errors, msg)
		}
	}
	for _, t := range targets {
		result.Destinations = append(result.Destinations, fmt.Sprintf("%s:%d", t.dest, req.Port))
	}
	for round := range req.Repeat {
		if round > 0 {
			select {
			case <-ctx.Done():
				return result
			case <-time.After(req.Interval):
			}
		}
		for _, t := range targets {
			conn, ok := conns[t.source]
			if !ok {
				var laddr *net.UDPAddr
				if t.source.IsValid() {
					laddr = net.UDPAddrFromAddrPort(netip.AddrPortFrom(t.source, 0))
				}
				// Go active SO_BROADCAST sur les sockets UDP : la diffusion est autorisée.
				c, err := net.ListenUDP("udp4", laddr)
				if err != nil {
					addErr(fmt.Sprintf("%s : %v", t.source, err))
					conns[t.source] = nil
					continue
				}
				conns[t.source], conn = c, c
			}
			if conn == nil {
				continue
			}
			dest := net.UDPAddrFromAddrPort(netip.AddrPortFrom(t.dest, uint16(req.Port)))
			if _, err := conn.WriteToUDP(req.Packet, dest); err != nil {
				addErr(fmt.Sprintf("%s:%d : %v", t.dest, req.Port, err))
				continue
			}
			result.PacketsSent++
		}
	}
	return result
}

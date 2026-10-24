// Package wake envoie le paquet magique Wake-on-LAN depuis le NAS (docs/HUB.md, §8) : diffusion limitée et
// diffusion dirigée de chaque réseau privé du NAS, trois fois, comme les applications. C'est la raison du
// réseau de l'hôte pour le conteneur : une diffusion ne sort pas d'un réseau Docker ponté.
package wake

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol2"
)

const (
	// Port est le port de destination habituel du paquet magique.
	Port = 9
	// repeat et interval : chaque destination reçoit le paquet trois fois (l'UDP ne garantit rien, et
	// certaines cartes ratent le premier).
	repeat   = 3
	interval = 120 * time.Millisecond
)

// Limited est l'adresse de diffusion limitée.
var Limited = netip.AddrFrom4([4]byte{255, 255, 255, 255})

// Packet construit le paquet : 6 octets 0xFF puis 16 fois l'adresse MAC (« AA:BB:CC:DD:EE:FF »).
func Packet(mac string) ([]byte, error) {
	normalized, ok := protocol2.NormalizeMAC(mac)
	if !ok {
		return nil, errors.New("adresse MAC invalide")
	}
	hw, err := net.ParseMAC(normalized)
	if err != nil {
		return nil, err
	}
	packet := make([]byte, 0, 6+16*6)
	for range 6 {
		packet = append(packet, 0xFF)
	}
	for range 16 {
		packet = append(packet, hw...)
	}
	return packet, nil
}

// Directed renvoie l'adresse de diffusion d'un sous-réseau IPv4 (192.168.1.20/24 → 192.168.1.255), ou
// false pour les préfixes /31 et /32 qui n'en ont pas.
func Directed(prefix netip.Prefix) (netip.Addr, bool) {
	if !prefix.Addr().Is4() || prefix.Bits() < 0 || prefix.Bits() >= 31 {
		return netip.Addr{}, false
	}
	ip := prefix.Addr().As4()
	host := uint32(0xFFFFFFFF) >> prefix.Bits()
	v := uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3]) | host
	return netip.AddrFrom4([4]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}), true
}

// Targets renvoie les destinations : diffusion limitée, puis diffusion dirigée de chaque réseau IPv4 privé
// des cartes réseau données (prefixes : adresse et masque de chaque carte).
func Targets(prefixes []netip.Prefix, port int) []netip.AddrPort {
	out := []netip.AddrPort{netip.AddrPortFrom(Limited, uint16(port))}
	seen := map[netip.Addr]bool{Limited: true}
	for _, p := range prefixes {
		if !p.Addr().Is4() || !p.Addr().IsPrivate() {
			continue
		}
		if d, ok := Directed(p); ok && !seen[d] {
			seen[d] = true
			out = append(out, netip.AddrPortFrom(d, uint16(port)))
		}
	}
	return out
}

// LocalPrefixes renvoie l'adresse et le masque de chaque carte réseau active du NAS.
func LocalPrefixes() []netip.Prefix {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []netip.Prefix
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			if ipNet, ok := a.(*net.IPNet); ok {
				if p, err := netip.ParsePrefix(ipNet.String()); err == nil {
					out = append(out, p)
				}
			}
		}
	}
	return out
}

// Send envoie le paquet magique de mac depuis le NAS ; renvoie le nombre de paquets partis.
func Send(ctx context.Context, mac string) (int, error) {
	packet, err := Packet(mac)
	if err != nil {
		return 0, err
	}
	return SendTo(ctx, packet, Targets(LocalPrefixes(), Port))
}

// SendTo envoie packet à chaque destination, trois fois ; une destination en échec n'empêche pas les
// autres. L'erreur n'est renvoyée que si aucun paquet n'est parti.
func SendTo(ctx context.Context, packet []byte, targets []netip.AddrPort) (int, error) {
	// Go active SO_BROADCAST sur les sockets UDP : la diffusion est permise.
	conn, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	sent := 0
	var last error
	for round := range repeat {
		if round > 0 {
			select {
			case <-ctx.Done():
				return sent, nil
			case <-time.After(interval):
			}
		}
		for _, t := range targets {
			if _, err := conn.WriteToUDPAddrPort(packet, t); err != nil {
				last = fmt.Errorf("%s : %w", t, err)
				continue
			}
			sent++
		}
	}
	if sent == 0 {
		return 0, last
	}
	return sent, nil
}

// Package netstate décrit le réseau local de l'ordinateur (équivalent de LanNetworkMonitor côté
// Android) : cartes Ethernet / Wi-Fi actives, leurs adresses (pour la diffusion du paquet magique)
// et la présence d'un VPN.
package netstate

import (
	"fmt"
	"net/netip"
	"sort"
	"strings"
)

// Transport est le type de carte réseau.
type Transport string

const (
	WiFi     Transport = "wifi"
	Ethernet Transport = "ethernet"
)

// Interface est une carte réseau locale utilisable.
type Interface struct {
	Name       string
	Transport  Transport
	Addresses  []netip.Prefix // IPv4 uniquement
	HasGateway bool
}

// State est l'état du réseau local.
type State struct {
	Interfaces []Interface
	// VPNActive : un VPN (WireGuard, Tailscale, OpenVPN...) est actif.
	VPNActive bool
}

// Connected indique qu'au moins une carte Ethernet / Wi-Fi a une adresse IPv4.
func (s State) Connected() bool { return len(s.Interfaces) > 0 }

// Primary renvoie la carte principale : celle qui a une passerelle, Ethernet de préférence.
func (s State) Primary() (Interface, bool) {
	if len(s.Interfaces) == 0 {
		return Interface{}, false
	}
	return s.Interfaces[0], true
}

// PrimaryAddress renvoie « 192.168.1.20/24 » pour l'affichage, ou "".
func (s State) PrimaryAddress() string {
	if p, ok := s.Primary(); ok && len(p.Addresses) > 0 {
		return p.Addresses[0].String()
	}
	return ""
}

// Prefixes renvoie toutes les adresses IPv4 locales (pour la diffusion dirigée).
func (s State) Prefixes() []netip.Prefix {
	var out []netip.Prefix
	for _, i := range s.Interfaces {
		out = append(out, i.Addresses...)
	}
	return out
}

// Key résume l'état pour détecter un changement.
func (s State) Key() string {
	var b strings.Builder
	for _, i := range s.Interfaces {
		fmt.Fprintf(&b, "%s|%s|%v|%v;", i.Name, i.Transport, i.Addresses, i.HasGateway)
	}
	fmt.Fprintf(&b, "vpn=%v", s.VPNActive)
	return b.String()
}

// sortInterfaces place en tête les cartes avec passerelle, puis Ethernet avant Wi-Fi (plus fiable).
func sortInterfaces(list []Interface) {
	sort.SliceStable(list, func(a, b int) bool {
		if list[a].HasGateway != list[b].HasGateway {
			return list[a].HasGateway
		}
		return list[a].Transport == Ethernet && list[b].Transport == WiFi
	})
}

// isUsableIPv4 écarte les adresses sans intérêt pour le réseau local.
func isUsableIPv4(addr netip.Addr) bool {
	addr = addr.Unmap()
	return addr.Is4() && !addr.IsLoopback() && !addr.IsUnspecified() && !addr.IsLinkLocalUnicast()
}

// isVirtualAdapter reconnaît les cartes des machines virtuelles (Hyper-V, VirtualBox, VMware, WSL,
// Docker...) : ce ne sont pas des réseaux physiques où se trouvent les PC à réveiller.
func isVirtualAdapter(name string) bool {
	n := strings.ToLower(name)
	for _, word := range []string{"virtual", "hyper-v", "vmware", "virtualbox", "host-only", "vethernet", "wsl", "docker", "vbox", "veth", "virbr", "br-"} {
		if strings.Contains(n, word) {
			return true
		}
	}
	return false
}

// isVPNAdapter reconnaît les cartes des VPN courants (nom ou description).
func isVPNAdapter(name string) bool {
	n := strings.ToLower(name)
	for _, word := range []string{"vpn", "wireguard", "wintun", "tailscale", "tap-windows", "zerotier", "openconnect"} {
		if strings.Contains(n, word) {
			return true
		}
	}
	for _, prefix := range []string{"tun", "tap", "wg", "utun", "ppp", "zt"} {
		if strings.HasPrefix(n, prefix) && !strings.HasPrefix(n, "tunnel") {
			return true
		}
	}
	return false
}

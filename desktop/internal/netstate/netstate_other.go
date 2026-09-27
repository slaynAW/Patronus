//go:build !windows

package netstate

import (
	"net"
	"net/netip"
	"strings"
)

// Current lit l'état des cartes réseau (hors Windows : développement ; type déduit du nom).
func Current() (State, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return State{}, err
	}
	var state State
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		var prefixes []netip.Prefix
		for _, a := range addrs {
			ipNet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip, ok := netip.AddrFromSlice(ipNet.IP)
			ones, _ := ipNet.Mask.Size()
			if ok && isUsableIPv4(ip) {
				prefixes = append(prefixes, netip.PrefixFrom(ip.Unmap(), ones))
			}
		}
		if len(prefixes) == 0 {
			continue
		}
		name := strings.ToLower(iface.Name)
		switch {
		case isVPNAdapter(name):
			state.VPNActive = true
		case isVirtualAdapter(name):
		case strings.HasPrefix(name, "wl"):
			state.Interfaces = append(state.Interfaces, Interface{Name: iface.Name, Transport: WiFi, Addresses: prefixes, HasGateway: true})
		default:
			state.Interfaces = append(state.Interfaces, Interface{Name: iface.Name, Transport: Ethernet, Addresses: prefixes, HasGateway: true})
		}
	}
	sortInterfaces(state.Interfaces)
	return state, nil
}

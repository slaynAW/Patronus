package netstate

import (
	"errors"
	"net/netip"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Current lit l'état des cartes réseau (GetAdaptersAddresses).
func Current() (State, error) {
	size := uint32(16 * 1024)
	var buf []byte
	var first *windows.IpAdapterAddresses
	for range 4 {
		buf = make([]byte, size)
		first = (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))
		err := windows.GetAdaptersAddresses(windows.AF_INET,
			windows.GAA_FLAG_INCLUDE_GATEWAYS|windows.GAA_FLAG_SKIP_ANYCAST|windows.GAA_FLAG_SKIP_MULTICAST|windows.GAA_FLAG_SKIP_DNS_SERVER,
			0, first, &size)
		if err == nil {
			break
		}
		if !errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) {
			return State{}, err
		}
		first = nil
	}
	if first == nil {
		return State{}, errors.New("liste des cartes réseau indisponible")
	}

	var state State
	for a := first; a != nil; a = a.Next {
		if a.OperStatus != windows.IfOperStatusUp || a.IfType == windows.IF_TYPE_SOFTWARE_LOOPBACK {
			continue
		}
		var prefixes []netip.Prefix
		for u := a.FirstUnicastAddress; u != nil; u = u.Next {
			ip, ok := netip.AddrFromSlice(u.Address.IP())
			if !ok || !isUsableIPv4(ip) {
				continue
			}
			if prefix, err := ip.Unmap().Prefix(int(u.OnLinkPrefixLength)); err == nil {
				prefixes = append(prefixes, netip.PrefixFrom(ip.Unmap(), prefix.Bits()))
			}
		}
		if len(prefixes) == 0 {
			continue
		}
		name := windows.UTF16PtrToString(a.FriendlyName)
		description := windows.UTF16PtrToString(a.Description)
		var transport Transport
		switch a.IfType {
		case windows.IF_TYPE_ETHERNET_CSMACD:
			transport = Ethernet
		case windows.IF_TYPE_IEEE80211:
			transport = WiFi
		}
		switch {
		case transport == "" || isVPNAdapter(description):
			// Tunnel, PPP, carte virtuelle d'un VPN (Wintun, TAP...) : VPN.
			state.VPNActive = true
		case isVirtualAdapter(description) || isVirtualAdapter(name):
			// Machines virtuelles : ignorées.
		default:
			state.Interfaces = append(state.Interfaces, Interface{
				Name: name, Transport: transport, Addresses: prefixes, HasGateway: a.FirstGatewayAddress != nil,
			})
		}
	}
	sortInterfaces(state.Interfaces)
	return state, nil
}

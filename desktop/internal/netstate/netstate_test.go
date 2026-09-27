package netstate

import (
	"net/netip"
	"testing"
)

func TestPrimaryAndClassification(t *testing.T) {
	list := []Interface{
		{Name: "Wi-Fi", Transport: WiFi, Addresses: []netip.Prefix{netip.MustParsePrefix("192.168.1.30/24")}, HasGateway: true},
		{Name: "Ethernet 2", Transport: Ethernet, Addresses: []netip.Prefix{netip.MustParsePrefix("10.0.0.5/8")}},
		{Name: "Ethernet", Transport: Ethernet, Addresses: []netip.Prefix{netip.MustParsePrefix("192.168.1.20/24")}, HasGateway: true},
	}
	sortInterfaces(list)
	s := State{Interfaces: list}
	if p, _ := s.Primary(); p.Name != "Ethernet" || s.PrimaryAddress() != "192.168.1.20/24" || len(s.Prefixes()) != 3 {
		t.Errorf("carte principale : %+v", list)
	}
	if (State{}).Connected() || !s.Connected() || s.Key() == (State{}).Key() {
		t.Error("état de connexion incorrect")
	}
	for _, name := range []string{"Hyper-V Virtual Ethernet Adapter", "VirtualBox Host-Only Ethernet Adapter", "vEthernet (WSL)", "docker0"} {
		if !isVirtualAdapter(name) {
			t.Errorf("%s devrait être virtuelle", name)
		}
	}
	for _, name := range []string{"TAP-Windows Adapter V9", "WireGuard Tunnel", "Tailscale Tunnel", "tun0", "wg0"} {
		if !isVPNAdapter(name) {
			t.Errorf("%s devrait être un VPN", name)
		}
	}
	for _, name := range []string{"Intel(R) Ethernet Connection I219-V", "Realtek PCIe GbE Family Controller", "eth0", "wlan0"} {
		if isVirtualAdapter(name) || isVPNAdapter(name) {
			t.Errorf("%s devrait être physique", name)
		}
	}
	if isUsableIPv4(netip.MustParseAddr("169.254.1.1")) || isUsableIPv4(netip.MustParseAddr("127.0.0.1")) || !isUsableIPv4(netip.MustParseAddr("192.168.1.2")) {
		t.Error("filtrage des adresses incorrect")
	}
}

func TestCurrentDoesNotFail(t *testing.T) {
	s, err := Current()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("réseau : %+v (principale %s)", s, s.PrimaryAddress())
}

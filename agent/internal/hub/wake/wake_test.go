package wake

import (
	"bytes"
	"context"
	"net"
	"net/netip"
	"slices"
	"testing"
	"time"
)

func TestPacket(t *testing.T) {
	p, err := Packet("aa-bb-cc-dd-ee-0f")
	if err != nil || len(p) != 102 {
		t.Fatalf("paquet : %d octets, %v", len(p), err)
	}
	if !bytes.Equal(p[:6], bytes.Repeat([]byte{0xFF}, 6)) || !bytes.Equal(p[96:], []byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0x0F}) {
		t.Fatalf("contenu : %x", p)
	}
	for _, bad := range []string{"", "FF:FF:FF:FF:FF:FF", "00:00:00:00:00:00", "aa:bb:cc"} {
		if _, err := Packet(bad); err == nil {
			t.Errorf("%q accepté", bad)
		}
	}
}

func TestTargets(t *testing.T) {
	got := Targets([]netip.Prefix{
		netip.MustParsePrefix("192.168.1.20/24"), netip.MustParsePrefix("192.168.1.21/24"),
		netip.MustParsePrefix("10.0.5.3/16"), netip.MustParsePrefix("172.17.0.1/16"),
		netip.MustParsePrefix("203.0.113.5/24"), netip.MustParsePrefix("192.168.9.9/32"), netip.MustParsePrefix("fd00::1/64"),
	}, 9)
	want := []netip.AddrPort{
		netip.MustParseAddrPort("255.255.255.255:9"), netip.MustParseAddrPort("192.168.1.255:9"),
		netip.MustParseAddrPort("10.0.255.255:9"), netip.MustParseAddrPort("172.17.255.255:9"),
	}
	if !slices.Equal(got, want) {
		t.Fatalf("destinations : %v", got)
	}
}

func TestSendTo(t *testing.T) {
	ln, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	packet, _ := Packet("AA:BB:CC:DD:EE:01")
	sent, err := SendTo(context.Background(), packet, []netip.AddrPort{ln.LocalAddr().(*net.UDPAddr).AddrPort()})
	if err != nil || sent != 3 {
		t.Fatalf("envoi : %d, %v", sent, err)
	}
	_ = ln.SetReadDeadline(time.Now().Add(2 * time.Second))
	for range 3 {
		buf := make([]byte, 200)
		n, err := ln.Read(buf)
		if err != nil || !bytes.Equal(buf[:n], packet) {
			t.Fatalf("reçu : %x, %v", buf[:n], err)
		}
	}
}

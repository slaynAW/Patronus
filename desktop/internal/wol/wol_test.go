package wol

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/desktop/internal/model"
)

func TestSharedPacketVectors(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "protocol", "test-vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var vectors struct {
		MagicPackets []struct {
			MAC       string  `json:"mac"`
			SecureOn  *string `json:"secureOn"`
			PacketHex string  `json:"packetHex"`
		} `json:"magicPackets"`
	}
	if err := json.Unmarshal(raw, &vectors); err != nil {
		t.Fatal(err)
	}
	for _, v := range vectors.MagicPackets {
		mac, _ := model.ParseMAC(v.MAC)
		var password []byte
		if v.SecureOn != nil {
			password, _ = model.SecureOnPasswordBytes(*v.SecureOn)
		}
		packet, err := BuildPacket(mac, password)
		if err != nil || hex.EncodeToString(packet) != v.PacketHex {
			t.Errorf("%s : %x (%v)", v.MAC, packet, err)
		}
	}
	if _, err := BuildPacket(model.MAC{1}, []byte{1, 2}); err == nil {
		t.Error("mot de passe SecureOn de mauvaise taille accepté")
	}
}

func TestDirectedBroadcast(t *testing.T) {
	cases := map[string]string{
		"192.168.1.37/24": "192.168.1.255",
		"10.0.12.4/22":    "10.0.15.255",
		"172.16.0.1/12":   "172.31.255.255",
		"10.1.2.3/0":      "255.255.255.255",
	}
	for in, want := range cases {
		got, ok := Directed(netip.MustParsePrefix(in))
		if !ok || got.String() != want {
			t.Errorf("%s → %v %v", in, got, ok)
		}
	}
	for _, in := range []string{"192.168.1.1/31", "192.168.1.1/32", "fe80::1/64"} {
		if _, ok := Directed(netip.MustParsePrefix(in)); ok {
			t.Errorf("%s ne devrait pas avoir de diffusion", in)
		}
	}
}

func TestTargets(t *testing.T) {
	override := netip.MustParseAddr("192.168.1.255")
	routes := []Route{{netip.MustParsePrefix("192.168.1.37/24")}, {netip.MustParsePrefix("10.0.0.5/8")}}
	var got []string
	for _, tg := range Targets(override, routes) {
		got = append(got, tg.source.String()+">"+tg.dest.String())
	}
	want := []string{"invalid IP>192.168.1.255", "192.168.1.37>192.168.1.255", "192.168.1.37>255.255.255.255",
		"10.0.0.5>10.255.255.255", "10.0.0.5>255.255.255.255"}
	if len(got) != len(want) {
		t.Fatalf("%v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%d : %s ≠ %s", i, got[i], want[i])
		}
	}
	if only := Targets(netip.Addr{}, nil); len(only) != 1 || only[0].dest != Limited {
		t.Errorf("sans carte réseau : %v", only)
	}
}

func TestSendOnLoopback(t *testing.T) {
	receiver, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer receiver.Close()
	port := receiver.LocalAddr().(*net.UDPAddr).Port
	mac, _ := model.ParseMAC("AA:BB:CC:DD:EE:01")
	packet, _ := BuildPacket(mac, nil)
	result := Send(context.Background(), Request{
		Packet:   packet,
		Override: netip.MustParseAddr("127.0.0.1"),
		Port:     port,
		Routes:   []Route{{netip.MustParsePrefix("127.0.0.1/32")}},
		Repeat:   2,
		Interval: 10 * time.Millisecond,
	})
	if !result.Success() {
		t.Fatalf("aucun paquet envoyé : %+v", result)
	}
	_ = receiver.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 200)
	for range 2 {
		n, err := receiver.Read(buf)
		if err != nil || !bytes.Equal(buf[:n], packet) {
			t.Fatalf("paquet reçu incorrect : %v", err)
		}
	}
}

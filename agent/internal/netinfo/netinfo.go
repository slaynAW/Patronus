// Package netinfo détecte l'adresse IP et l'adresse MAC du PC, pour pré-remplir l'appairage.
package netinfo

import (
	"errors"
	"fmt"
	"net"
)

// Interface décrit la carte réseau retenue.
type Interface struct {
	Name string
	IP   net.IP
	MAC  net.HardwareAddr
}

// Detect trouve la carte réseau principale. Si preferredIP est fourni, c'est la carte qui porte
// cette adresse qui est retenue (utile si le PC a plusieurs cartes : Wi-Fi + Ethernet, VPN...).
func Detect(preferredIP string) (*Interface, error) {
	var want net.IP
	if preferredIP != "" {
		want = net.ParseIP(preferredIP)
		if want == nil {
			return nil, fmt.Errorf("adresse IP invalide : %q", preferredIP)
		}
	} else {
		want = outboundIP()
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var fallback *Interface
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 || len(iface.HardwareAddr) != 6 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP.To4() == nil {
				continue
			}
			candidate := &Interface{Name: iface.Name, IP: ipNet.IP.To4(), MAC: iface.HardwareAddr}
			if want != nil && candidate.IP.Equal(want) {
				return candidate, nil
			}
			if fallback == nil && ipNet.IP.IsPrivate() {
				fallback = candidate
			}
		}
	}
	if preferredIP != "" {
		return nil, fmt.Errorf("aucune carte réseau ne porte l'adresse %s", preferredIP)
	}
	if fallback != nil {
		return fallback, nil
	}
	return nil, errors.New("aucune carte réseau avec une adresse IPv4 privée n'a été trouvée")
}

// outboundIP renvoie l'adresse utilisée pour sortir vers le réseau (aucun paquet n'est envoyé :
// « connecter » une socket UDP ne fait que choisir la route).
func outboundIP() net.IP {
	conn, err := net.Dial("udp4", "192.0.2.1:9")
	if err != nil {
		return nil
	}
	defer conn.Close()
	if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
		return addr.IP.To4()
	}
	return nil
}

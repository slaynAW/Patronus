package netprofile

import "testing"

func TestRelevantAndFirstPublic(t *testing.T) {
	profiles := []Profile{
		{Name: "VPN", Interface: "Tailscale", Index: 20, Category: Public},
		{Name: "Maison", Interface: "Ethernet", Index: 7, Category: Private},
	}
	lan := Relevant(profiles, "ethernet")
	if len(lan) != 1 || lan[0].Index != 7 {
		t.Fatalf("réseau de la carte locale : %+v", lan)
	}
	if _, ok := FirstPublic(lan); ok {
		t.Error("réseau Privé signalé Public")
	}
	if all := Relevant(profiles, "Wi-Fi"); len(all) != 2 {
		t.Errorf("carte inconnue : tous les réseaux attendus, %+v", all)
	}
	if p, ok := FirstPublic(profiles); !ok || p.Name != "VPN" {
		t.Errorf("premier réseau Public : %+v %v", p, ok)
	}
	if Private.String() != "Privé" || Category(9).String() != "inconnu (9)" {
		t.Error("libellés des types de réseau")
	}
}

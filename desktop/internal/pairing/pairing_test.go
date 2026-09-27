package pairing

import (
	"net"
	"strings"
	"testing"

	"github.com/slaynaw/wakeonlan/agent/protocol"
)

func TestParseAgentLink(t *testing.T) {
	key, _ := protocol.NewKey()
	mac, _ := net.ParseMAC("aa:bb:cc:dd:ee:ff")
	// Lien produit par l'agent Go (url.QueryEscape encode les espaces en « + »).
	link := "wolagent://pair?v=1&n=PC+de+Salon+%26+Jeux&h=192.168.1.42&p=9771&m=" + strings.ReplaceAll(mac.String(), ":", "%3A") + "&k=" + key
	info, err := Parse("  " + link + "\n")
	if err != nil {
		t.Fatal(err)
	}
	want := Info{Name: "PC de Salon & Jeux", Host: "192.168.1.42", Port: 9771, MAC: "AA:BB:CC:DD:EE:FF", Key: key}
	if info != want {
		t.Errorf("%+v", info)
	}
	if strings.Contains(info.String(), key) {
		t.Error("clé visible")
	}
	noName, err := Parse("WOLAGENT://PAIR?v=1&h=pc.local&k=" + key)
	if err != nil || noName.Name != "pc.local" || noName.Port != 9770 || noName.MAC != "" {
		t.Errorf("valeurs par défaut : %+v %v", noName, err)
	}
	bad := map[string]string{
		"https://example.com":                            "Ce n'est pas un lien d'appairage wolagent://",
		strings.Replace(link, key, "abc", 1):             "Clé invalide dans le lien",
		strings.Replace(link, "v=1", "v=2", 1):           "Version de lien non prise en charge : 2",
		strings.Replace(link, "h=192.168.1.42", "h=", 1): "Adresse du PC invalide dans le lien",
		strings.Replace(link, "p=9771", "p=0", 1):        "Port invalide dans le lien",
		strings.Replace(link, "m=", "m=zz", 1):           "Adresse MAC invalide dans le lien",
		"wolagent://pair?v=1&=x":                         "Lien d'appairage mal formé",
		"wolagent://pair?h=pc":                           "Version de lien non prise en charge : null",
	}
	for in, msg := range bad {
		if _, err := Parse(in); err == nil || err.Error() != msg {
			t.Errorf("%s : %v (attendu %q)", in, err, msg)
		}
	}
}

package pairing

import (
	"net"
	"strings"
	"testing"

	"github.com/slaynaw/wakeonlan/agent/protocol"
	"github.com/slaynaw/wakeonlan/agent/protocol2"
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
	want := Info{Version: 1, Name: "PC de Salon & Jeux", Host: "192.168.1.42", Port: 9771, MAC: "AA:BB:CC:DD:EE:FF", Key: key}
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
		strings.Replace(link, "v=1", "v=2", 1):           "Empreinte de l'agent invalide dans le lien",
		strings.Replace(link, "v=1", "v=3", 1):           "Version de lien non prise en charge : 3",
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

// TestParseV2Link : lien affiché par « wol-agent pair » (agent 1.11) : empreinte de l'agent et code à
// usage unique, aucune clé.
func TestParseV2Link(t *testing.T) {
	agent := protocol2.PinOf([]byte("agent de test"))
	l := protocol2.Link{Name: "PC Bureau", Host: "192.168.1.20", Port: 9771, MAC: "30:9C:23:01:02:03", Agent: agent,
		Code: "7KQ29MXP", Role: protocol2.RoleAdmin, Expires: 1791880773}
	info, err := Parse(" " + l.String() + "\n")
	if err != nil {
		t.Fatal(err)
	}
	want := Info{Version: 2, Name: "PC Bureau", Host: "192.168.1.20", Port: 9771, MAC: "30:9C:23:01:02:03",
		Agent: agent.String(), AgentShort: agent.Short(), Code: "7KQ29MXP", Role: "admin", Expires: 1791880773}
	if info != want {
		t.Errorf("%+v", info)
	}
	if strings.Contains(info.String(), "7KQ29MXP") {
		t.Error("code visible dans le journal")
	}
	// Valeurs par défaut : port 9770, nom tiré de l'hôte, code avec tiret et minuscules accepté.
	minimal, err := Parse("WOLAGENT://PAIR?v=2&h=pc.local&a=" + agent.String() + "&c=7kq2-9mxp")
	if err != nil || minimal.Name != "pc.local" || minimal.Port != 9770 || minimal.MAC != "" || minimal.Code != "7KQ29MXP" ||
		minimal.Role != "" || minimal.Expires != 0 {
		t.Errorf("valeurs par défaut : %+v %v", minimal, err)
	}
	link := l.String()
	bad := map[string]string{
		strings.Replace(link, "a="+agent.String(), "a=abc", 1):  "Empreinte de l'agent invalide dans le lien",
		strings.Replace(link, "c=7KQ29MXP", "c=7KQ29MX", 1):     "Code d'appairage invalide dans le lien",
		strings.Replace(link, "c=7KQ29MXP", "c=7KQ29MXU", 1):    "Code d'appairage invalide dans le lien",
		strings.Replace(link, "h=192.168.1.20", "h=", 1):        "Adresse du PC invalide dans le lien",
		strings.Replace(link, "p=9771", "p=70000", 1):           "Port invalide dans le lien",
		strings.Replace(link, "m=30:9C:23:01:02:03", "m=zz", 1): "Adresse MAC invalide dans le lien",
		strings.Replace(link, "r=admin", "r=owner", 1):          "Rôle invalide dans le lien",
		strings.Replace(link, "e=1791880773", "e=0", 1):         "Date d'expiration invalide dans le lien",
		link + "&c=7KQ29MXP":                                    "Lien d'appairage mal formé",
		link + "&k=" + strings.Repeat("A", 43):                  "",
	}
	for in, msg := range bad {
		_, err := Parse(in)
		if msg == "" {
			if err != nil {
				t.Errorf("%s : paramètre inconnu refusé (%v)", in, err)
			}
			continue
		}
		if err == nil || err.Error() != msg {
			t.Errorf("%s : %v (attendu %q)", in, err, msg)
		}
	}
}

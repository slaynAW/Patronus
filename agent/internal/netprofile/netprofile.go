// Package netprofile lit le type des réseaux Windows (Public, Privé, Domaine) et l'état du pare-feu.
//
// Sur un réseau classé « Public », la règle de pare-feu de l'agent (réseaux Privé et Domaine) ne
// s'applique pas : Windows bloque alors l'agent sans rien afficher, et les applications voient le PC
// éteint alors qu'il est allumé. C'est le réglage de Windows pour tout réseau qu'il découvre, sauf
// si l'utilisateur le déclare de confiance.
package netprofile

import (
	"errors"
	"fmt"
	"strings"
)

// Category est le type d'un réseau Windows.
type Category int

const (
	Public  Category = 0
	Private Category = 1
	Domain  Category = 2
)

func (c Category) String() string {
	switch c {
	case Public:
		return "Public"
	case Private:
		return "Privé"
	case Domain:
		return "Domaine"
	}
	return fmt.Sprintf("inconnu (%d)", int(c))
}

// Profile est un réseau connecté.
type Profile struct {
	// Name est le nom du réseau (« Livebox-1A2B », « Réseau 2 »).
	Name string
	// Interface et Index désignent la carte réseau (« Ethernet », numéro Windows).
	Interface string
	Index     int
	Category  Category
}

// Firewall décrit ce que le pare-feu de Windows laisse passer vers l'agent sur les réseaux Publics.
type Firewall struct {
	// PublicEnabled : pare-feu actif pour les réseaux Publics.
	PublicEnabled bool
	// RuleFound : règle de l'agent présente et active ; RuleAllowsPublic : elle couvre aussi les
	// réseaux Publics (installation avec --firewall-public).
	RuleFound        bool
	RuleAllowsPublic bool
}

// BlocksPublic indique que l'agent est injoignable depuis un réseau Public.
func (f Firewall) BlocksPublic() bool {
	return f.PublicEnabled && !(f.RuleFound && f.RuleAllowsPublic)
}

// ErrUnsupported : lecture des réseaux prise en charge sous Windows uniquement.
var ErrUnsupported = errors.New("types de réseau Windows : non pris en charge sur ce système")

// Relevant garde les réseaux de la carte iface (celle du réseau local), ou tous si aucun ne correspond.
func Relevant(profiles []Profile, iface string) []Profile {
	var out []Profile
	for _, p := range profiles {
		if iface != "" && strings.EqualFold(p.Interface, iface) {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return profiles
	}
	return out
}

// FirstPublic renvoie le premier réseau Public de la liste.
func FirstPublic(profiles []Profile) (Profile, bool) {
	for _, p := range profiles {
		if p.Category == Public {
			return p, true
		}
	}
	return Profile{}, false
}

// Advice explique comment classer un réseau en Privé à la main.
const Advice = "Paramètres Windows → Réseau et Internet → Ethernet (ou Wi-Fi) → ce réseau → " +
	"Type de profil réseau : Privé."

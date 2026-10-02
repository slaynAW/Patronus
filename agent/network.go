package main

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"runtime"
	"time"

	"github.com/slaynaw/wakeonlan/agent/internal/netalert"
	"github.com/slaynaw/wakeonlan/agent/internal/netinfo"
	"github.com/slaynaw/wakeonlan/agent/internal/netprofile"
	"github.com/slaynaw/wakeonlan/agent/internal/service"
	"github.com/slaynaw/wakeonlan/agent/internal/session"
	"github.com/slaynaw/wakeonlan/agent/internal/terminal"
)

// Réseau « Public » (Windows) : le pare-feu bloque l'agent, les applications voient le PC éteint.

// lanInterface renvoie le nom de la carte du réseau local ("" si inconnue).
func lanInterface() string {
	if iface, err := netinfo.Detect(""); err == nil {
		return iface.Name
	}
	return ""
}

// blockedNetwork renvoie le réseau Public sur lequel le pare-feu bloque l'agent, s'il y en a un.
func blockedNetwork() (netprofile.Profile, bool) {
	if runtime.GOOS != "windows" {
		return netprofile.Profile{}, false
	}
	profiles, err := netprofile.Profiles()
	if err != nil {
		return netprofile.Profile{}, false
	}
	public, ok := netprofile.FirstPublic(netprofile.Relevant(profiles, lanInterface()))
	if !ok {
		return netprofile.Profile{}, false
	}
	fw, err := netprofile.ReadFirewall(service.FirewallRule)
	if err != nil || !fw.BlocksPublic() {
		return netprofile.Profile{}, false
	}
	return public, true
}

// describeNetworks résume les réseaux Windows et le pare-feu (status, rapport de diagnostic).
func describeNetworks() string {
	profiles, err := netprofile.Profiles()
	if err != nil {
		return "illisibles : " + err.Error()
	}
	if len(profiles) == 0 {
		return "aucun réseau connecté"
	}
	var out string
	for i, p := range profiles {
		if i > 0 {
			out += " ; "
		}
		out += fmt.Sprintf("« %s » (%s) : %s", p.Name, p.Interface, p.Category)
	}
	if fw, err := netprofile.ReadFirewall(service.FirewallRule); err == nil {
		out += fmt.Sprintf(" — pare-feu sur les réseaux Publics : %s, règle de l'agent : %s",
			pick(fw.PublicEnabled, "actif", "inactif"),
			pick(!fw.RuleFound, "absente", pick(fw.RuleAllowsPublic, "tous les réseaux", "réseaux Privés et Domaine")))
	}
	return out
}

// warnBlockedNetwork signale un réseau Public dans le terminal ; fix : propose de le classer en Privé.
func warnBlockedNetwork(fix bool) {
	p, blocked := blockedNetwork()
	if !blocked {
		return
	}
	fmt.Printf("\n⚠ Le réseau « %s » (carte %s) est classé « Public » par Windows : le pare-feu bloque\n", p.Name, p.Interface)
	fmt.Println("  l'agent, et les applications verront ce PC éteint.")
	if !fix || !terminal.IsAdmin() {
		fmt.Println("  S'il s'agit de votre réseau domestique : " + netprofile.Advice)
		return
	}
	if !terminal.Confirm("  S'il s'agit de votre réseau domestique (box, réseau de confiance), le classer en Privé maintenant ?") {
		fmt.Println("  Réseau laissé en Public. Pour le faire plus tard : " + netprofile.Advice)
		return
	}
	if err := netprofile.SetPrivate(p.Index); err != nil {
		fmt.Printf("  Impossible (%v). À faire à la main : %s\n", err, netprofile.Advice)
		return
	}
	fmt.Printf("  ✔ Réseau « %s » classé en Privé : les applications peuvent joindre l'agent.\n", p.Name)
}

// startNetworkAlert lance, dans le service Windows, la surveillance du type de réseau.
func startNetworkAlert(ctx context.Context, cfgPath string, logger *log.Logger) {
	if runtime.GOOS != "windows" {
		return
	}
	checker := &netalert.Checker{
		Interface: lanInterface,
		Profiles:  netprofile.Profiles,
		Firewall:  func() (netprofile.Firewall, error) { return netprofile.ReadFirewall(service.FirewallRule) },
		Ask: func(message string) (session.Answer, error) {
			return session.Ask(message, netalert.AskTimeout, true)
		},
		Notify:     session.Notify,
		SetPrivate: netprofile.SetPrivate,
		StatePath:  filepath.Join(filepath.Dir(cfgPath), "network-alert.json"),
		Logf:       logger.Printf,
		Now:        time.Now,
	}
	go checker.Run(ctx)
}

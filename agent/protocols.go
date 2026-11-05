package main

import (
	"errors"
	"flag"
	"fmt"

	"github.com/slaynaw/wakeonlan/agent/internal/config"
	"github.com/slaynaw/wakeonlan/agent/internal/service"
	"github.com/slaynaw/wakeonlan/agent/internal/terminal"
)

// restartService applique la configuration enregistrée ; remplaçable dans les tests.
var restartService = service.Restart

// v1Help explique comment rétablir wolagent/1 pour un appareil qui ne connaît que lui.
const v1Help = "Téléphone sous Android 8 ou 9, ou application antérieure à 1.12 : « wol-agent protocols v1 on », " +
	"puis « wol-agent pair --v1 »."

// cmdProtocols affiche les protocoles acceptés par l'agent ; « v1 on » rétablit l'ancien protocole
// wolagent/1 (clé partagée), « v1 off » le refuse et efface sa clé (docs/PROTOCOLE-V2.md §11). Depuis
// l'agent 1.14.0, une nouvelle installation n'accepte que wolagent/2.
func cmdProtocols(args []string) error {
	fs := flag.NewFlagSet("protocols", flag.ExitOnError)
	cfgPath := fs.String("config", config.DefaultPath(), "fichier de configuration")
	_ = fs.Parse(args)
	rest := fs.Args()
	if len(rest) != 0 && (len(rest) != 2 || rest[0] != "v1" || (rest[1] != "on" && rest[1] != "off")) {
		return errors.New("utilisation : wol-agent protocols [v1 on|off]")
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		if !terminal.IsAdmin() {
			return fmt.Errorf("%w\n(la configuration n'est lisible qu'en administrateur)", err)
		}
		return err
	}
	if len(rest) == 0 {
		fmt.Println("Protocoles acceptés :", describeV2(cfg, *cfgPath))
		if !cfg.Accepts(config.ProtocolV1) {
			fmt.Println(v1Help)
		}
		return nil
	}

	on := rest[1] == "on"
	switch {
	case on && cfg.Accepts(config.ProtocolV1):
		fmt.Println("wolagent/1 est déjà accepté : « wol-agent pair --v1 » affiche l'ancien QR code.")
		return nil
	case !on && !cfg.Accepts(config.ProtocolV1) && cfg.Key == "":
		fmt.Println("wolagent/1 est déjà refusé.")
		return nil
	case on:
		if err := cfg.EnableV1(); err != nil {
			return err
		}
	default:
		cfg.DisableV1()
	}
	if err := config.Save(*cfgPath, cfg); err != nil {
		return fmt.Errorf("enregistrement de la configuration : %w", err)
	}
	if on {
		fmt.Println("✔ wolagent/1 accepté de nouveau, avec sa clé : « wol-agent pair --v1 » affiche l'ancien QR code.")
		fmt.Println("  Ce QR code contient la clé de l'agent : ne le partagez pas. Les appareils en wolagent/2 ne")
		fmt.Println("  sont pas concernés ; « wol-agent protocols v1 off » refuse de nouveau wolagent/1.")
	} else {
		fmt.Println("✔ wolagent/1 refusé, et sa clé effacée : seuls les appareils en wolagent/2 se connectent.")
		fmt.Println("  Un appareil encore en wolagent/1 doit être appairé de nouveau (« wol-agent pair »).")
	}
	if err := restartService(); err != nil {
		fmt.Println("Attention : redémarrez le service pour appliquer ce changement :", err)
	}
	return nil
}

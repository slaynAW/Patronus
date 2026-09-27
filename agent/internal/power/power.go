// Package power exécute les actions d'alimentation (extinction, redémarrage, veille) selon le système.
package power

import (
	"fmt"
	"log"
)

// Action est une action d'alimentation.
type Action string

const (
	Shutdown Action = "shutdown"
	Reboot   Action = "reboot"
	Sleep    Action = "sleep"
)

// Parse convertit une commande du protocole en action.
func Parse(cmd string) (Action, bool) {
	switch Action(cmd) {
	case Shutdown, Reboot, Sleep:
		return Action(cmd), true
	}
	return "", false
}

// Label renvoie un libellé lisible.
func (a Action) Label() string {
	switch a {
	case Shutdown:
		return "Extinction"
	case Reboot:
		return "Redémarrage"
	case Sleep:
		return "Mise en veille"
	}
	return string(a)
}

// Controller exécute une action. force = fermer les applications sans attendre.
type Controller interface {
	Do(action Action, force bool) error
}

// System renvoie le contrôleur du système courant.
func System() Controller { return systemController{} }

// DryRun ne fait que journaliser : pratique pour tester l'installation sans éteindre le PC.
type DryRun struct{ Logger *log.Logger }

func (d DryRun) Do(action Action, force bool) error {
	d.Logger.Printf("[simulation] %s demandée (forcer=%v) : aucune action réelle", action.Label(), force)
	return nil
}

func unsupported(action Action) error {
	return fmt.Errorf("%s non prise en charge sur ce système", action.Label())
}

//go:build !windows

package terminal

import (
	"errors"
	"os"
)

// EnableANSI indique si la sortie est un terminal capable d'afficher des couleurs.
func EnableANSI() bool {
	info, err := os.Stdout.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	term := os.Getenv("TERM")
	return term != "" && term != "dumb"
}

// LaunchedFromExplorer n'a de sens que sous Windows.
func LaunchedFromExplorer() bool { return false }

// IsAdmin est vrai si le processus tourne en root.
func IsAdmin() bool { return os.Geteuid() == 0 }

// RelaunchElevated n'est pas proposé hors Windows : utilisez sudo.
func RelaunchElevated([]string) error {
	return errors.New("relancez la commande avec sudo")
}

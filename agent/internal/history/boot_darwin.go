package history

import (
	"time"

	"golang.org/x/sys/unix"
)

// BootID renvoie l'identifiant unique du démarrage en cours.
func BootID() string {
	id, err := unix.Sysctl("kern.bootsessionuuid")
	if err != nil {
		return ""
	}
	return id
}

// SuspendedTotal n'est pas disponible simplement sous macOS : la veille est déduite d'un saut de
// l'horloge murale (l'horloge monotone s'arrête pendant la veille).
func SuspendedTotal() (time.Duration, bool) { return 0, false }

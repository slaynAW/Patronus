package history

import (
	"os"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// BootID renvoie l'identifiant unique du démarrage en cours.
func BootID() string {
	raw, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

// SuspendedTotal renvoie le temps passé en veille depuis le démarrage (CLOCK_BOOTTIME − CLOCK_MONOTONIC).
func SuspendedTotal() (time.Duration, bool) {
	// Horloge monotone lue en premier : l'écart ne peut pas être négatif.
	var boot, mono unix.Timespec
	if unix.ClockGettime(unix.CLOCK_MONOTONIC, &mono) != nil || unix.ClockGettime(unix.CLOCK_BOOTTIME, &boot) != nil {
		return 0, false
	}
	return max(0, time.Duration(boot.Nano()-mono.Nano())), true
}

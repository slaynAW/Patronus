//go:build !linux && !windows && !darwin

package history

import "time"

// BootID n'est pas disponible sur ce système.
func BootID() string { return "" }

// SuspendedTotal n'est pas disponible sur ce système.
func SuspendedTotal() (time.Duration, bool) { return 0, false }

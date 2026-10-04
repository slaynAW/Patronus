//go:build !windows

package history

import "time"

// SystemCause : cause inconnue hors de Windows (pas de journal d'événements lu).
func SystemCause(_, _ time.Time) Cause { return Cause{} }

//go:build !windows

package status

import (
	"context"
	"net/netip"
	"time"
)

// Ping n'est disponible que sous Windows (hors Windows : développement, les sondes TCP suffisent).
func Ping(context.Context, netip.Addr, time.Duration) bool { return false }

// Package netdial ouvre les connexions TCP de l'application (agent et sondes d'état) et reconnaît
// une connexion refusée, qui prouve que la machine est allumée.
package netdial

import (
	"context"
	"errors"
	"net"
	"syscall"
	"time"
)

// Dial ouvre une connexion TCP vers address avec un délai maximal.
func Dial(ctx context.Context, address string, timeout time.Duration) (net.Conn, error) {
	d := net.Dialer{Timeout: timeout, Control: control}
	return d.DialContext(ctx, "tcp", address)
}

// IsRefused indique que la machine a répondu « port fermé » (RST) : elle est donc allumée.
func IsRefused(err error) bool {
	if err == nil {
		return false
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return isRefusedErrno(errno)
	}
	return false
}

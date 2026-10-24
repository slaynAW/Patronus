//go:build !windows

package netinfo

import (
	"errors"
	"syscall"
)

// ConnRefused indique une connexion refusée : rien n'écoute (service arrêté), mais la machine a répondu.
func ConnRefused(err error) bool { return errors.Is(err, syscall.ECONNREFUSED) }

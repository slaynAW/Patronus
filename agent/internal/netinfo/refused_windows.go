package netinfo

import (
	"errors"
	"syscall"

	"golang.org/x/sys/windows"
)

// ConnRefused indique une connexion refusée : rien n'écoute (service arrêté), mais la machine a répondu.
func ConnRefused(err error) bool {
	return errors.Is(err, windows.WSAECONNREFUSED) || errors.Is(err, syscall.ECONNREFUSED)
}

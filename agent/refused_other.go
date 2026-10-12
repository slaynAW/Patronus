//go:build !windows

package main

import (
	"errors"
	"syscall"
)

// connRefused indique une connexion refusée : rien n'écoute (service arrêté).
func connRefused(err error) bool { return errors.Is(err, syscall.ECONNREFUSED) }

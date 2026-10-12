package main

import (
	"errors"
	"syscall"

	"golang.org/x/sys/windows"
)

// connRefused indique une connexion refusée : rien n'écoute (service arrêté).
func connRefused(err error) bool {
	return errors.Is(err, windows.WSAECONNREFUSED) || errors.Is(err, syscall.ECONNREFUSED)
}

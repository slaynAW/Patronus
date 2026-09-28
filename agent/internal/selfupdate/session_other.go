//go:build !windows

package selfupdate

import (
	"context"
	"errors"
)

// Supported indique que l'installation des mises à jour est prise en charge sur ce système.
const Supported = false

var errUnsupported = errors.New("mise à jour automatique disponible sous Windows uniquement")

// Ask : pas de fenêtre hors Windows.
func Ask(context.Context, string, Offer) (Answer, error) { return NoUser, errUnsupported }

// Notify : sans effet hors Windows.
func Notify(string, bool) {}

// StartFinisher : non pris en charge hors Windows.
func StartFinisher(string) error { return errUnsupported }

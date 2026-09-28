//go:build !windows && !linux && !darwin

package service

import (
	"context"
	"errors"
)

var errUnsupported = errors.New("installation automatique non prise en charge : lancez « wol-agent run »")

func DefaultBinary() string                             { return "/usr/local/bin/wol-agent" }
func Install(Options) error                             { return errUnsupported }
func Uninstall() error                                  { return errUnsupported }
func Restart() error                                    { return errUnsupported }
func Status() string                                    { return "non pris en charge" }
func IsRunning() bool                                   { return false }
func Run(func(ctx context.Context) error) (bool, error) { return false, nil }

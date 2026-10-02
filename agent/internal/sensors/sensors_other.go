//go:build !windows && !linux

package sensors

import "github.com/slaynaw/wakeonlan/agent/protocol"

// Pas encore de lecture des capteurs sur ce système (macOS…).
func read() protocol.Temperatures { return protocol.Temperatures{} }

func details() []string { return nil }

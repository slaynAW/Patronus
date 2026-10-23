//go:build !linux && !windows

package measures

import "github.com/slaynaw/wakeonlan/agent/protocol"

// memory : pas encore lue sur ce système.
func memory() *protocol.Measure { return nil }

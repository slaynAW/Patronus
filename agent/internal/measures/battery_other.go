//go:build !linux && !windows

package measures

import "github.com/slaynaw/wakeonlan/agent/protocol"

// battery : pas encore lue sur ce système.
func battery() *protocol.Measure { return nil }

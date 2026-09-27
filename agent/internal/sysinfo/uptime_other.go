//go:build !linux && !windows && !darwin

package sysinfo

import "time"

func uptime() time.Duration { return 0 }

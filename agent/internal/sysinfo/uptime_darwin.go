package sysinfo

import (
	"time"

	"golang.org/x/sys/unix"
)

func uptime() time.Duration {
	tv, err := unix.SysctlTimeval("kern.boottime")
	if err != nil {
		return 0
	}
	return time.Since(time.Unix(tv.Unix()))
}

package sysinfo

import (
	"time"

	"golang.org/x/sys/windows"
)

var getTickCount64 = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetTickCount64")

func uptime() time.Duration {
	ms, _, _ := getTickCount64.Call()
	return time.Duration(ms) * time.Millisecond
}

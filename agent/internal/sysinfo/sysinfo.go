// Package sysinfo fournit les informations renvoyées par la commande « status ».
package sysinfo

import (
	"os"
	"runtime"
	"time"
)

// Info décrit la machine.
type Info struct {
	Hostname string
	OS       string
	Arch     string
	Uptime   time.Duration
}

// Current renvoie les informations actuelles de la machine.
func Current() Info {
	host, _ := os.Hostname()
	return Info{Hostname: host, OS: runtime.GOOS, Arch: runtime.GOARCH, Uptime: uptime()}
}

//go:build !windows

package netdial

import "syscall"

func control(_, _ string, _ syscall.RawConn) error { return nil }

func isRefusedErrno(errno syscall.Errno) bool { return errno == syscall.ECONNREFUSED }

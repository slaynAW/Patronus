//go:build !windows

package session

import "time"

func ask(string, time.Duration, bool) (Answer, error) { return NoUser, nil }

func notify(string, bool) {}

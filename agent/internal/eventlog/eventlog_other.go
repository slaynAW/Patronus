//go:build !windows

package eventlog

// Query : pas de journal d'événements Windows sur ce système.
func Query(channel, query string, max int) ([]Event, error) { return nil, ErrUnsupported }

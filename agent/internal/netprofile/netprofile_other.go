//go:build !windows

package netprofile

// Profiles : sans objet hors Windows.
func Profiles() ([]Profile, error) { return nil, nil }

// ReadFirewall : sans objet hors Windows (aucun blocage signalé).
func ReadFirewall(string) (Firewall, error) { return Firewall{}, nil }

// SetPrivate : non pris en charge hors Windows.
func SetPrivate(int) error { return ErrUnsupported }

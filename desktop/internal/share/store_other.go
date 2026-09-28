//go:build !windows

package share

// Hors Windows (développement et tests uniquement), le fichier est en clair (dossier en 0700).
const fileName = "share.json"

func protect(plain []byte) ([]byte, error) { return append([]byte(nil), plain...), nil }

func unprotect(sealed []byte) ([]byte, error) { return append([]byte(nil), sealed...), nil }

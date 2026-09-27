//go:build !windows

package config

// Hors Windows (développement et tests uniquement), le fichier est en clair, lisible par le seul
// propriétaire (dossier créé en 0700).
const fileName = "config.json"

func protect(plain []byte) ([]byte, error) { return append([]byte(nil), plain...), nil }

func unprotect(sealed []byte) ([]byte, error) { return append([]byte(nil), sealed...), nil }

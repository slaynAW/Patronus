//go:build !windows

// Package dpapi : hors Windows (développement et tests uniquement), les données restent en clair ;
// les fichiers sont dans un dossier lisible par le seul propriétaire (0700).
package dpapi

// Protect renvoie une copie de data.
func Protect(data, _ []byte) ([]byte, error) { return append([]byte(nil), data...), nil }

// Unprotect renvoie une copie de data.
func Unprotect(data, _ []byte) ([]byte, error) { return append([]byte(nil), data...), nil }

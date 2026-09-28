package config

import (
	"errors"

	"github.com/slaynaw/wakeonlan/desktop/internal/dpapi"
)

const fileName = "config.dat"

var entropy = []byte("wakeonlan-desktop-config/1")

func protect(plain []byte) ([]byte, error) { return dpapi.Protect(plain, entropy) }

func unprotect(sealed []byte) ([]byte, error) {
	out, err := dpapi.Unprotect(sealed, entropy)
	if err != nil {
		return nil, errors.New("déchiffrement impossible (fichier d'un autre compte Windows ?)")
	}
	return out, nil
}

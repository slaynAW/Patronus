//go:build !windows

package identity

import (
	"encoding/pem"
	"errors"
)

// seal écrit la clé en PEM : le fichier, en 0600 dans un dossier en 0700, n'est lisible que par root.
func seal(der []byte) ([]byte, error) {
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

func unseal(data []byte) ([]byte, error) {
	block, rest := pem.Decode(data)
	if block == nil || block.Type != "PRIVATE KEY" || len(rest) != 0 {
		return nil, errors.New("format inattendu")
	}
	return block.Bytes, nil
}

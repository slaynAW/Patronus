// Package dpapi chiffre les fichiers de l'application pour l'utilisateur Windows courant (DPAPI) :
// illisibles depuis un autre compte ou un autre PC, même en copiant le fichier.
package dpapi

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Protect chiffre data. entropy (propre à chaque fichier) empêche un autre programme de l'utilisateur
// de le déchiffrer par un simple appel à CryptUnprotectData.
func Protect(data, entropy []byte) ([]byte, error) { return call(data, entropy, true) }

// Unprotect déchiffre data.
func Unprotect(data, entropy []byte) ([]byte, error) { return call(data, entropy, false) }

func call(data, entropy []byte, encrypt bool) ([]byte, error) {
	if len(data) == 0 || len(entropy) == 0 {
		return nil, errors.New("données vides")
	}
	in := windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
	ent := windows.DataBlob{Size: uint32(len(entropy)), Data: &entropy[0]}
	var out windows.DataBlob
	var err error
	if encrypt {
		err = windows.CryptProtectData(&in, nil, &ent, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	} else {
		err = windows.CryptUnprotectData(&in, nil, &ent, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	}
	if err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	result := make([]byte, out.Size)
	copy(result, unsafe.Slice(out.Data, out.Size))
	return result, nil
}

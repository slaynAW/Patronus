package config

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

const fileName = "config.dat"

// Entropie supplémentaire : un autre programme de l'utilisateur utilisant DPAPI sans la connaître
// ne peut pas déchiffrer le fichier par simple appel à CryptUnprotectData.
var entropy = []byte("wakeonlan-desktop-config/1")

func protect(plain []byte) ([]byte, error) {
	return dpapi(plain, true)
}

func unprotect(sealed []byte) ([]byte, error) {
	out, err := dpapi(sealed, false)
	if err != nil {
		return nil, errors.New("déchiffrement impossible (fichier d'un autre compte Windows ?)")
	}
	return out, nil
}

func dpapi(data []byte, encrypt bool) ([]byte, error) {
	if len(data) == 0 {
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

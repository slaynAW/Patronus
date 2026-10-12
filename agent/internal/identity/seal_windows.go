package identity

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

// entropy distingue la clé de l'agent des autres secrets DPAPI de la machine.
var entropy = []byte("Patronus wol-agent identity")

// seal chiffre la clé par DPAPI (machine) : en plus des droits du dossier (SYSTEM et Administrateurs),
// une copie du fichier est illisible sur un autre PC.
func seal(der []byte) ([]byte, error) { return dpapi(der, true) }

func unseal(data []byte) ([]byte, error) { return dpapi(data, false) }

func dpapi(data []byte, encrypt bool) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("données vides")
	}
	in := windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
	ent := windows.DataBlob{Size: uint32(len(entropy)), Data: &entropy[0]}
	flags := uint32(windows.CRYPTPROTECT_UI_FORBIDDEN | windows.CRYPTPROTECT_LOCAL_MACHINE)
	var out windows.DataBlob
	var err error
	if encrypt {
		err = windows.CryptProtectData(&in, nil, &ent, 0, nil, flags, &out)
	} else {
		err = windows.CryptUnprotectData(&in, nil, &ent, 0, nil, flags, &out)
	}
	if err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}

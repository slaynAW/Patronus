package pcspecs

import "github.com/slaynaw/wakeonlan/desktop/internal/dpapi"

const fileName = "specs.dat"

var entropy = []byte("wakeonlan-desktop-specs/1")

func protect(plain []byte) ([]byte, error) { return dpapi.Protect(plain, entropy) }

func unprotect(sealed []byte) ([]byte, error) { return dpapi.Unprotect(sealed, entropy) }

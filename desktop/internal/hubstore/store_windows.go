package hubstore

import "github.com/slaynaw/wakeonlan/desktop/internal/dpapi"

const fileName = "hub.dat"

var entropy = []byte("patronus-desktop-hub/1")

func protect(plain []byte) ([]byte, error) { return dpapi.Protect(plain, entropy) }

func unprotect(sealed []byte) ([]byte, error) { return dpapi.Unprotect(sealed, entropy) }

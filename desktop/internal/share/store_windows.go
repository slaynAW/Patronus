package share

import "github.com/slaynaw/wakeonlan/desktop/internal/dpapi"

const fileName = "share.dat"

var entropy = []byte("wakeonlan-desktop-share/1")

func protect(plain []byte) ([]byte, error) { return dpapi.Protect(plain, entropy) }

func unprotect(sealed []byte) ([]byte, error) { return dpapi.Unprotect(sealed, entropy) }

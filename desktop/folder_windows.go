package main

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/slaynaw/wakeonlan/desktop/internal/app"
)

var (
	procSHBrowseForFolderW   = windows.NewLazySystemDLL("shell32.dll").NewProc("SHBrowseForFolderW")
	procSHGetPathFromIDListW = windows.NewLazySystemDLL("shell32.dll").NewProc("SHGetPathFromIDListW")
	procCoTaskMemFree        = windows.NewLazySystemDLL("ole32.dll").NewProc("CoTaskMemFree")
)

// browseInfo reprend BROWSEINFOW (champs alignés comme en C sur x64 et ARM64).
type browseInfo struct {
	owner       uintptr
	root        uintptr
	displayName *uint16
	title       *uint16
	flags       uint32
	callback    uintptr
	lParam      uintptr
	image       int32
}

const (
	bifReturnOnlyFSDirs = 0x0001
	bifNewDialogStyle   = 0x0040 // fenêtre redimensionnable, création de dossier
)

// PickFolder affiche le choix d'un dossier (sur le fil de la fenêtre, déjà initialisé pour COM).
func (p *winPlatform) PickFolder(title string) (string, error) {
	type result struct {
		path string
		err  error
	}
	done := make(chan result, 1)
	p.w.Dispatch(func() {
		path, err := browseForFolder(p.hwnd, title)
		done <- result{path, err}
		p.w.Dispatch(func() {})
	})
	r := <-done
	return r.path, r.err
}

func browseForFolder(owner windows.HWND, title string) (string, error) {
	name := make([]uint16, windows.MAX_PATH)
	caption, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return "", err
	}
	info := browseInfo{
		owner:       uintptr(owner),
		displayName: &name[0],
		title:       caption,
		flags:       bifReturnOnlyFSDirs | bifNewDialogStyle,
	}
	pidl, _, _ := procSHBrowseForFolderW.Call(uintptr(unsafe.Pointer(&info)))
	if pidl == 0 {
		return "", app.ErrCancelled
	}
	defer procCoTaskMemFree.Call(pidl)
	path := make([]uint16, windows.MAX_LONG_PATH)
	if ok, _, _ := procSHGetPathFromIDListW.Call(pidl, uintptr(unsafe.Pointer(&path[0]))); ok == 0 {
		return "", errors.New("ce n'est pas un dossier du disque : choisissez un dossier ordinaire")
	}
	return windows.UTF16ToString(path), nil
}

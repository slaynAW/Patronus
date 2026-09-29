package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/slaynaw/wakeonlan/desktop/internal/app"
	"github.com/slaynaw/wakeonlan/desktop/internal/diag"
)

var (
	shell32             = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteExW = shell32.NewProc("ShellExecuteExW")
)

// shellExecuteInfo reprend SHELLEXECUTEINFOW (champs alignés comme en C sur x64 et ARM64).
type shellExecuteInfo struct {
	size       uint32
	mask       uint32
	hwnd       uintptr
	verb       *uint16
	file       *uint16
	parameters *uint16
	directory  *uint16
	show       int32
	instApp    uintptr
	idList     uintptr
	class      *uint16
	keyClass   uintptr
	hotKey     uint32
	icon       uintptr
	process    windows.Handle
}

const (
	seeMaskNoCloseProcess = 0x00000040 // renvoie le processus lancé
	seeMaskNoAsync        = 0x00000100 // attend la fin du lancement (invite comprise)
)

// RunInstaller lance l'installation de l'agent téléchargé et vérifié par le moteur. Le fichier est
// ouvert en lecture seule sans partage en écriture ni en suppression : personne ne peut plus le
// modifier, le renommer ni l'effacer. Son empreinte est revérifiée sur ce fichier verrouillé, puis
// « install --pause » est lancé avec l'invite administrateur. Le verrou est gardé jusqu'à la fin de
// l'installation (l'agent copie alors son propre fichier), puis le fichier est effacé.
func (p *winPlatform) RunInstaller(path, want string) error {
	f, err := lockVerified(path, want)
	if err != nil {
		return err
	}
	process, err := runElevated(p.hwnd, path, "install --pause")
	if err != nil {
		f.Close()
		return err
	}
	diag.Info("agent", "installation lancée (invite administrateur acceptée)")
	go func() {
		_, _ = windows.WaitForSingleObject(process, windows.INFINITE)
		var code uint32
		if windows.GetExitCodeProcess(process, &code) == nil {
			diag.Info("agent", "installation terminée (code de sortie %d)", code)
		}
		_ = windows.CloseHandle(process)
		f.Close()
		if err := os.Remove(path); err != nil {
			diag.Warn("agent", "fichier d'installation non effacé : %v", err)
		}
	}()
	return nil
}

// lockVerified ouvre path en lecture seule sans partage en écriture ni en suppression (verrou tenu
// tant que le fichier renvoyé reste ouvert), puis vérifie son empreinte SHA-256 sur ce fichier.
func lockVerified(path, want string) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil,
		windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, fmt.Errorf("fichier de l'agent inaccessible : %w", err)
	}
	f := os.NewFile(uintptr(h), path)
	sum := sha256.New()
	if _, err := io.Copy(sum, f); err != nil || hex.EncodeToString(sum.Sum(nil)) != want {
		f.Close()
		return nil, errors.New("le fichier de l'agent a changé depuis sa vérification : installation annulée")
	}
	return f, nil
}

// runElevated lance file avec l'invite administrateur (« runas ») et renvoie son processus.
func runElevated(owner windows.HWND, file, params string) (windows.Handle, error) {
	// ShellExecuteEx demande COM sur le fil appelant.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE); err == nil || err == syscall.Errno(1) {
		defer windows.CoUninitialize() // S_OK ou S_FALSE (déjà initialisé) : à équilibrer
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	f, err := windows.UTF16PtrFromString(file)
	if err != nil {
		return 0, err
	}
	args, err := windows.UTF16PtrFromString(params)
	if err != nil {
		return 0, err
	}
	info := shellExecuteInfo{
		mask:       seeMaskNoCloseProcess | seeMaskNoAsync,
		hwnd:       uintptr(owner),
		verb:       verb,
		file:       f,
		parameters: args,
		show:       windows.SW_SHOWNORMAL,
	}
	info.size = uint32(unsafe.Sizeof(info))
	if ok, _, callErr := procShellExecuteExW.Call(uintptr(unsafe.Pointer(&info))); ok == 0 {
		if errors.Is(callErr, windows.ERROR_CANCELLED) {
			return 0, app.ErrCancelled
		}
		return 0, fmt.Errorf("lancement de l'installation impossible : %w", callErr)
	}
	if info.process == 0 {
		return 0, errors.New("installation lancée, mais impossible de la suivre")
	}
	return info.process, nil
}

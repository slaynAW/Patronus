package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"

	"github.com/slaynaw/wakeonlan/desktop/internal/app"
)

var (
	user32                            = windows.NewLazySystemDLL("user32.dll")
	procSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	procGetDpiForSystem               = user32.NewProc("GetDpiForSystem")
	procIsIconic                      = user32.NewProc("IsIconic")
	procFindWindowW                   = user32.NewProc("FindWindowW")
	procShowWindow                    = user32.NewProc("ShowWindow")
	procSetForegroundWindow           = user32.NewProc("SetForegroundWindow")
	procSystemParametersInfoW         = user32.NewProc("SystemParametersInfoW")
	procOpenClipboard                 = user32.NewProc("OpenClipboard")
	procCloseClipboard                = user32.NewProc("CloseClipboard")
	procGetClipboardData              = user32.NewProc("GetClipboardData")
	procEmptyClipboard                = user32.NewProc("EmptyClipboard")
	procSetClipboardData              = user32.NewProc("SetClipboardData")

	kernel32         = windows.NewLazySystemDLL("kernel32.dll")
	procGlobalLock   = kernel32.NewProc("GlobalLock")
	procGlobalUnlock = kernel32.NewProc("GlobalUnlock")
	procGlobalAlloc  = kernel32.NewProc("GlobalAlloc")
	procGlobalFree   = kernel32.NewProc("GlobalFree")

	comdlg32                 = windows.NewLazySystemDLL("comdlg32.dll")
	procGetSaveFileNameW     = comdlg32.NewProc("GetSaveFileNameW")
	procCommDlgExtendedError = comdlg32.NewProc("CommDlgExtendedError")
)

const (
	dpiAwarenessPerMonitorV2 = ^uintptr(3) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 = -4
	swRestore                = 9
	spiGetWorkArea           = 0x0030
	cfUnicodeText            = 13
	gmemMoveable             = 0x0002
	ofnOverwritePrompt       = 0x00000002
	ofnNoChangeDir           = 0x00000008
	ofnPathMustExist         = 0x00000800
	ofnExplorer              = 0x00080000
	idYes                    = 6
)

// singleInstance garde le mutex ouvert pendant toute la vie du programme.
var singleInstance windows.Handle

// setDPIAware active la netteté sur les écrans à haute densité (déjà demandé par le manifeste ;
// cet appel couvre une compilation sans ressources).
func setDPIAware() {
	if procSetProcessDpiAwarenessContext.Find() == nil {
		_, _, _ = procSetProcessDpiAwarenessContext.Call(dpiAwarenessPerMonitorV2)
	}
}

// scaled convertit une taille en pixels logiques (96 ppp) en pixels réels.
func scaled(px int) int {
	dpi := uintptr(96)
	if procGetDpiForSystem.Find() == nil {
		if d, _, _ := procGetDpiForSystem.Call(); d > 0 {
			dpi = d
		}
	}
	return px * int(dpi) / 96
}

// workArea renvoie la taille de la zone de travail de l'écran principal (hors barre des tâches).
func workArea() (int, int) {
	var r struct{ Left, Top, Right, Bottom int32 }
	if ok, _, _ := procSystemParametersInfoW.Call(spiGetWorkArea, 0, uintptr(unsafe.Pointer(&r)), 0); ok == 0 {
		return 0, 0
	}
	return int(r.Right - r.Left), int(r.Bottom - r.Top)
}

func acquireSingleInstance() bool {
	name, _ := windows.UTF16PtrFromString(`Local\WakeOnLan.Desktop`)
	h, err := windows.CreateMutex(nil, false, name)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		if h != 0 {
			_ = windows.CloseHandle(h)
		}
		return false
	}
	singleInstance = h
	return true
}

func focusExistingWindow(title string) {
	class, _ := windows.UTF16PtrFromString("webview")
	name, _ := windows.UTF16PtrFromString(title)
	hwnd, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(name)))
	if hwnd != 0 {
		_, _, _ = procShowWindow.Call(hwnd, swRestore)
		_, _, _ = procSetForegroundWindow.Call(hwnd)
	}
}

func isIconic(hwnd windows.HWND) bool {
	r, _, _ := procIsIconic.Call(uintptr(hwnd))
	return r != 0
}

func messageBox(text string, flags uint32) int32 {
	t, _ := windows.UTF16PtrFromString(text)
	c, _ := windows.UTF16PtrFromString(windowTitle)
	ret, _ := windows.MessageBox(0, t, c, flags)
	return ret
}

func shellOpen(hwnd windows.HWND, url string) error {
	verb, _ := windows.UTF16PtrFromString("open")
	file, err := windows.UTF16PtrFromString(url)
	if err != nil {
		return err
	}
	return windows.ShellExecute(windows.Handle(hwnd), verb, file, nil, nil, windows.SW_SHOWNORMAL)
}

// winPlatform implémente app.Platform avec les boîtes de dialogue et le presse-papiers de Windows.
type winPlatform struct {
	w    webview2.WebView
	hwnd windows.HWND
	// exe : programme en cours (relancé après une mise à jour).
	exe string
}

func (p *winPlatform) OpenURL(url string) error { return shellOpen(p.hwnd, url) }

// Relaunch démarre la nouvelle version (elle attend la fermeture de celle-ci pour prendre la main)
// puis ferme la fenêtre.
func (p *winPlatform) Relaunch() error {
	if p.exe == "" {
		return errors.New("emplacement de l'application inconnu")
	}
	cmd := exec.Command(p.exe, afterUpdateFlag)
	if err := cmd.Start(); err != nil {
		return err
	}
	_ = cmd.Process.Release()
	p.w.Dispatch(func() { p.w.Terminate() })
	return nil
}

// SaveFile affiche la boîte « Enregistrer sous » (sur le fil de la fenêtre) puis écrit le fichier.
func (p *winPlatform) SaveFile(suggestedName string, content []byte) (string, error) {
	type result struct {
		path string
		err  error
	}
	done := make(chan result, 1)
	p.w.Dispatch(func() {
		path, err := saveFileDialog(p.hwnd, suggestedName)
		done <- result{path, err}
		// Les messages postés pendant la boîte modale ont pu être absorbés : relance la file.
		p.w.Dispatch(func() {})
	})
	r := <-done
	if r.err != nil {
		return "", r.err
	}
	tmp := r.path + ".tmp"
	if err := os.WriteFile(tmp, content, 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, r.path); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return r.path, nil
}

type openFileName struct {
	structSize    uint32
	owner         uintptr
	instance      uintptr
	filter        *uint16
	customFilter  *uint16
	maxCustFilter uint32
	filterIndex   uint32
	file          *uint16
	maxFile       uint32
	fileTitle     *uint16
	maxFileTitle  uint32
	initialDir    *uint16
	title         *uint16
	flags         uint32
	fileOffset    uint16
	fileExtension uint16
	defExt        *uint16
	custData      uintptr
	hook          uintptr
	templateName  *uint16
	reserved      unsafe.Pointer
	reserved2     uint32
	flagsEx       uint32
}

func saveFileDialog(owner windows.HWND, suggestedName string) (string, error) {
	file := make([]uint16, windows.MAX_LONG_PATH)
	copy(file, windows.StringToUTF16(suggestedName))
	filter := utf16List("Sauvegarde Wake On LAN (*.json)", "*.json", "Tous les fichiers", "*.*")
	defExt, _ := windows.UTF16PtrFromString("json")
	title, _ := windows.UTF16PtrFromString("Exporter la configuration")
	var initialDir *uint16
	if home, err := os.UserHomeDir(); err == nil {
		initialDir, _ = windows.UTF16PtrFromString(filepath.Join(home, "Documents"))
	}
	ofn := openFileName{
		owner:       uintptr(owner),
		filter:      &filter[0],
		filterIndex: 1,
		file:        &file[0],
		maxFile:     uint32(len(file)),
		initialDir:  initialDir,
		title:       title,
		flags:       ofnOverwritePrompt | ofnNoChangeDir | ofnPathMustExist | ofnExplorer,
		defExt:      defExt,
	}
	ofn.structSize = uint32(unsafe.Sizeof(ofn))
	if ok, _, _ := procGetSaveFileNameW.Call(uintptr(unsafe.Pointer(&ofn))); ok == 0 {
		if code, _, _ := procCommDlgExtendedError.Call(); code != 0 {
			return "", errors.New("boîte de dialogue d'enregistrement indisponible")
		}
		return "", app.ErrCancelled
	}
	return windows.UTF16ToString(file), nil
}

// utf16List construit une liste de chaînes séparées par des zéros et terminée par un double zéro.
func utf16List(items ...string) []uint16 {
	var out []uint16
	for _, s := range items {
		out = append(out, windows.StringToUTF16(s)...)
	}
	return append(out, 0)
}

// openClipboard ouvre le presse-papiers (quelques essais : une autre application peut le tenir).
func openClipboard() error {
	for range 5 {
		if opened, _, _ := procOpenClipboard.Call(0); opened != 0 {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return errors.New("presse-papiers occupé")
}

// ReadClipboard lit le texte du presse-papiers.
func (p *winPlatform) ReadClipboard() (string, error) {
	if err := openClipboard(); err != nil {
		return "", err
	}
	defer procCloseClipboard.Call()
	handle, _, _ := procGetClipboardData.Call(cfUnicodeText)
	if handle == 0 {
		return "", nil
	}
	ptr, _, _ := procGlobalLock.Call(handle)
	if ptr == 0 {
		return "", errors.New("presse-papiers illisible")
	}
	defer procGlobalUnlock.Call(handle)
	text := windows.UTF16PtrToString(*(**uint16)(unsafe.Pointer(&ptr)))
	// Seuls les liens de l'application sont utiles (appairage, partage) : on ne renvoie pas le reste
	// du presse-papiers à l'interface.
	lower := strings.ToLower(strings.TrimSpace(text))
	if !strings.HasPrefix(lower, "wolagent://") && !strings.HasPrefix(lower, "wolshare://") {
		return "", nil
	}
	return text, nil
}

// WriteClipboard copie un texte dans le presse-papiers.
func (p *winPlatform) WriteClipboard(text string) error {
	data, err := windows.UTF16FromString(text)
	if err != nil {
		return err
	}
	if err := openClipboard(); err != nil {
		return err
	}
	defer procCloseClipboard.Call()
	procEmptyClipboard.Call()
	handle, _, _ := procGlobalAlloc.Call(gmemMoveable, uintptr(len(data)*2))
	if handle == 0 {
		return errors.New("mémoire insuffisante")
	}
	ptr, _, _ := procGlobalLock.Call(handle)
	if ptr == 0 {
		procGlobalFree.Call(handle)
		return errors.New("presse-papiers inaccessible")
	}
	copy(unsafe.Slice(*(**uint16)(unsafe.Pointer(&ptr)), len(data)), data)
	procGlobalUnlock.Call(handle)
	if set, _, _ := procSetClipboardData.Call(cfUnicodeText, handle); set == 0 {
		procGlobalFree.Call(handle)
		return errors.New("copie impossible")
	}
	return nil // le presse-papiers possède désormais la mémoire
}

// Attributs DWM de la barre de titre propres à Windows 11 (refusés, sans conséquence, par Windows 10).
const (
	dwmwaCaptionColor = 35
	dwmwaTextColor    = 36
)

// setDarkTitleBar donne à la barre de titre le thème sombre de l'interface (Windows 10 20H1+) et,
// sous Windows 11, la couleur de la barre de navigation (#161A1F, texte #E7E9EC).
func setDarkTitleBar(hwnd windows.HWND) {
	set := func(attribute uint32, value uint32) {
		_ = windows.DwmSetWindowAttribute(hwnd, attribute, unsafe.Pointer(&value), uint32(unsafe.Sizeof(value)))
	}
	set(windows.DWMWA_USE_IMMERSIVE_DARK_MODE, 1)
	set(dwmwaCaptionColor, colorRef(0x16, 0x1a, 0x1f))
	set(dwmwaTextColor, colorRef(0xe7, 0xe9, 0xec))
}

// colorRef code une couleur au format COLORREF de Windows (0x00BBGGRR).
func colorRef(r, g, b uint32) uint32 { return b<<16 | g<<8 | r }

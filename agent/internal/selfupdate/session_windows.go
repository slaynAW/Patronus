package selfupdate

import (
	"context"
	"os/exec"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Le service tourne dans la session 0, sans bureau : ses fenêtres sont affichées dans la session de
// l'utilisateur connecté par WTSSendMessage (boîte de message classique, gérée par Windows).

var procWTSSendMessage = windows.NewLazySystemDLL("wtsapi32.dll").NewProc("WTSSendMessageW")

const (
	mbOK              = 0x0
	mbYesNo           = 0x4
	mbIconWarning     = 0x30
	mbIconInformation = 0x40
	idYes             = 6
)

// Supported indique que l'installation des mises à jour est prise en charge sur ce système.
const Supported = true

// userSession renvoie la session où un utilisateur est connecté (la console en priorité, sinon une
// session à distance active), ou false si personne n'est connecté.
func userSession() (uint32, bool) {
	hasUser := func(id uint32) bool {
		var token windows.Token
		if windows.WTSQueryUserToken(id, &token) != nil {
			return false
		}
		token.Close()
		return true
	}
	if id := windows.WTSGetActiveConsoleSessionId(); id != 0xFFFFFFFF && hasUser(id) {
		return id, true
	}
	var sessions *windows.WTS_SESSION_INFO
	var count uint32
	if windows.WTSEnumerateSessions(0, 0, 1, &sessions, &count) != nil {
		return 0, false
	}
	defer windows.WTSFreeMemory(uintptr(unsafe.Pointer(sessions)))
	for _, s := range unsafe.Slice(sessions, count) {
		if s.State == windows.WTSActive && s.SessionID != 0 && hasUser(s.SessionID) {
			return s.SessionID, true
		}
	}
	return 0, false
}

// send affiche une boîte de message dans la session de l'utilisateur ; wait : attend la réponse
// (au plus timeout) et la renvoie.
func send(style uint32, message string, timeout time.Duration, wait bool) (uint32, bool, error) {
	session, ok := userSession()
	if !ok {
		return 0, false, nil
	}
	title, err := windows.UTF16FromString(Title)
	if err != nil {
		return 0, true, err
	}
	text, err := windows.UTF16FromString(message)
	if err != nil {
		return 0, true, err
	}
	var response uint32
	var waitFlag uintptr
	if wait {
		waitFlag = 1
	}
	r, _, callErr := procWTSSendMessage.Call(
		0, // WTS_CURRENT_SERVER_HANDLE
		uintptr(session),
		uintptr(unsafe.Pointer(&title[0])), uintptr((len(title)-1)*2),
		uintptr(unsafe.Pointer(&text[0])), uintptr((len(text)-1)*2),
		uintptr(style),
		uintptr(timeout/time.Second),
		uintptr(unsafe.Pointer(&response)),
		waitFlag,
	)
	if r == 0 {
		return 0, true, callErr
	}
	return response, true, nil
}

// Ask propose la mise à jour à l'utilisateur connecté (fenêtre « Oui / Non »).
func Ask(_ context.Context, current string, o Offer) (Answer, error) {
	if testAccept == "1" {
		return Accept, nil
	}
	response, found, err := send(mbYesNo|mbIconInformation, Message(current, o), AskTimeout, true)
	switch {
	case !found:
		return NoUser, nil
	case err != nil:
		return Later, err
	case response == idYes:
		return Accept, nil
	}
	return Later, nil
}

// Notify informe l'utilisateur connecté (sans attendre sa réponse) ; sans effet si personne n'est connecté.
func Notify(message string, warning bool) {
	style := uint32(mbOK | mbIconInformation)
	if warning {
		style = mbOK | mbIconWarning
	}
	_, _, _ = send(style, message, 10*time.Minute, false)
}

// StartFinisher lance « <exe> update-finish » dans un processus détaché : il survit à l'arrêt du
// service qu'il redémarre.
func StartFinisher(exe string) error {
	cmd := exec.Command(exe, "update-finish")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP,
		HideWindow:    true,
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

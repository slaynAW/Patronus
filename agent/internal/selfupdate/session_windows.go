package selfupdate

import (
	"context"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"

	"github.com/slaynaw/wakeonlan/agent/internal/session"
)

// Supported indique que l'installation des mises à jour est prise en charge sur ce système.
const Supported = true

// Ask propose la mise à jour à l'utilisateur connecté (fenêtre « Oui / Non »).
func Ask(_ context.Context, current string, o Offer) (Answer, error) {
	if testAccept == "1" {
		return Accept, nil
	}
	answer, err := session.Ask(Message(current, o), AskTimeout, false)
	switch {
	case answer == session.NoUser:
		return NoUser, nil
	case err != nil:
		return Later, err
	case answer == session.Yes:
		return Accept, nil
	}
	return Later, nil
}

// Notify informe l'utilisateur connecté (sans attendre sa réponse) ; sans effet si personne n'est connecté.
func Notify(message string, warning bool) { session.Notify(message, warning) }

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

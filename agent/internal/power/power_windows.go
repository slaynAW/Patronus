package power

import (
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/sys/windows"
)

type systemController struct{}

func (systemController) Do(action Action, force bool) error {
	switch action {
	case Shutdown, Reboot:
		flag := "/s"
		if action == Reboot {
			flag = "/r"
		}
		// Arrêt COMPLET (pas l'arrêt « hybride » du démarrage rapide), ce qui fiabilise le
		// Wake-on-LAN suivant. /d p:0:0 = arrêt planifié, raison « Autre ».
		args := []string{flag, "/t", "0", "/d", "p:0:0", "/c", "Demandé depuis l'application Patronus"}
		if force {
			args = append(args, "/f")
		}
		return exec.Command(filepath.Join(os.Getenv("SystemRoot"), "System32", "shutdown.exe"), args...).Run()
	case Sleep:
		return suspend()
	}
	return unsupported(action)
}

var (
	powrprof        = windows.NewLazySystemDLL("powrprof.dll")
	setSuspendState = powrprof.NewProc("SetSuspendState")
)

// suspend met le PC en veille (S3 / veille moderne) via SetSuspendState(hibernate=FALSE).
func suspend() error {
	if err := enableShutdownPrivilege(); err != nil {
		return err
	}
	if err := setSuspendState.Find(); err != nil {
		return err
	}
	ret, _, callErr := setSuspendState.Call(0, 0, 0)
	if ret == 0 {
		return callErr
	}
	return nil
}

// enableShutdownPrivilege active SeShutdownPrivilege, requis par SetSuspendState.
func enableShutdownPrivilege() error {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &token); err != nil {
		return err
	}
	defer token.Close()
	var luid windows.LUID
	if err := windows.LookupPrivilegeValue(nil, windows.StringToUTF16Ptr("SeShutdownPrivilege"), &luid); err != nil {
		return err
	}
	privileges := windows.Tokenprivileges{
		PrivilegeCount: 1,
		Privileges:     [1]windows.LUIDAndAttributes{{Luid: luid, Attributes: windows.SE_PRIVILEGE_ENABLED}},
	}
	return windows.AdjustTokenPrivileges(token, false, &privileges, 0, nil, nil)
}

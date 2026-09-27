package power

import "os/exec"

type systemController struct{}

func (systemController) Do(action Action, force bool) error {
	args := map[Action][]string{
		Shutdown: {"poweroff"},
		Reboot:   {"reboot"},
		Sleep:    {"suspend"},
	}[action]
	if args == nil {
		return unsupported(action)
	}
	if force {
		// Ignore les « inhibiteurs » (applications qui retardent l'arrêt).
		args = append(args, "--ignore-inhibitors")
	}
	if path, err := exec.LookPath("systemctl"); err == nil {
		return exec.Command(path, args...).Run()
	}
	// Systèmes sans systemd.
	switch action {
	case Shutdown:
		return exec.Command("shutdown", "-h", "now").Run()
	case Reboot:
		return exec.Command("shutdown", "-r", "now").Run()
	}
	return unsupported(action)
}

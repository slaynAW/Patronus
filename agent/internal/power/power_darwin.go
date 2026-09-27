package power

import "os/exec"

type systemController struct{}

func (systemController) Do(action Action, _ bool) error {
	switch action {
	case Shutdown:
		return exec.Command("/sbin/shutdown", "-h", "now").Run()
	case Reboot:
		return exec.Command("/sbin/shutdown", "-r", "now").Run()
	case Sleep:
		return exec.Command("/usr/bin/pmset", "sleepnow").Run()
	}
	return unsupported(action)
}

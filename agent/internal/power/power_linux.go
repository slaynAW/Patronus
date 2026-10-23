package power

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/slaynaw/wakeonlan/agent/internal/dsm"
)

type systemController struct{}

func (systemController) Do(action Action, force bool) error {
	if dsm.Detect() {
		return dsmDo(action)
	}
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

// dsmDo : sur un NAS Synology, l'extinction et le redémarrage passent par l'outil de DSM (synopoweroff),
// lancé par systemd hors du bac à sable de l'agent (unités écrites par « wol-agent install »). DSM arrête
// proprement ses paquets et ses volumes, et refuse pendant une opération critique : « forcer » est donc sans
// effet. La veille n'existe pas sur un NAS.
func dsmDo(action Action) error {
	unit, err := dsmUnit(action)
	if err != nil {
		return err
	}
	if out, err := exec.Command("systemctl", "start", "--no-block", unit).CombinedOutput(); err != nil {
		return fmt.Errorf("%s : %v (%s) ; relancez « wol-agent install »", unit, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func dsmUnit(action Action) (string, error) {
	switch action {
	case Shutdown:
		return dsm.PowerOffUnit, nil
	case Reboot:
		return dsm.RebootUnit, nil
	}
	return "", fmt.Errorf("%s non prise en charge sur un NAS Synology", action.Label())
}

package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/slaynaw/wakeonlan/agent/internal/dsm"
)

const (
	unitDir  = "/etc/systemd/system/"
	unitName = "wol-agent.service"
	unitPath = unitDir + unitName
)

// DefaultBinary est l'emplacement d'installation de l'exécutable.
func DefaultBinary() string { return "/usr/local/bin/wol-agent" }

const unitTemplate = `[Unit]
Description=Patronus - agent d'extinction à distance
Documentation=https://github.com/slaynAW/Patronus
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=%s run --config %s
Restart=always
RestartSec=5
# Durcissement : l'agent n'a besoin que du réseau, de systemctl et de son dossier (identité, appareils
# autorisés, journal, mesures), le seul où il écrit. ReadWriteDirectories, ancien nom de ReadWritePaths,
# est compris par toutes les versions de systemd (DSM compris).
NoNewPrivileges=yes
ProtectSystem=full
ReadWriteDirectories=%s
ProtectHome=yes
PrivateTmp=yes
ProtectKernelTunables=yes
ProtectKernelModules=yes
ProtectControlGroups=yes
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6 AF_NETLINK
LockPersonality=yes

[Install]
WantedBy=multi-user.target
`

// unitFile : service de l'agent, qui n'écrit que dans le dossier de sa configuration.
func unitFile(binary, config string) string {
	return fmt.Sprintf(unitTemplate, binary, strconv.Quote(config), filepath.Dir(config))
}

// dsmUnitTemplate : unité ponctuelle qui éteint ou redémarre un NAS Synology avec l'outil de DSM, démarrée
// par l'agent (sans [Install] : jamais au démarrage).
const dsmUnitTemplate = `[Unit]
Description=Patronus - %s du NAS demandé par l'agent (outil de DSM)

[Service]
Type=oneshot
ExecStart=%s
`

// dsmUnits : unités d'extinction et de redémarrage d'un NAS Synology, par nom.
func dsmUnits() map[string]string {
	return map[string]string{
		dsm.PowerOffUnit: fmt.Sprintf(dsmUnitTemplate, "extinction", dsm.PowerOffTool),
		dsm.RebootUnit:   fmt.Sprintf(dsmUnitTemplate, "redémarrage", dsm.PowerOffTool+" -r"),
	}
}

// Install installe (ou met à jour) le service systemd et ouvre le pare-feu s'il est actif.
func Install(opts Options) error {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return errors.New("systemd est requis ; sinon, lancez « wol-agent run » avec votre gestionnaire de services")
	}
	_ = run("systemctl", "stop", unitName)
	if err := CopySelf(opts.Binary); err != nil {
		return err
	}
	if err := os.WriteFile(unitPath, []byte(unitFile(opts.Binary, opts.Config)), 0o644); err != nil {
		return err
	}
	if dsm.Detect() {
		for name, unit := range dsmUnits() {
			if err := os.WriteFile(unitDir+name, []byte(unit), 0o644); err != nil {
				return err
			}
		}
	}
	if err := run("systemctl", "daemon-reload"); err != nil {
		return err
	}
	if err := run("systemctl", "enable", "--now", unitName); err != nil {
		return err
	}
	if err := run("systemctl", "restart", unitName); err != nil {
		return err
	}
	if !opts.NoFirewall {
		openFirewall(opts.Port)
	}
	return nil
}

// Uninstall arrête et supprime le service.
func Uninstall() error {
	_ = run("systemctl", "disable", "--now", unitName)
	for _, path := range []string{unitPath, unitDir + dsm.PowerOffUnit, unitDir + dsm.RebootUnit} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	_ = run("systemctl", "daemon-reload")
	return nil
}

// Restart redémarre le service.
func Restart() error { return run("systemctl", "restart", unitName) }

// RefreshLabels : rien à renommer (pas de mise à jour automatique hors Windows).
func RefreshLabels() {}

// IsRunning indique si le service est démarré.
func IsRunning() bool {
	return exec.Command("systemctl", "is-active", "--quiet", unitName).Run() == nil
}

// Status décrit l'état du service.
func Status() string {
	out, _ := exec.Command("systemctl", "is-active", unitName).Output()
	switch state := strings.TrimSpace(string(out)); state {
	case "active":
		return "en cours d'exécution"
	case "":
		return "non installé"
	default:
		return state
	}
}

// Run : sous Linux, systemd lance simplement « wol-agent run ».
func Run(func(ctx context.Context) error) (bool, error) { return false, nil }

// openFirewall ouvre le port si firewalld ou ufw est actif (sinon, rien à faire).
func openFirewall(port int) {
	rule := strconv.Itoa(port) + "/tcp"
	if exec.Command("firewall-cmd", "--state").Run() == nil {
		if run("firewall-cmd", "--permanent", "--add-port="+rule) == nil {
			_ = run("firewall-cmd", "--reload")
			fmt.Println("Pare-feu (firewalld) : port", rule, "ouvert.")
		}
		return
	}
	if out, err := exec.Command("ufw", "status").Output(); err == nil && strings.Contains(string(out), "Status: active") {
		if run("ufw", "allow", rule) == nil {
			fmt.Println("Pare-feu (ufw) : port", rule, "ouvert.")
		}
	}
}

func run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s : %v (%s)", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

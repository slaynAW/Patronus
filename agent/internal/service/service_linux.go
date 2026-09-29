package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

const (
	unitName = "wol-agent.service"
	unitPath = "/etc/systemd/system/" + unitName
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
# Durcissement : l'agent n'écrit rien sur le disque et n'a besoin que du réseau et de systemctl.
NoNewPrivileges=yes
ProtectSystem=full
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

// Install installe (ou met à jour) le service systemd et ouvre le pare-feu s'il est actif.
func Install(opts Options) error {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return errors.New("systemd est requis ; sinon, lancez « wol-agent run » avec votre gestionnaire de services")
	}
	_ = run("systemctl", "stop", unitName)
	if err := CopySelf(opts.Binary); err != nil {
		return err
	}
	unit := fmt.Sprintf(unitTemplate, opts.Binary, strconv.Quote(opts.Config))
	if err := os.WriteFile(unitPath, []byte(unit), 0o644); err != nil {
		return err
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
	if err := os.Remove(unitPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
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

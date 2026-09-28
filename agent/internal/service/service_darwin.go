package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const (
	label     = "io.github.slaynaw.wolagent"
	plistPath = "/Library/LaunchDaemons/" + label + ".plist"
)

// DefaultBinary est l'emplacement d'installation de l'exécutable.
func DefaultBinary() string { return "/usr/local/bin/wol-agent" }

const plistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key><string>%s</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>run</string>
        <string>--config</string>
        <string>%s</string>
    </array>
    <key>RunAtLoad</key><true/>
    <key>KeepAlive</key><true/>
    <key>StandardErrorPath</key><string>/Library/Logs/wol-agent.log</string>
    <key>StandardOutPath</key><string>/Library/Logs/wol-agent.log</string>
</dict>
</plist>
`

// Install installe (ou met à jour) le démon launchd.
func Install(opts Options) error {
	_ = run("launchctl", "bootout", "system/"+label)
	if err := CopySelf(opts.Binary); err != nil {
		return err
	}
	plist := fmt.Sprintf(plistTemplate, label, xmlEscape(opts.Binary), xmlEscape(opts.Config))
	if err := os.WriteFile(plistPath, []byte(plist), 0o644); err != nil {
		return err
	}
	return run("launchctl", "bootstrap", "system", plistPath)
}

// Uninstall arrête et supprime le démon.
func Uninstall() error {
	_ = run("launchctl", "bootout", "system/"+label)
	if err := os.Remove(plistPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// Restart redémarre le démon.
func Restart() error { return run("launchctl", "kickstart", "-k", "system/"+label) }

// IsRunning indique si le service est chargé par launchd.
func IsRunning() bool { return exec.Command("launchctl", "print", "system/"+label).Run() == nil }

// Status décrit l'état du démon.
func Status() string {
	if exec.Command("launchctl", "print", "system/"+label).Run() != nil {
		return "non installé ou arrêté"
	}
	return "en cours d'exécution"
}

// Run : sous macOS, launchd lance simplement « wol-agent run ».
func Run(func(ctx context.Context) error) (bool, error) { return false, nil }

func run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s : %v (%s)", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

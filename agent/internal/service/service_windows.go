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
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	name         = "WolAgent"
	displayName  = "Wake On LAN - Agent"
	description  = "Permet d'éteindre, redémarrer ou mettre en veille ce PC depuis l'application Wake On LAN (réseau local, commandes authentifiées)."
	firewallRule = "Wake On LAN - Agent"
)

// DefaultBinary est l'emplacement d'installation de l'exécutable.
func DefaultBinary() string {
	base := os.Getenv("ProgramFiles")
	if base == "" {
		base = `C:\Program Files`
	}
	return filepath.Join(base, "WolAgent", "wol-agent.exe")
}

// Install installe (ou met à jour) et démarre le service Windows, puis ouvre le pare-feu.
func Install(opts Options) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("accès au gestionnaire de services impossible (droits administrateur ?) : %w", err)
	}
	defer m.Disconnect()

	// Mise à jour : on arrête l'ancien service pour pouvoir remplacer l'exécutable.
	if s, err := m.OpenService(name); err == nil {
		stop(s)
		s.Close()
	}
	if err := CopySelf(opts.Binary); err != nil {
		return err
	}

	s, err := m.OpenService(name)
	if err == nil {
		// Service existant : on repart de sa configuration actuelle (champs non modifiés conservés).
		cfg, err := s.Config()
		if err != nil {
			s.Close()
			return err
		}
		cfg.DisplayName = displayName
		cfg.Description = description
		cfg.StartType = mgr.StartAutomatic
		cfg.BinaryPathName = syscall.EscapeArg(opts.Binary) + " run --config " + syscall.EscapeArg(opts.Config)
		if err := s.UpdateConfig(cfg); err != nil {
			s.Close()
			return err
		}
	} else {
		cfg := mgr.Config{DisplayName: displayName, Description: description, StartType: mgr.StartAutomatic}
		s, err = m.CreateService(name, opts.Binary, cfg, "run", "--config", opts.Config)
		if err != nil {
			return fmt.Errorf("création du service impossible : %w", err)
		}
	}
	defer s.Close()

	// Relance automatique en cas d'arrêt inattendu.
	restart := mgr.RecoveryAction{Type: mgr.ServiceRestart, Delay: 5 * time.Second}
	_ = s.SetRecoveryActions([]mgr.RecoveryAction{restart, restart, restart}, 24*3600)

	if err := s.Start(); err != nil && !errors.Is(err, windows.ERROR_SERVICE_ALREADY_RUNNING) {
		return fmt.Errorf("démarrage du service impossible : %w", err)
	}
	if !opts.NoFirewall {
		return openFirewall(opts)
	}
	return nil
}

// Uninstall arrête et supprime le service et la règle de pare-feu.
func Uninstall() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	if s, err := m.OpenService(name); err == nil {
		stop(s)
		if err := s.Delete(); err != nil {
			s.Close()
			return err
		}
		s.Close()
	}
	_ = netsh("delete", "rule", "name="+firewallRule)
	return nil
}

// Restart redémarre le service (après un changement de clé).
func Restart() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(name)
	if err != nil {
		return errors.New("service non installé")
	}
	defer s.Close()
	stop(s)
	return s.Start()
}

// IsRunning indique si le service est démarré.
func IsRunning() bool {
	m, err := mgr.Connect()
	if err != nil {
		return false
	}
	defer m.Disconnect()
	s, err := m.OpenService(name)
	if err != nil {
		return false
	}
	defer s.Close()
	st, err := s.Query()
	return err == nil && st.State == svc.Running
}

// Status décrit l'état du service.
func Status() string {
	m, err := mgr.Connect()
	if err != nil {
		return "inconnu (" + err.Error() + ")"
	}
	defer m.Disconnect()
	s, err := m.OpenService(name)
	if err != nil {
		return "non installé"
	}
	defer s.Close()
	st, err := s.Query()
	if err != nil {
		return "inconnu"
	}
	switch st.State {
	case svc.Running:
		return "en cours d'exécution"
	case svc.Stopped:
		return "arrêté"
	default:
		return "en transition (" + strconv.Itoa(int(st.State)) + ")"
	}
}

// Run exécute run() comme service Windows si le programme a été lancé par le gestionnaire de services.
func Run(run func(ctx context.Context) error) (bool, error) {
	isService, err := svc.IsWindowsService()
	if err != nil || !isService {
		return false, err
	}
	return true, svc.Run(name, &handler{run: run})
}

type handler struct {
	run func(ctx context.Context) error
}

func (h *handler) Execute(_ []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	done := make(chan error, 1)
	go func() { done <- h.run(ctx) }()
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case req := <-requests:
			switch req.Cmd {
			case svc.Interrogate:
				status <- req.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending}
				if req.Cmd == svc.Shutdown {
					cancel(ErrSystemShutdown)
				} else {
					cancel(ErrStopRequested)
				}
				<-done
				return false, 0
			}
		case err := <-done:
			if err != nil {
				return true, 1 // code d'erreur : Windows relancera le service
			}
			return false, 0
		}
	}
}

func stop(s *mgr.Service) {
	if _, err := s.Control(svc.Stop); err != nil {
		return
	}
	for i := 0; i < 40; i++ {
		st, err := s.Query()
		if err != nil || st.State == svc.Stopped {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func openFirewall(opts Options) error {
	_ = netsh("delete", "rule", "name="+firewallRule)
	profiles := "private,domain"
	if opts.FirewallPublic {
		profiles = "any"
	}
	// Uniquement le sous-réseau local, et uniquement pour l'exécutable de l'agent.
	return netsh("add", "rule",
		"name="+firewallRule,
		"dir=in", "action=allow", "protocol=TCP",
		"localport="+strconv.Itoa(opts.Port),
		"program="+opts.Binary,
		"profile="+profiles,
		"remoteip=localsubnet",
		"enable=yes",
	)
}

// netsh exécute « netsh advfirewall firewall … ». La ligne de commande est construite à la main :
// netsh attend la forme clé="valeur avec espaces", que l'échappement standard ne produit pas.
func netsh(args ...string) error {
	exe := filepath.Join(os.Getenv("SystemRoot"), "System32", "netsh.exe")
	line := syscall.EscapeArg(exe) + " advfirewall firewall"
	for _, arg := range args {
		if k, v, ok := strings.Cut(arg, "="); ok && strings.ContainsAny(v, " \t") {
			arg = k + `="` + v + `"`
		}
		line += " " + arg
	}
	cmd := exec.Command(exe)
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: line}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pare-feu : %v (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

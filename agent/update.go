package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/slaynaw/wakeonlan/agent/internal/config"
	"github.com/slaynaw/wakeonlan/agent/internal/selfupdate"
	"github.com/slaynaw/wakeonlan/agent/internal/service"
	"github.com/slaynaw/wakeonlan/agent/internal/terminal"
	"github.com/slaynaw/wakeonlan/agent/update"
)

// Mises à jour de l'agent (voir le package selfupdate).

func updateSource() (update.Source, error) {
	src, err := update.Official()
	src.UserAgent = "wol-agent/" + version
	return src, err
}

// startAutoUpdate lance la recherche quotidienne des mises à jour dans le service (Windows).
func startAutoUpdate(ctx context.Context, cfgPath string, logger *log.Logger) {
	if !selfupdate.Supported || version == "dev" {
		return
	}
	cfg, err := config.Load(cfgPath)
	if err != nil || !cfg.AutoUpdates() {
		return
	}
	src, err := updateSource()
	if err != nil {
		logger.Printf("mise à jour : %v", err)
		return
	}
	exe, err := os.Executable()
	if err != nil {
		logger.Printf("mise à jour : %v", err)
		return
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	auto := &selfupdate.Auto{
		Source:    src,
		Current:   version,
		StatePath: filepath.Join(filepath.Dir(cfgPath), "update-state.json"),
		Ask: func(ctx context.Context, o selfupdate.Offer) (selfupdate.Answer, error) {
			return selfupdate.Ask(ctx, version, o)
		},
		Apply: func(ctx context.Context, o selfupdate.Offer) error {
			next, err := selfupdate.Download(ctx, src, o, exe)
			if err != nil {
				return err
			}
			if err := update.Replace(exe, next); err != nil {
				_ = os.Remove(next)
				return err
			}
			// La nouvelle version redémarre le service depuis un processus détaché.
			if err := selfupdate.StartFinisher(exe); err != nil {
				_ = selfupdate.Restore(exe)
				return err
			}
			return nil
		},
		Logger:     logger,
		FirstDelay: selfupdate.StartDelay(),
	}
	go auto.Run(ctx)
}

// cmdUpdateFinish est lancé par le service (nouvelle version, processus détaché) une fois
// l'exécutable remplacé : redémarre le service, ou revient à la version précédente.
func cmdUpdateFinish() error {
	logger, closeLog := newLogger()
	defer closeLog()
	exe := service.DefaultBinary()
	if err := selfupdate.Finish(exe, service.Restart, serviceHealthy); err != nil {
		logger.Printf("mise à jour : %v", err)
		selfupdate.Notify("La mise à jour de l'agent Patronus a échoué : "+err.Error()+".", true)
		return err
	}
	update.CleanupOld(exe)
	logger.Printf("mise à jour : agent %s installé", version)
	selfupdate.Notify(fmt.Sprintf("L'agent Patronus a été mis à jour (version %s).", version), false)
	// Nom affiché (service, pare-feu) des installations antérieures au passage à « Patronus » ; en
	// dernier : la mise à jour est terminée et notée au journal, quoi qu'il arrive ici.
	service.RefreshLabels()
	return nil
}

// serviceHealthy attend que le service redémarré accepte les connexions (20 s au plus).
func serviceHealthy() bool {
	cfg, err := config.Load(config.DefaultPath())
	if err != nil {
		return false
	}
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(cfg.Port))
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(500 * time.Millisecond) {
		if !service.IsRunning() {
			continue
		}
		if conn, err := net.DialTimeout("tcp", addr, 2*time.Second); err == nil {
			_ = conn.Close()
			return true
		}
	}
	return false
}

func cmdUpdate(args []string) error {
	fs := flag.NewFlagSet("update", flag.ExitOnError)
	yes := fs.Bool("yes", false, "installer sans demander de confirmation")
	check := fs.Bool("check", false, "vérifier seulement, sans installer")
	auto := fs.String("auto", "", "« on » ou « off » : active ou désactive la recherche quotidienne (Windows)")
	pause := fs.Bool("pause", false, "attendre une touche à la fin")
	_ = fs.Parse(args)

	needsAdmin := *auto != "" || (!*check && selfupdate.Supported)
	if needsAdmin && !terminal.IsAdmin() {
		if runtime.GOOS == "windows" {
			fmt.Println("Droits administrateur nécessaires : validez la demande de Windows…")
			return terminal.RelaunchElevated(append([]string{"update"}, withPause(args)...))
		}
		return errors.New("droits administrateur nécessaires : relancez avec sudo")
	}
	if *pause {
		defer terminal.Pause()
	}
	if *auto != "" {
		return setAutoUpdate(*auto)
	}

	ctx := context.Background()
	exe := service.DefaultBinary()
	current := version
	if v, err := selfupdate.ProbeVersion(ctx, exe); err == nil {
		current = v // version de l'agent installé
	} else if !*check {
		return fmt.Errorf("agent non installé (%s) : lancez d'abord « wol-agent install »", exe)
	}
	src, err := updateSource()
	if err != nil {
		return err
	}
	offer, err := selfupdate.Check(ctx, src, current)
	if errors.Is(err, update.ErrNotPublished) {
		fmt.Println("Aucune version publiée ne propose encore de mise à jour de l'agent.")
		return nil
	}
	if err != nil {
		return fmt.Errorf("recherche de mise à jour impossible : %w", err)
	}
	if offer == nil {
		fmt.Printf("✔ L'agent est à jour (version %s).\n", current)
		return nil
	}
	fmt.Printf("Nouvelle version de l'agent : %s → %s\n", current, offer.Version)
	if notes := strings.TrimSpace(offer.Notes); notes != "" {
		fmt.Println("\n" + notes + "\n")
	}
	if *check {
		return nil
	}
	if !selfupdate.Supported {
		fmt.Println("Installation automatique : Windows uniquement. Téléchargez la nouvelle version depuis")
		fmt.Println(update.DefaultBase + "/latest puis relancez « sudo wol-agent install » (la clé est conservée).")
		return nil
	}
	if !*yes && !confirm("Installer maintenant ? [O/n] ") {
		return nil
	}
	fmt.Println("Téléchargement…")
	next, err := selfupdate.Download(ctx, src, *offer, exe)
	if err != nil {
		return err
	}
	if err := update.Replace(exe, next); err != nil {
		_ = os.Remove(next)
		return err
	}
	fmt.Println("Redémarrage du service…")
	if err := selfupdate.Finish(exe, service.Restart, serviceHealthy); err != nil {
		return err
	}
	// L'ancienne version peut être ce programme même (en cours d'exécution) : elle sera alors
	// supprimée à la prochaine mise à jour.
	_ = os.Remove(exe + ".old")
	fmt.Printf("✔ Agent mis à jour : version %s (clé et appairage conservés).\n", offer.Version)
	return nil
}

func setAutoUpdate(value string) error {
	var on bool
	switch strings.ToLower(value) {
	case "on", "oui":
		on = true
	case "off", "non":
	default:
		return fmt.Errorf("--auto : « on » ou « off » attendu (reçu %q)", value)
	}
	path := config.DefaultPath()
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	cfg.AutoUpdate = &on
	if err := config.Save(path, cfg); err != nil {
		return err
	}
	if err := service.Restart(); err != nil {
		fmt.Println("Attention : redémarrez le service pour appliquer ce réglage :", err)
	}
	if on {
		fmt.Println("✔ Recherche quotidienne des mises à jour activée (installation après votre accord).")
	} else {
		fmt.Println("✔ Recherche automatique des mises à jour désactivée (« wol-agent update » reste possible).")
	}
	return nil
}

func confirm(question string) bool {
	fmt.Print(question)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "", "o", "oui", "y", "yes":
		return true
	}
	return false
}

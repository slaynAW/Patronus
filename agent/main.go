// Commande wol-agent : agent installé sur les PC pour les éteindre / redémarrer / mettre en veille
// depuis les applications « Patronus ». Voir agent/README.md.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/slaynaw/wakeonlan/agent/internal/config"
	"github.com/slaynaw/wakeonlan/agent/internal/history"
	"github.com/slaynaw/wakeonlan/agent/internal/netinfo"
	"github.com/slaynaw/wakeonlan/agent/internal/pairing"
	"github.com/slaynaw/wakeonlan/agent/internal/power"
	"github.com/slaynaw/wakeonlan/agent/internal/sensors"
	"github.com/slaynaw/wakeonlan/agent/internal/server"
	"github.com/slaynaw/wakeonlan/agent/internal/service"
	"github.com/slaynaw/wakeonlan/agent/internal/sysinfo"
	"github.com/slaynaw/wakeonlan/agent/internal/terminal"
	"github.com/slaynaw/wakeonlan/agent/protocol"
)

// version est injectée à la compilation (-ldflags "-X main.version=1.0.0").
var version = "dev"

const usageText = `wol-agent %s — agent Patronus (extinction à distance depuis le téléphone ou un PC)

Utilisation : wol-agent <commande> [options]

Commandes :
  install      Installe l'agent comme service (démarrage automatique) et affiche le QR code d'appairage.
               Options : --port 9770, --name "PC Bureau", --ip 192.168.1.20, --no-firewall, --firewall-public
  pair         Réaffiche le QR code / lien d'appairage.  Options : --ip, --png fichier.png, --invert
  rotate-key   Génère une nouvelle clé (l'ancienne ne fonctionne plus : ré-appairez le téléphone).
  status       Affiche l'état du service et la configuration.
  diagnostic   Crée un rapport de diagnostic chiffré par un mot de passe (état, journal), à transmettre
               pour analyser un problème. Options : --out fichier.diag
  update       Recherche une nouvelle version de l'agent et l'installe (Windows ; clé conservée).
               Options : --check (vérifier seulement), --yes (sans confirmation),
               --auto on|off (recherche quotidienne, installée après accord de l'utilisateur connecté)
  uninstall    Désinstalle le service.  Option : --purge (supprime aussi la configuration).
  run          Lance l'agent au premier plan (utilisé par le service).  Options : --config, --dry-run
  version      Affiche la version.

Les commandes install, pair, rotate-key, update et uninstall nécessitent les droits administrateur
(Windows : invite de commandes « Exécuter en tant qu'administrateur » ; Linux/macOS : sudo).
`

func main() {
	args := os.Args[1:]
	// Double-clic sur l'exécutable sous Windows : installation guidée.
	if len(args) == 0 && terminal.LaunchedFromExplorer() {
		args = []string{"install", "--pause"}
	}
	if len(args) == 0 {
		fmt.Printf(usageText, version)
		os.Exit(2)
	}

	var err error
	switch args[0] {
	case "run":
		err = cmdRun(args[1:])
	case "install":
		err = cmdInstall(args[1:])
	case "pair":
		err = cmdPair(args[1:])
	case "rotate-key":
		err = cmdRotateKey(args[1:])
	case "status":
		err = cmdStatus(args[1:])
	case "diagnostic":
		err = cmdDiagnostic(args[1:])
	case "uninstall":
		err = cmdUninstall(args[1:])
	case "update":
		err = cmdUpdate(args[1:])
	case "update-finish": // lancé par le service après le remplacement de l'exécutable
		err = cmdUpdateFinish()
	case "version", "--version", "-v":
		fmt.Println("wol-agent", version)
	case "help", "--help", "-h":
		fmt.Printf(usageText, version)
	default:
		fmt.Fprintf(os.Stderr, "Commande inconnue : %s\n\n", args[0])
		fmt.Printf(usageText, version)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "\nErreur :", err)
		if contains(args, "--pause") {
			terminal.Pause()
		}
		os.Exit(1)
	}
}

func cmdRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	cfgPath := fs.String("config", config.DefaultPath(), "fichier de configuration")
	dryRun := fs.Bool("dry-run", false, "simulation : journalise les actions sans les exécuter")
	_ = fs.Parse(args)

	logger, closeLog := newLogger()
	defer closeLog()

	runServer := func(ctx context.Context) error {
		cfg, err := config.Load(*cfgPath)
		if err != nil {
			logger.Print(err)
			return err
		}
		var controller power.Controller = power.System()
		if *dryRun {
			controller = power.DryRun{Logger: logger}
		}
		srv, err := server.New(cfg, controller, version, logger)
		if err != nil {
			return err
		}
		srv.SetTemperatures(sensors.NewCache(sensors.Read).Get)
		journal := startHistory(*cfgPath, logger)
		if journal != nil {
			srv.SetHistory(journal)
			watch, stopWatch := context.WithCancel(context.Background())
			defer stopWatch()
			watcher := &history.Watcher{
				Log:       journal,
				AlivePath: alivePath(*cfgPath),
				OnError:   func(err error) { logger.Printf("journal : %v", err) },
			}
			go watcher.Run(watch)
		}
		err = srv.ListenAndServe(ctx)
		// Arrêt du service : arrêt du PC, sauf arrêt demandé (mise à jour, désinstallation). Sous
		// Linux / macOS la cause est inconnue : l'arrêt est provisoire et retiré si le PC n'a pas redémarré.
		if journal != nil && ctx.Err() != nil && !errors.Is(context.Cause(ctx), service.ErrStopRequested) {
			if err := journal.Stopping(); err != nil {
				logger.Printf("journal : %v", err)
			}
		}
		return err
	}

	// Lancé par le gestionnaire de services Windows ? Le service recherche aussi les mises à jour.
	asService := func(ctx context.Context) error {
		if !*dryRun {
			startAutoUpdate(ctx, *cfgPath, logger)
			startNetworkAlert(ctx, *cfgPath, logger)
		}
		return runServer(ctx)
	}
	if handled, err := service.Run(asService); handled || err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runServer(ctx)
}

// historyPath et alivePath : journal du PC et dernier signe de vie, à côté de la configuration.
func historyPath(cfgPath string) string { return filepath.Join(filepath.Dir(cfgPath), "history.json") }

func alivePath(cfgPath string) string { return filepath.Join(filepath.Dir(cfgPath), "alive.json") }

// startHistory ouvre le journal et enregistre le démarrage (nil si le journal est indisponible :
// l'agent fonctionne quand même).
func startHistory(cfgPath string, logger *log.Logger) *history.Log {
	journal, err := history.Open(historyPath(cfgPath), time.Now)
	if err != nil {
		logger.Printf("journal indisponible : %v", err)
		return nil
	}
	boot := history.CurrentBoot(sysinfo.Current().Uptime, time.Now())
	if err := journal.Started(boot, history.ReadAlive(alivePath(cfgPath))); err != nil {
		logger.Printf("journal : %v", err)
	}
	return journal
}

func cmdInstall(args []string) error {
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	port := fs.Int("port", config.DefaultPort, "port TCP d'écoute")
	name := fs.String("name", "", "nom du PC affiché dans l'application (nom de la machine par défaut)")
	ip := fs.String("ip", "", "adresse IP à utiliser pour l'appairage (détectée automatiquement)")
	noFirewall := fs.Bool("no-firewall", false, "ne pas modifier le pare-feu")
	firewallPublic := fs.Bool("firewall-public", false, "Windows : autoriser aussi les réseaux « publics » (déconseillé)")
	pause := fs.Bool("pause", false, "attendre une touche à la fin")
	_ = fs.Parse(args)

	if !terminal.IsAdmin() {
		if runtime.GOOS == "windows" {
			fmt.Println("Droits administrateur nécessaires : validez la demande de Windows…")
			return terminal.RelaunchElevated(append([]string{"install"}, withPause(args)...))
		}
		return errors.New("droits administrateur nécessaires : relancez avec « sudo wol-agent install »")
	}
	if *pause {
		defer terminal.Pause()
	}

	cfgPath := config.DefaultPath()
	cfg, err := config.Load(cfgPath)
	switch {
	case err == nil:
		fmt.Println("Configuration existante conservée (la clé ne change pas : pas besoin de ré-appairer).")
		if *name != "" {
			cfg.Name = *name
		}
		if isFlagSet(fs, "port") {
			cfg.Port = *port
		}
	case fileExists(cfgPath):
		return err
	default:
		if cfg, err = config.New(*name, *port); err != nil {
			return err
		}
	}
	if err := config.Save(cfgPath, cfg); err != nil {
		return fmt.Errorf("enregistrement de la configuration : %w", err)
	}

	opts := service.Options{
		Binary:         service.DefaultBinary(),
		Config:         cfgPath,
		Port:           cfg.Port,
		FirewallPublic: *firewallPublic,
		NoFirewall:     *noFirewall,
	}
	fmt.Println("Installation du service…")
	if err := service.Install(opts); err != nil {
		return err
	}
	fmt.Printf("✔ Agent installé (%s) et démarré sur le port TCP %d.\n", opts.Binary, cfg.Port)
	fmt.Printf("  Configuration : %s\n\n", cfgPath)
	if err := showPairing(cfg, *ip, "", false); err != nil {
		return err
	}
	if !*noFirewall {
		warnBlockedNetwork(true)
	}
	return nil
}

func cmdPair(args []string) error {
	fs := flag.NewFlagSet("pair", flag.ExitOnError)
	ip := fs.String("ip", "", "adresse IP à annoncer (détectée automatiquement)")
	pngPath := fs.String("png", "", "enregistre aussi le QR code dans ce fichier PNG")
	invert := fs.Bool("invert", false, "inverse les couleurs (terminal à fond clair sans couleurs)")
	cfgPath := fs.String("config", config.DefaultPath(), "fichier de configuration")
	pause := fs.Bool("pause", false, "attendre une touche à la fin")
	_ = fs.Parse(args)
	if *pause {
		defer terminal.Pause()
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		if !terminal.IsAdmin() {
			return fmt.Errorf("%w\n(la configuration n'est lisible qu'en administrateur)", err)
		}
		return err
	}
	if err := showPairing(cfg, *ip, *pngPath, *invert); err != nil {
		return err
	}
	warnBlockedNetwork(false)
	return nil
}

func cmdRotateKey(args []string) error {
	fs := flag.NewFlagSet("rotate-key", flag.ExitOnError)
	cfgPath := fs.String("config", config.DefaultPath(), "fichier de configuration")
	_ = fs.Parse(args)
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}
	fresh, err := config.New(cfg.Name, cfg.Port)
	if err != nil {
		return err
	}
	cfg.Key = fresh.Key
	if err := config.Save(*cfgPath, cfg); err != nil {
		return err
	}
	if err := service.Restart(); err != nil {
		fmt.Println("Attention : redémarrez le service pour appliquer la nouvelle clé :", err)
	}
	fmt.Println("✔ Nouvelle clé générée. L'ancienne est désormais refusée : ré-appairez le téléphone.")
	return showPairing(cfg, "", "", false)
}

func cmdStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	cfgPath := fs.String("config", config.DefaultPath(), "fichier de configuration")
	_ = fs.Parse(args)
	fmt.Printf("wol-agent %s\nService       : %s\nConfiguration : %s\n", version, service.Status(), *cfgPath)
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Println("                (", err, ")")
	}
	fmt.Printf("Températures  : %s\n", describeTemperatures(sensors.Read()))
	for _, line := range sensors.Details() {
		fmt.Println("                " + line)
	}
	if err != nil {
		return nil
	}
	fmt.Printf("Nom           : %s\nPort          : %d\nCommandes     : %v\nRéseaux       : %v\n", cfg.Name, cfg.Port, cfg.Commands, cfg.Allow)
	if runtime.GOOS == "windows" {
		if cfg.AutoUpdates() {
			fmt.Println("Mises à jour  : recherche quotidienne, installées après accord de l'utilisateur connecté")
		} else {
			fmt.Println("Mises à jour  : recherche automatique désactivée (« wol-agent update --auto on »)")
		}
	}
	if iface, err := netinfo.Detect(""); err == nil {
		fmt.Printf("Carte réseau  : %s — IP %s — MAC %s\n", iface.Name, iface.IP, iface.MAC)
	}
	if runtime.GOOS == "windows" {
		fmt.Printf("Réseaux       : %s\n", describeNetworks())
		warnBlockedNetwork(false)
	}
	if journal, err := history.Open(historyPath(*cfgPath), time.Now); err == nil {
		h := journal.Snapshot()
		fmt.Printf("Journal       : %d évènement(s) depuis le %s\n", len(h.Events), time.Unix(h.From, 0).Format("02/01/2006 15:04"))
		for _, e := range h.Events[max(0, len(h.Events)-5):] {
			fmt.Printf("                %s  %s\n", time.Unix(e.T, 0).Format("02/01 15:04"), describeEvent(e))
		}
	}
	return nil
}

func describeEvent(e protocol.HistoryEvent) string {
	switch e.K {
	case protocol.HistoryBoot:
		return "démarrage"
	case protocol.HistoryShutdown:
		return "arrêt"
	case protocol.HistoryLost:
		return "arrêt non enregistré (coupure ?)"
	case protocol.HistorySleep:
		return "mise en veille"
	case protocol.HistoryResume:
		return "sortie de veille"
	case protocol.HistoryCommand:
		return fmt.Sprintf("%s demandée par %s", power.Action(e.A).Label(), eventClient(e))
	case protocol.HistoryWake:
		return "démarrage demandé par " + eventClient(e)
	}
	return e.K
}

func describeTemperatures(t protocol.Temperatures) string {
	var parts []string
	if t.CPU != nil {
		parts = append(parts, fmt.Sprintf("processeur %.0f °C", *t.CPU))
	} else if t.CPUHint == protocol.CPUHintLHM {
		parts = append(parts, "processeur : "+lhmAdvice(t.LHM))
	}
	if t.GPU != nil {
		gpu := fmt.Sprintf("carte graphique %.0f °C", *t.GPU)
		if t.GPUName != "" {
			gpu += " (" + t.GPUName + ")"
		}
		parts = append(parts, gpu)
	}
	if len(parts) == 0 {
		return "non disponibles sur ce PC"
	}
	return strings.Join(parts, " · ")
}

// lhmAdvice dit quoi faire pour que l'agent lise la température du processeur dans
// LibreHardwareMonitor.
func lhmAdvice(state string) string {
	switch state {
	case protocol.LHMWebOff:
		return "LibreHardwareMonitor tourne, mais son serveur web est désactivé : Options → Remote Web Server → Run"
	case protocol.LHMAuth:
		return "le serveur web de LibreHardwareMonitor demande un mot de passe : désactivez-le (Options → Remote Web Server → Authentication)"
	case protocol.LHMNoSensor:
		return "LibreHardwareMonitor ne la lit pas : installez son pilote PawnIO (proposé au démarrage de LHM 0.9.5 ou plus), ou mettez LHM à jour"
	}
	return "lancez LibreHardwareMonitor en administrateur, avec Options → Remote Web Server → Run"
}

func eventClient(e protocol.HistoryEvent) string {
	if e.B == "" {
		return e.C
	}
	return e.B + " (" + e.C + ")"
}

func cmdUninstall(args []string) error {
	fs := flag.NewFlagSet("uninstall", flag.ExitOnError)
	purge := fs.Bool("purge", false, "supprime aussi la configuration (et donc la clé)")
	_ = fs.Parse(args)
	if !terminal.IsAdmin() {
		return errors.New("droits administrateur nécessaires")
	}
	if err := service.Uninstall(); err != nil {
		return err
	}
	if *purge {
		if err := os.RemoveAll(config.Dir()); err != nil {
			return err
		}
	}
	fmt.Println("✔ Agent désinstallé.")
	if runtime.GOOS != "windows" {
		_ = os.Remove(service.DefaultBinary())
	} else {
		fmt.Println("  Vous pouvez supprimer le dossier", filepath.Dir(service.DefaultBinary()))
	}
	return nil
}

// showPairing affiche le QR code et le lien à scanner / coller dans l'application.
func showPairing(cfg *config.Config, ip, pngPath string, invert bool) error {
	info := pairing.Info{Name: cfg.Name, Port: cfg.Port, Key: cfg.Key}
	iface, err := netinfo.Detect(ip)
	if err != nil {
		if ip == "" {
			return fmt.Errorf("%w (précisez l'adresse avec --ip)", err)
		}
		return err
	}
	info.Host = iface.IP.String()
	info.MAC = iface.MAC.String()
	link := pairing.Link(info)

	fmt.Println("Sur le téléphone : « Ajouter un PC » → « Scanner le QR code » :")
	fmt.Println()
	if err := pairing.WriteTerminalQR(os.Stdout, link, terminal.EnableANSI(), invert); err != nil {
		return err
	}
	fmt.Printf("\nPC : %s — IP %s — MAC %s (carte %s)\n", info.Name, info.Host, info.MAC, iface.Name)
	fmt.Println("⚠ Le Wake-on-LAN passe par la carte réseau FILAIRE : si cette carte est en Wi-Fi,")
	fmt.Println("  corrigez l'adresse MAC dans l'application (ou utilisez --ip avec l'IP de la carte Ethernet).")
	fmt.Println("\nLien d'appairage (application Windows : « Ajouter un PC » → « Coller le lien » ;\nsur le téléphone, « Coller un lien » si le QR code ne passe pas) :")
	fmt.Println(link)
	fmt.Println("\nCe QR code contient la clé secrète de l'agent : ne le partagez pas.")
	if pngPath != "" {
		f, err := os.OpenFile(pngPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return err
		}
		defer f.Close()
		if err := pairing.WritePNG(f, link, 8); err != nil {
			return err
		}
		fmt.Println("QR code enregistré dans", pngPath, "(supprimez-le après usage).")
	}
	return nil
}

// newLogger journalise sur la sortie standard et, sous Windows (service sans console), dans un fichier.
func newLogger() (*log.Logger, func()) {
	if runtime.GOOS != "windows" {
		return log.New(os.Stderr, "", log.LstdFlags), func() {}
	}
	path := filepath.Join(config.Dir(), "agent.log")
	if info, err := os.Stat(path); err == nil && info.Size() > 1<<20 {
		_ = os.Rename(path, path+".old")
	}
	_ = os.MkdirAll(config.Dir(), 0o700)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return log.New(os.Stderr, "", log.LstdFlags), func() {}
	}
	return log.New(io.MultiWriter(os.Stderr, f), "", log.LstdFlags), func() { _ = f.Close() }
}

func withPause(args []string) []string {
	if contains(args, "--pause") {
		return args
	}
	return append(args, "--pause")
}

func isFlagSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func contains(list []string, value string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}

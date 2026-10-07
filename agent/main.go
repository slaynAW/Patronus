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
	"sync"
	"syscall"
	"time"

	"github.com/slaynaw/wakeonlan/agent/internal/config"
	"github.com/slaynaw/wakeonlan/agent/internal/disks"
	"github.com/slaynaw/wakeonlan/agent/internal/history"
	"github.com/slaynaw/wakeonlan/agent/internal/metrics"
	"github.com/slaynaw/wakeonlan/agent/internal/netinfo"
	"github.com/slaynaw/wakeonlan/agent/internal/pairing"
	"github.com/slaynaw/wakeonlan/agent/internal/power"
	"github.com/slaynaw/wakeonlan/agent/internal/sensors"
	"github.com/slaynaw/wakeonlan/agent/internal/server"
	"github.com/slaynaw/wakeonlan/agent/internal/service"
	"github.com/slaynaw/wakeonlan/agent/internal/specs"
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
		if cfg.RecordsMetrics() {
			// Relevés continus : l'état renvoie le dernier, les applications lisent les minutes enregistrées.
			recorder := metrics.New(metricsDir(*cfgPath), sensors.Read, time.Now)
			recorder.OnError = func(err error) { logger.Printf("mesures : %v", err) }
			recording, stopRecording := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() { recorder.Run(recording); close(done) }()
			defer func() { stopRecording(); <-done }()
			srv.SetTemperatures(recorder.Latest)
			srv.SetMetrics(recorder)
		} else {
			srv.SetTemperatures(sensors.NewCache(sensors.Read).Get)
		}
		// Disques : relevés en arrière-plan (espace, santé, erreurs), joints à l'état.
		diskReader := disks.New()
		diskReader.OnError = onceAnHour(logger, "disques")
		diskCtx, stopDisks := context.WithCancel(context.Background())
		defer stopDisks()
		go diskReader.Run(diskCtx)
		srv.SetDisks(diskReader.Read)
		// Fiche du PC : lue une fois au démarrage, puis toutes les quelques heures.
		specsCache := specs.NewCache(specs.Read)
		specsCache.Warm()
		srv.SetSpecs(specsCache.Get)
		journal := startHistory(*cfgPath, logger)
		if journal != nil {
			srv.SetHistory(journal)
			watch, stopWatch := context.WithCancel(context.Background())
			defer stopWatch()
			// Windows écrit le code d'un écran bleu quelques minutes après le démarrage : cause précisée ensuite.
			go func() {
				for _, wait := range []time.Duration{3 * time.Minute, 7 * time.Minute} {
					select {
					case <-watch.Done():
						return
					case <-time.After(wait):
					}
					if changed, err := journal.Reexplain(); err != nil {
						logger.Printf("journal : %v", err)
					} else if changed {
						logger.Print("journal : cause de l'arrêt anormal précisée")
					}
				}
			}()
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

// metricsDir : mesures enregistrées en continu (un fichier par jour), à côté de la configuration.
func metricsDir(cfgPath string) string { return filepath.Join(filepath.Dir(cfgPath), "metrics") }

// startHistory ouvre le journal et enregistre le démarrage (nil si le journal est indisponible :
// l'agent fonctionne quand même).
func startHistory(cfgPath string, logger *log.Logger) *history.Log {
	journal, err := history.Open(historyPath(cfgPath), time.Now)
	if err != nil {
		logger.Printf("journal indisponible : %v", err)
		return nil
	}
	boot := history.CurrentBoot(sysinfo.Current().Uptime, time.Now())
	journal.Explain = history.SystemCause
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
	fmt.Printf("Capteurs      : %s\n", describeTemperatures(sensors.Read()))
	for _, line := range sensors.Details() {
		fmt.Println("                " + line)
	}
	diskInfo, diskProblems := disks.ReadNow()
	for i, line := range append(describeDisks(diskInfo), diskProblems...) {
		if i == 0 {
			fmt.Printf("Disques       : %s\n", line)
		} else {
			fmt.Println("                " + line)
		}
	}
	for i, line := range describeSpecs(specs.Read()) {
		if i == 0 {
			fmt.Printf("Fiche du PC   : %s\n", line)
		} else {
			fmt.Println("                " + line)
		}
	}
	if err != nil {
		return nil
	}
	fmt.Printf("Nom           : %s\nPort          : %d\nCommandes     : %v\nRéseaux       : %v\n", cfg.Name, cfg.Port, cfg.Commands, cfg.Allow)
	fmt.Printf("Mesures       : %s\n", describeMetrics(cfg, *cfgPath))
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
		return "arrêt anormal : " + lostCause(e)
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

// lostCause décrit la cause d'un arrêt non enregistré.
func lostCause(e protocol.HistoryEvent) string {
	switch e.R {
	case protocol.LostBSOD:
		if e.D != "" {
			return "écran bleu (" + e.D + ")"
		}
		return "écran bleu"
	case protocol.LostButton:
		return "arrêt forcé avec le bouton d'alimentation"
	case protocol.LostPower:
		return "coupure de courant ou blocage"
	case protocol.LostHardware:
		return "erreur matérielle fatale (processeur, mémoire ou carte mère)"
	}
	return "non enregistré (coupure ?)"
}

// onceAnHour journalise les erreurs d'un relevé au plus une fois par heure et par type.
func onceAnHour(logger *log.Logger, area string) func(string, error) {
	var mu sync.Mutex
	last := map[string]time.Time{}
	return func(what string, err error) {
		mu.Lock()
		defer mu.Unlock()
		if time.Since(last[what]) < time.Hour {
			return
		}
		last[what] = time.Now()
		logger.Printf("%s : %s : %v", area, what, err)
	}
}

// describeDisks décrit les disques (une ligne par lecteur et par disque).
func describeDisks(d *protocol.Disks) []string {
	if d == nil {
		return []string{"non disponibles sur ce PC"}
	}
	var lines []string
	for _, v := range d.Volumes {
		name := v.Mount
		if v.Label != "" || v.FS != "" {
			name += " (" + strings.Trim(v.Label+", "+v.FS, ", ") + ")"
		}
		used := 0.0
		if v.Total > 0 {
			used = 100 * float64(v.Total-v.Free) / float64(v.Total)
		}
		lines = append(lines, fmt.Sprintf("%s : %s libres sur %s (%.0f %% utilisé)", name, humanBytes(v.Free), humanBytes(v.Total), used))
	}
	for _, dr := range d.Drives {
		kind := strings.TrimSpace(strings.ToUpper(dr.Media) + " " + dr.Bus)
		var parts []string
		if health := map[string]string{protocol.DriveHealthy: "bon état", protocol.DriveWarning: "À SURVEILLER", protocol.DriveUnhealthy: "EN PANNE"}[dr.Health]; health != "" {
			parts = append(parts, health)
		}
		if dr.Temp != nil {
			t := fmt.Sprintf("%.0f °C", *dr.Temp)
			if dr.TempMax != nil {
				t += fmt.Sprintf(" (max %.0f °C)", *dr.TempMax)
			}
			parts = append(parts, t)
		}
		if dr.Wear != nil {
			parts = append(parts, fmt.Sprintf("usure %d %%", *dr.Wear))
		}
		if dr.Hours != nil {
			parts = append(parts, fmt.Sprintf("%d h de fonctionnement", *dr.Hours))
		}
		if dr.ReadErrors != nil || dr.WriteErrors != nil {
			n := int64(0)
			for _, v := range []*int64{dr.ReadErrors, dr.WriteErrors} {
				if v != nil {
					n += *v
				}
			}
			parts = append(parts, fmt.Sprintf("%d erreur(s) non corrigée(s)", n))
		}
		line := fmt.Sprintf("%s [%s]", dr.Name, strings.Trim(kind+", "+humanBytes(dr.Size), ", "))
		if len(parts) > 0 {
			line += " : " + strings.Join(parts, ", ")
		}
		lines = append(lines, line)
	}
	if d.Errors > 0 {
		lines = append(lines, fmt.Sprintf("%d erreur(s) d'accès aux disques signalée(s) par Windows en %d jours, la dernière le %s",
			d.Errors, protocol.DiskErrorDays, time.Unix(d.LastError, 0).Format("02/01/2006 15:04")))
	}
	if len(lines) == 0 {
		return []string{"aucun disque lu"}
	}
	return lines
}

// describeSpecs décrit la fiche du PC (une ligne par élément).
func describeSpecs(s protocol.Specs) []string {
	var lines []string
	if s.Model != "" {
		lines = append(lines, "Modèle : "+s.Model)
	}
	if c := s.CPU; c != nil {
		var parts []string
		if c.Count > 1 {
			parts = append(parts, fmt.Sprintf("%d processeurs", c.Count))
		}
		if c.Cores > 0 {
			parts = append(parts, fmt.Sprintf("%d cœurs", c.Cores))
		}
		if c.Threads > 0 {
			parts = append(parts, fmt.Sprintf("%d threads", c.Threads))
		}
		if c.MHz > 0 {
			parts = append(parts, fmt.Sprintf("%d MHz", c.MHz))
		}
		lines = append(lines, strings.TrimSuffix(fmt.Sprintf("Processeur : %s (%s)", c.Name, strings.Join(parts, ", ")), " ()"))
	}
	if m := s.Memory; m != nil {
		line := "Mémoire : " + humanBytes(m.Total)
		if len(m.Modules) > 0 {
			line += fmt.Sprintf(", %d barrette(s)", len(m.Modules))
			if m.Slots > 0 {
				line += fmt.Sprintf(" sur %d emplacements", m.Slots)
			}
		}
		lines = append(lines, line)
		for _, mod := range m.Modules {
			kind := mod.Type
			if mod.MTs > 0 {
				kind = strings.TrimPrefix(fmt.Sprintf("%s-%d", kind, mod.MTs), "-")
			}
			lines = append(lines, "  "+strings.Join(nonEmpty(mod.Slot, humanBytes(mod.Size), kind, mod.Maker, mod.Part), " · "))
		}
	}
	for _, g := range s.GPUs {
		var parts []string
		if g.Integrated {
			parts = append(parts, "intégrée")
		}
		if g.VRAM > 0 {
			parts = append(parts, humanBytes(g.VRAM))
		}
		if g.Driver != "" {
			parts = append(parts, "pilote "+g.Driver)
		}
		lines = append(lines, strings.TrimSuffix(fmt.Sprintf("Carte graphique : %s (%s)", g.Name, strings.Join(parts, ", ")), " ()"))
	}
	if b := s.Board; b != nil {
		line := "Carte mère : " + strings.Join(nonEmpty(b.Maker, b.Model), " ")
		if b.BIOS != "" {
			line += ", BIOS " + b.BIOS
			if b.BIOSDate != "" {
				line += " du " + b.BIOSDate
			}
		}
		lines = append(lines, line)
	}
	if o := s.OS; o != nil {
		lines = append(lines, strings.TrimSuffix(fmt.Sprintf("Système : %s (%s)", o.Name, o.Version), " ()"))
	}
	if len(lines) == 0 {
		lines = append(lines, "illisible")
	}
	return lines
}

func nonEmpty(values ...string) []string {
	var out []string
	for _, v := range values {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

// humanBytes écrit une taille comme l'Explorateur Windows (« 931 Go », « 1,8 To »).
func humanBytes(n uint64) string {
	units := []string{"o", "Ko", "Mo", "Go", "To", "Po"}
	v := float64(n)
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	if v >= 100 || i == 0 {
		return fmt.Sprintf("%.0f %s", v, units[i])
	}
	return strings.Replace(fmt.Sprintf("%.1f %s", v, units[i]), ".", ",", 1)
}

func describeTemperatures(t protocol.Temperatures) string {
	var parts []string
	cpu := ""
	if t.CPU != nil {
		cpu = fmt.Sprintf("%.0f °C", *t.CPU)
	}
	if t.CPULoad != nil {
		cpu = strings.TrimPrefix(cpu+fmt.Sprintf(", utilisé à %.0f %%", *t.CPULoad), ", ")
	}
	if cpu != "" {
		parts = append(parts, "processeur "+cpu)
	}
	if t.CPU == nil && t.CPUHint == protocol.CPUHintLHM {
		parts = append(parts, "température du processeur : "+lhmAdvice(t.LHM))
	}
	if t.GPU != nil || t.GPULoad != nil {
		var values []string
		if t.GPU != nil {
			values = append(values, fmt.Sprintf("%.0f °C", *t.GPU))
		}
		if t.GPULoad != nil {
			values = append(values, fmt.Sprintf("utilisée à %.0f %%", *t.GPULoad))
		}
		gpu := "carte graphique " + strings.Join(values, ", ")
		if t.GPUName != "" {
			gpu += " (" + t.GPUName + ")"
		}
		if t.GPUShared {
			gpu += ", intégrée au processeur : température de la puce"
		}
		parts = append(parts, gpu)
	}
	if len(parts) == 0 {
		return "non disponibles sur ce PC"
	}
	return strings.Join(parts, " · ")
}

// describeMetrics décrit l'enregistrement continu des mesures.
func describeMetrics(cfg *config.Config, cfgPath string) string {
	if !cfg.RecordsMetrics() {
		return "enregistrement désactivé (« metrics »: false dans config.json)"
	}
	days := metrics.New(metricsDir(cfgPath), nil, time.Now).Days()
	if len(days) == 0 {
		return fmt.Sprintf("enregistrées chaque minute, gardées %d jours (aucune pour l'instant)", protocol.MetricsDays)
	}
	return fmt.Sprintf("enregistrées chaque minute, gardées %d jours : %d jour(s), du %s au %s (UTC)",
		protocol.MetricsDays, len(days), days[0], days[len(days)-1])
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

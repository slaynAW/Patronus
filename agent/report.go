package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/slaynaw/wakeonlan/agent/diagnostic"
	"github.com/slaynaw/wakeonlan/agent/internal/config"
	"github.com/slaynaw/wakeonlan/agent/internal/history"
	"github.com/slaynaw/wakeonlan/agent/internal/netinfo"
	"github.com/slaynaw/wakeonlan/agent/internal/sensors"
	"github.com/slaynaw/wakeonlan/agent/internal/service"
	"github.com/slaynaw/wakeonlan/agent/internal/terminal"
)

// logTail : fin du journal technique jointe au rapport (octets, par fichier).
const logTail = 256 << 10

// cmdDiagnostic crée un rapport de diagnostic chiffré par un mot de passe (docs/DIAGNOSTIC.md) : état
// du service, configuration (sans la clé), carte réseau, températures, journal des démarrages et
// arrêts, fin du journal technique. Le mot de passe est demandé deux fois sans être affiché, ou lu
// dans la variable PATRONUS_DIAG_PASSWORD.
func cmdDiagnostic(args []string) error {
	fs := flag.NewFlagSet("diagnostic", flag.ExitOnError)
	cfgPath := fs.String("config", config.DefaultPath(), "fichier de configuration")
	out := fs.String("out", "", "fichier du rapport (par défaut : sur le Bureau, sinon dans le dossier courant)")
	_ = fs.Parse(args)

	password := os.Getenv("PATRONUS_DIAG_PASSWORD")
	if password == "" {
		fmt.Printf("Le rapport sera chiffré par un mot de passe (%d caractères au moins), à transmettre séparément.\n", diagnostic.MinPasswordLength)
		var err error
		if password, err = terminal.ReadPassword("Mot de passe : "); err != nil {
			return err
		}
		confirmation, err := terminal.ReadPassword("Confirmation : ")
		if err != nil {
			return err
		}
		if confirmation != password {
			return errors.New("les deux mots de passe sont différents")
		}
	}
	if utf8.RuneCountInString(password) < diagnostic.MinPasswordLength {
		return fmt.Errorf("mot de passe trop court (%d caractères au moins)", diagnostic.MinPasswordLength)
	}

	now := time.Now()
	sealed, err := diagnostic.Seal(agentReport(*cfgPath, now), password, "wol-agent "+version, now.UTC().Format(time.RFC3339))
	if err != nil {
		return err
	}
	path := *out
	if path == "" {
		path = filepath.Join(reportDir(), "patronus-diagnostic-agent-"+now.Format("2006-01-02")+".diag")
	}
	if err := os.WriteFile(path, sealed, 0o600); err != nil {
		return err
	}
	fmt.Println("Rapport chiffré enregistré :", path)
	fmt.Println("Transmettez ce fichier et, séparément, le mot de passe.")
	return nil
}

// reportDir renvoie le Bureau de l'utilisateur s'il existe, sinon le dossier courant.
func reportDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		desktop := filepath.Join(home, "Desktop")
		if info, err := os.Stat(desktop); err == nil && info.IsDir() {
			return desktop
		}
	}
	return "."
}

// agentReport rédige le rapport (texte). Jamais la clé de l'agent : seulement sa présence.
func agentReport(cfgPath string, now time.Time) string {
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	section := func(title string) { line("\n=== %s ===", title) }

	line("=== Patronus — rapport de diagnostic de l'agent ===")
	line("Agent : wol-agent %s (%s/%s, %s)", version, runtime.GOOS, runtime.GOARCH, runtime.Version())
	host, _ := os.Hostname()
	line("PC : %s", host)
	line("Créé le : %s (heure locale %s)", now.UTC().Format(time.RFC3339), now.Format("2006-01-02 15:04:05 -07:00"))
	line("Droits administrateur : %s", pick(terminal.IsAdmin(), "oui", "non (configuration et journal peut-être illisibles)"))

	section("Service et configuration")
	line("Service : %s", service.Status())
	line("Configuration : %s", cfgPath)
	if cfg, err := config.Load(cfgPath); err != nil {
		line("Configuration illisible : %v", err)
	} else {
		line("Nom « %s », port %d, clé %s, commandes %v, réseaux autorisés %v, mises à jour automatiques %t",
			cfg.Name, cfg.Port, pick(cfg.Key != "", "présente", "absente"), cfg.Commands, cfg.Allow, cfg.AutoUpdates())
	}
	if iface, err := netinfo.Detect(""); err == nil {
		line("Carte réseau : %s, IP %s, MAC %s", iface.Name, iface.IP, iface.MAC)
	} else {
		line("Carte réseau : %v", err)
	}
	line("Températures : %s", describeTemperatures(sensors.Read()))

	section("Fichiers de l'agent")
	dir := filepath.Dir(cfgPath)
	if entries, err := os.ReadDir(dir); err != nil {
		line("Dossier %s illisible : %v", dir, err)
	} else {
		for _, e := range entries {
			if info, err := e.Info(); err == nil {
				line("- %s : %d octets, modifié le %s", e.Name(), info.Size(), info.ModTime().UTC().Format(time.RFC3339))
			}
		}
	}

	section("Journal des démarrages, arrêts et commandes")
	if journal, err := history.Open(historyPath(cfgPath), time.Now); err != nil {
		line("Illisible : %v", err)
	} else {
		h := journal.Snapshot()
		line("%d évènement(s) depuis le %s", len(h.Events), time.Unix(h.From, 0).UTC().Format(time.RFC3339))
		for _, e := range h.Events {
			line("%s  %s", time.Unix(e.T, 0).UTC().Format(time.RFC3339), describeEvent(e))
		}
	}

	for _, path := range logFiles() {
		data, err := readTail(path, logTail)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		section("Journal technique : " + path)
		if err != nil {
			line("Illisible : %v", err)
			continue
		}
		b.Write(data)
		if len(data) > 0 && data[len(data)-1] != '\n' {
			b.WriteByte('\n')
		}
	}
	if runtime.GOOS == "linux" {
		section("Journal technique : journalctl -u wol-agent")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		out, err := exec.CommandContext(ctx, "journalctl", "-u", "wol-agent", "-n", "1000", "--no-pager", "-o", "short-iso").CombinedOutput()
		cancel()
		if err != nil {
			line("Illisible : %v", err)
		}
		b.Write(bytes.ToValidUTF8(out, nil))
	}
	return b.String()
}

// logFiles renvoie les fichiers du journal technique de l'agent (Linux : journal de systemd).
func logFiles() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{filepath.Join(config.Dir(), "agent.log.old"), filepath.Join(config.Dir(), "agent.log")}
	case "darwin":
		return []string{"/Library/Logs/wol-agent.log"}
	}
	return nil
}

// readTail lit les limit derniers octets d'un fichier, à partir d'un début de ligne.
func readTail(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	start := max(0, info.Size()-limit)
	data := make([]byte, info.Size()-start)
	if _, err := f.ReadAt(data, start); err != nil {
		return nil, err
	}
	if start > 0 {
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			data = data[i+1:]
		}
	}
	return bytes.ToValidUTF8(data, nil), nil
}

func pick(cond bool, yes, no string) string {
	if cond {
		return yes
	}
	return no
}

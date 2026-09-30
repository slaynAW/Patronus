package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/slaynaw/wakeonlan/agent/diagnostic"
	"github.com/slaynaw/wakeonlan/desktop/internal/diag"
	"github.com/slaynaw/wakeonlan/desktop/internal/model"
	"github.com/slaynaw/wakeonlan/desktop/internal/netstate"
	"github.com/slaynaw/wakeonlan/desktop/internal/share"
	"github.com/slaynaw/wakeonlan/desktop/internal/status"
)

// Journal et rapport de diagnostic (docs/DIAGNOSTIC.md). Le journal (paquet diag) note les actions,
// les changements d'état et les erreurs ; le rapport y ajoute l'état complet de l'application, puis
// est chiffré par un mot de passe choisi par l'utilisateur. Jamais de secret : ni clé d'agent, ni mot
// de passe SecureOn, ni jeton GitHub, ni clé privée (seulement leur présence, ou l'empreinte courte
// d'une clé publique).

// Domaines des évènements du journal.
const (
	areaApp     = "appli"
	areaUI      = "interface"
	areaData    = "données"
	areaNetwork = "réseau"
	areaActions = "actions"
	areaStatus  = "état"
	areaBackup  = "sauvegarde"
	areaShare   = "partage"
	areaUpdate  = "mise-à-jour"
	areaAgent   = "agent"
	areaHistory = "historique"
)

// quietCalls : appels fréquents ou sans effet, notés seulement en cas d'erreur.
var quietCalls = map[string]bool{
	"getState": true, "uiReady": true, "setVisible": true, "setLive": true, "getHistory": true,
	"getDevice": true, "readClipboard": true, "copyText": true, "parsePairing": true, "logClient": true,
	"shareRequestView": true, "refresh": true, "clearNotice": true, "dismissStartupMessage": true,
}

func pick(cond bool, yes, no string) string {
	if cond {
		return yes
	}
	return no
}

func joinErrors(errs []string) string {
	if len(errs) == 0 {
		return ""
	}
	return " ; erreurs : " + strings.Join(errs, " ; ")
}

// keyRef renvoie l'empreinte courte d'une clé publique de partage (« k: » et le début de son
// identifiant, comme l'application Android et les noms des fichiers d'accès du Gist).
func keyRef(public string) string {
	id := share.KeyID(public)
	if len(id) < 8 {
		return "-"
	}
	return "k:" + id[:8]
}

func shortID(id string) string {
	if len(id) > 10 {
		return id[:10]
	}
	return id
}

func clip(text string, limit int) string {
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	return string([]rune(text)[:limit]) + "…"
}

func millisText(ms int64) string {
	if ms <= 0 {
		return "-"
	}
	return time.UnixMilli(ms).UTC().Format(time.RFC3339)
}

func timeText(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.UTC().Format(time.RFC3339)
}

func describeNetwork(st netstate.State) string {
	if !st.Connected() {
		return "non connecté" + pick(st.VPNActive, ", VPN actif", "")
	}
	var parts []string
	for _, i := range st.Interfaces {
		var addrs []string
		for _, a := range i.Addresses {
			addrs = append(addrs, a.String())
		}
		parts = append(parts, fmt.Sprintf("%s « %s » %s%s", i.Transport, i.Name, strings.Join(addrs, " "), pick(i.HasGateway, "", " (sans passerelle)")))
	}
	return "connecté : " + strings.Join(parts, " ; ") + pick(st.VPNActive, ", VPN actif", "")
}

// describe résume les paramètres d'un appel pour le journal (jamais de mot de passe, de clé ni de
// texte collé). name donne le nom d'un PC (nil : identifiant seul).
func (p callParams) describe(method string, name func(id string) string) string {
	var parts []string
	add := func(format string, args ...any) { parts = append(parts, fmt.Sprintf(format, args...)) }
	if p.ID != "" {
		if n := nameOf(name, p.ID); n != "" {
			add("PC « %s » [%s]", n, shortID(p.ID))
		} else {
			add("PC [%s]", shortID(p.ID))
		}
	}
	switch method {
	case "moveDevice":
		add("décalage %d", p.Offset)
	case "setAutoUpdate":
		add("%s", pick(p.Enabled, "activées", "désactivées"))
	case "exportConfig":
		add("%s", pick(p.WithSecret, "complète", "sans secret"))
	case "importConfirm":
		add("%s", pick(p.Replace, "remplacer", "ajouter"))
	case "updateSettings":
		if p.Poll != nil {
			add("vérification %d s", *p.Poll)
		}
		if p.WakeTO != nil {
			add("attente %d s", *p.WakeTO)
		}
		if p.Confirm != nil {
			add("confirmation %t", *p.Confirm)
		}
	case "shareLogin", "shareRequest", "shareGrant":
		if p.Name != "" {
			add("nom « %s »", clip(p.Name, 64))
		}
	}
	if p.Action != "" {
		add("action %s", p.Action)
	}
	if p.Force {
		add("forcé")
	}
	if p.Platform != "" {
		add("plateforme %s", p.Platform)
	}
	if p.Install {
		add("installation")
	}
	if p.Device != "" {
		add("appareil %s", keyRef(p.Device))
	}
	if p.Owner != "" {
		add("partage de %s", keyRef(p.Owner))
	}
	if len(p.Rights) > 0 {
		add("%d droit(s)", len(p.Rights))
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, ", ") + ")"
}

func nameOf(name func(string) string, id string) string {
	if name == nil {
		return ""
	}
	return name(id)
}

// brief résume un appel sans prendre de verrou (après une panique).
func (p callParams) brief(method string) string { return p.describe(method, nil) }

// callLabel résume un appel avec le nom du PC concerné.
func (s *Service) callLabel(method string, p callParams) string {
	return p.describe(method, func(id string) string {
		d, ok := s.device(id)
		if !ok {
			return ""
		}
		return d.Name
	})
}

// locksFree vérifie qu'aucun verrou n'est resté pris après une panique (l'application se figerait).
// Un verrou pris normalement par une autre tâche est libéré en quelques millisecondes.
func (s *Service) locksFree() bool {
	locks := []*sync.Mutex{&s.mu, &s.histMu, &s.statusMu, &s.updates.mu, &s.agents.mu}
	if s.sharing != nil {
		locks = append(locks, &s.sharing.mu)
	}
	if s.backups != nil {
		locks = append(locks, &s.backups.mu)
	}
	deadline := time.Now().Add(2 * time.Second)
	for _, l := range locks {
		for !l.TryLock() {
			if time.Now().After(deadline) {
				return false
			}
			time.Sleep(10 * time.Millisecond)
		}
		l.Unlock()
	}
	return true
}

// --- Erreurs de l'interface ---

// clientLogLimit borne le nombre d'évènements envoyés par l'interface (erreur répétée en boucle).
type clientLogLimit struct {
	mu     sync.Mutex
	window time.Time
	count  int
}

const (
	clientLogMax    = 30
	clientLogWindow = 10 * time.Minute
)

// logClient note une erreur (ou un évènement) de l'interface.
func (s *Service) logClient(level, text string) {
	l := &s.clientLog
	l.mu.Lock()
	now := s.now()
	if now.Sub(l.window) > clientLogWindow {
		l.window, l.count = now, 0
	}
	l.count++
	count := l.count
	l.mu.Unlock()
	switch {
	case count == clientLogMax+1:
		diag.Warn(areaUI, "trop d'évènements de l'interface : les suivants sont ignorés pendant %s", clientLogWindow)
		return
	case count > clientLogMax:
		return
	}
	text = clip(text, 8000)
	switch level {
	case "error":
		diag.Error(areaUI, "page : %s", text)
	case "warn":
		diag.Warn(areaUI, "page : %s", text)
	default:
		diag.Info(areaUI, "page : %s", text)
	}
}

// --- Rapport ---

func (s *Service) appName() string { return "Patronus Windows " + s.version }

// exportDiagnostic crée le rapport, le chiffre avec password et propose de l'enregistrer.
func (s *Service) exportDiagnostic(password string) (any, error) {
	if utf8.RuneCountInString(password) < diagnostic.MinPasswordLength {
		return nil, fmt.Errorf("Mot de passe trop court (%d caractères au moins)", diagnostic.MinPasswordLength)
	}
	now := s.now()
	diag.Info(areaApp, "création d'un rapport de diagnostic")
	sealed, err := diagnostic.Seal(s.diagnosticReport(now), password, s.appName(), now.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	name := "patronus-diagnostic-" + now.Format("2006-01-02") + ".diag"
	path, err := s.platform.SaveFile(name, sealed)
	if errors.Is(err, ErrCancelled) {
		return map[string]any{"cancelled": true}, nil
	}
	if err != nil {
		return nil, err
	}
	result := map[string]any{"ok": true, "path": path, "name": name}
	if path == "" {
		result["download"] = map[string]any{"name": name, "content": string(sealed)}
	}
	return result, nil
}

// diagnosticReport rédige le rapport (texte).
func (s *Service) diagnosticReport(now time.Time) string {
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	section := func(title string) { line("\n=== %s ===", title) }

	line("=== Patronus — rapport de diagnostic ===")
	line("Application : %s", s.appName())
	line("Système : %s (%s/%s, %s)", pick(s.system != "", s.system, "inconnu"), runtime.GOOS, runtime.GOARCH, runtime.Version())
	line("Créé le : %s (heure locale %s)", now.UTC().Format(time.RFC3339), now.Format("2006-01-02 15:04:05 -07:00"))

	s.mu.Lock()
	cfg := s.cfg.Clone()
	network := s.network
	startup := s.startupMessage
	windowActive, pageVisible := s.windowActive, s.pageVisible
	s.mu.Unlock()

	section("Réseau")
	line("Réseau local : %s", describeNetwork(network))
	line("Fenêtre active : %s, page visible : %s", pick(windowActive, "oui", "non"), pick(pageVisible, "oui", "non"))
	if startup != "" {
		line("Message affiché au démarrage : %s", startup)
	}

	section("PC")
	statuses := s.monitor.Statuses()
	line("Réglages : vérification %d s, attente du démarrage %d s, confirmation %t",
		cfg.Settings.PollIntervalSeconds, cfg.Settings.WakeTimeoutSeconds, cfg.Settings.ConfirmPowerActions)
	line("%d PC de cet appareil :", len(cfg.Devices))
	for _, d := range cfg.Devices {
		st, ok := statuses[d.ID]
		b.WriteString(describeDevice(d, st, ok, ""))
	}
	if shared := s.sharedDevices(); len(shared) > 0 {
		line("%d PC reçus d'autres personnes :", len(shared))
		for _, sd := range shared {
			st, ok := statuses[sd.device.ID]
			b.WriteString(describeDevice(sd.device, st, ok, fmt.Sprintf(" partagé par « %s » %s,", sd.ownerName, keyRef(sd.owner))))
		}
	}

	section("Partage")
	s.describeSharing(line, cfg)

	section("Sauvegardes automatiques")
	s.describeBackups(line)

	section("Mises à jour")
	u := s.updates.snapshot()
	line("Disponibles %t, automatiques %t, dernière recherche %s, proposée %s, étape %s%s",
		u.Enabled, u.Auto, millisText(u.LastCheck), availableVersion(u), pick(u.Stage != "", u.Stage, "-"), errorSuffix(u.Error))
	a := s.agents.snapshot()
	line("Téléchargement de l'agent : disponible %t, dernière version connue %s, en cours %t",
		a.Enabled, pick(a.Latest != "", a.Latest, "-"), a.Busy)

	section("Historique")
	s.histMu.Lock()
	line("%d évènements enregistrés", len(s.hist.Events))
	ids := make([]string, 0, len(s.agentFetch))
	for id := range s.agentFetch {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		st := s.agentFetch[id]
		line("- journal de l'agent [%s] : %s, lu %s%s", shortID(id), pick(st.status != "", st.status, "jamais lu"),
			timeText(st.last), pick(st.wakesUnsupported, ", agent trop ancien pour les démarrages signalés", ""))
	}
	s.histMu.Unlock()

	section("Fichiers de l'application")
	dataDir := filepath.Dir(s.store.Path())
	line("Dossier : %s", dataDir)
	listFiles(line, dataDir, "")
	listFiles(line, filepath.Join(dataDir, "diagnostics"), "diagnostics/")

	records := diag.Records()
	section(fmt.Sprintf("Journal (%d évènements, du plus ancien au plus récent)", len(records)))
	for _, r := range records {
		line("%s", r)
	}
	return b.String()
}

func availableVersion(u UpdateView) string {
	if u.Available == nil {
		return "-"
	}
	return u.Available.Version
}

func errorSuffix(err string) string {
	if err == "" {
		return ""
	}
	return ", erreur : " + err
}

func listFiles(line func(string, ...any), dir, prefix string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		line("- %s : illisible (%v)", pick(prefix != "", prefix, "dossier"), err)
		return
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		if e.IsDir() {
			line("- %s%s/ (dossier)", prefix, e.Name())
			continue
		}
		mark := ""
		if strings.Contains(e.Name(), ".illisible-") {
			mark = " ← fichier illisible mis de côté"
		}
		line("- %s%s : %d octets, modifié le %s%s", prefix, e.Name(), info.Size(), timeText(info.ModTime()), mark)
	}
}

func describeDevice(d model.Device, st status.DeviceStatus, known bool, origin string) string {
	var b strings.Builder
	agent := "non"
	if d.Agent != nil {
		agent = fmt.Sprintf("port %d, clé %s", d.Agent.Port, pick(d.Agent.HasKey(), "présente", "absente"))
	}
	broadcast := "auto"
	if d.BroadcastAddress != nil {
		broadcast = *d.BroadcastAddress
	}
	fmt.Fprintf(&b, "- « %s » [%s]%s MAC %s, hôte %s, diffusion %s, port WoL %d, SecureOn %s, sondes %v, agent %s\n",
		d.Name, shortID(d.ID), origin, d.MAC, pick(d.Host != "", d.Host, "-"), broadcast, d.WolPort,
		pick(d.SecureOnPassword != nil, "oui", "non"), d.ProbePorts, agent)
	if !known {
		return b.String()
	}
	fmt.Fprintf(&b, "    état %s depuis %s", st.State, millisText(st.Since))
	if st.LastSeen != nil {
		fmt.Fprintf(&b, ", vu %s", millisText(*st.LastSeen))
	}
	if st.Method != "" {
		fmt.Fprintf(&b, ", via %s", st.Method)
	}
	if st.LatencyMs != nil {
		fmt.Fprintf(&b, ", latence %d ms", *st.LatencyMs)
	}
	if st.Agent != nil {
		fmt.Fprintf(&b, ", agent %s (%s/%s, %s)", st.Agent.Version, st.Agent.OS, st.Agent.Arch, st.Agent.Hostname)
	}
	if st.AgentError != "" {
		fmt.Fprintf(&b, ", erreur de l'agent %s", st.AgentError)
	}
	if st.UnknownReason != "" {
		fmt.Fprintf(&b, ", raison %s", st.UnknownReason)
	}
	if st.Notice != "" {
		fmt.Fprintf(&b, ", avis %s", st.Notice)
	}
	b.WriteString("\n")
	return b.String()
}

// describeSharing décrit l'état du partage (verrou sh.mu pris ici, jamais avec s.mu).
func (s *Service) describeSharing(line func(string, ...any), cfg model.AppConfig) {
	sh := s.sharing
	if sh == nil {
		line("Partage indisponible (désactivé dans cette version).")
		return
	}
	names := map[string]string{}
	for _, d := range cfg.Devices {
		names[d.ID] = d.Name
	}
	sh.mu.Lock()
	defer sh.mu.Unlock()
	line("Connexion GitHub possible : %s ; état du partage lu au démarrage : %s", pick(sh.gh.ClientID != "", "oui", "non"),
		pick(sh.loadErr == "", "oui", "NON, "+sh.loadErr))
	if l := sh.login; l != nil {
		line("Connexion GitHub en cours (code affiché)%s", errorSuffix(l.err))
	}
	if o := sh.state.Owner; o == nil {
		line("Je partage mes PC : non (aucun partage enregistré)")
	} else {
		pub := "illisible"
		if key, err := share.ParseOwnerPrivate(o.Key); err == nil {
			pub = keyRef(share.OwnerPublic(key))
		} else {
			pub += " (" + err.Error() + ")"
		}
		line("Je partage mes PC : oui, nom « %s », clé %s, compte @%s, gist %s, jeton %s, révision %d",
			o.Name, pub, pick(o.User != "", o.User, "-"), pick(o.Gist != "", shortID(o.Gist), "-"), pick(o.Token != "", "présent", "absent"), o.Revision)
		line("Publication : en cours %t, à refaire %t, dernière tentative %s%s", sh.publishing, sh.dirty, timeText(sh.lastPublish), errorSuffix(sh.publishErr))
		line("%d personne(s) autorisée(s) :", len(o.People))
		for _, p := range o.People {
			var rights []string
			for id, r := range p.Rights {
				n, ok := names[id]
				if !ok {
					n = "PC inconnu [" + shortID(id) + "]"
				}
				rights = append(rights, n+" = "+string(r))
			}
			slices.Sort(rights)
			_, published := o.Published[p.Device]
			line("- « %s » %s, ajoutée %s, publiée %s : %s", p.Name, keyRef(p.Device), millisText(p.Added),
				pick(published, "oui", "non"), strings.Join(rights, ", "))
		}
		if len(o.Withdrawn) > 0 {
			line("Accès retirés, fichiers à supprimer du Gist : %d", len(o.Withdrawn))
		}
	}
	if sh.state.DeviceKey != "" {
		if key, err := share.ParseDevicePrivate(sh.state.DeviceKey); err == nil {
			line("Clé de réception de cet appareil : %s", keyRef(share.DevicePublic(key)))
		} else {
			line("Clé de réception de cet appareil : illisible (%v)", err)
		}
	}
	line("PC partagés avec moi : %d", len(sh.state.Received))
	for _, a := range sh.state.Received {
		line("- de « %s » %s (@%s), mon nom « %s », actif %t, retiré %t, %d PC, révision %d, demandé %s, reçu %s, vérifié %s%s",
			a.OwnerName, keyRef(a.Owner), a.User, a.MyName, a.Active, a.Removed, len(a.Devices), a.Revision,
			millisText(a.Requested), millisText(a.Synced), timeText(sh.lastSync[a.Owner]), errorSuffix(sh.syncErr[a.Owner]))
	}
}

// summary résume le partage pour le journal de démarrage.
func (sh *sharer) summary() string {
	if sh == nil {
		return "indisponible"
	}
	sh.mu.Lock()
	defer sh.mu.Unlock()
	mine := "non activé"
	if o := sh.state.Owner; o != nil {
		mine = fmt.Sprintf("actif (%d personne(s), compte @%s, jeton %s)", len(o.People), o.User, pick(o.Token != "", "présent", "absent"))
	}
	return fmt.Sprintf("%s, %d partage(s) reçu(s)%s", mine, len(sh.state.Received), pick(sh.loadErr != "", ", ÉTAT ILLISIBLE : "+sh.loadErr, ""))
}

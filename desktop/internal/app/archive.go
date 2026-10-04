package app

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol"
	"github.com/slaynaw/wakeonlan/desktop/internal/agentclient"
	"github.com/slaynaw/wakeonlan/desktop/internal/archive"
	"github.com/slaynaw/wakeonlan/desktop/internal/backup"
	"github.com/slaynaw/wakeonlan/desktop/internal/diag"
	"github.com/slaynaw/wakeonlan/desktop/internal/history"
	"github.com/slaynaw/wakeonlan/desktop/internal/model"
	"github.com/slaynaw/wakeonlan/desktop/internal/share"
)

// Archives chiffrées (docs/ARCHIVES.md) : l'agent de chaque PC enregistre en continu ses mesures
// (températures, utilisation, une ligne par minute, 90 jours). L'application les rattrape et les
// range sur GitHub avec le journal des démarrages et arrêts, dans un Gist secret par mois chiffré par
// le mot de passe des sauvegardes (même compte GitHub que les sauvegardes). L'écran « Mesures » lit
// l'agent (90 derniers jours) et, au-delà ou PC éteint, les archives.

const (
	archiveEvery   = 30 * time.Minute
	archiveStart   = 2 * time.Minute
	archiveRetry   = 10 * time.Minute
	archiveTick    = 30 * time.Second
	archiveTimeout = 10 * time.Minute
	// archiveAgentTimeout : délai par échange avec un agent (une journée de mesures au plus).
	archiveAgentTimeout = 20 * time.Second
	viewTimeout         = 2 * time.Minute
	monthsTTL           = 10 * time.Minute
	gistTTL             = 10 * time.Minute
	agentDaysTTL        = time.Minute
	pastDayTTL          = 10 * time.Minute
	todayTTL            = 30 * time.Second
	// maxRangeDays borne une demande de l'écran « Mesures ».
	maxRangeDays   = 93
	maxChartPoints = 1500
)

type cachedDays struct {
	days []string
	err  error
	at   time.Time
}

type cachedRows struct {
	rows []protocol.MetricsRow
	at   time.Time
}

type cachedGist struct {
	files map[string]string
	at    time.Time
}

type cachedJournal struct {
	events []protocol.HistoryEvent
	at     time.Time
}

type archiver struct {
	wake chan struct{}

	mu      sync.Mutex
	running bool
	attempt time.Time
	failed  bool
	err     string
	// keys : clé de chaque Gist d'archives, dérivée une fois par session (PBKDF2 coûteux).
	keys map[string]*archive.Key
	// months : Gists d'archives du compte par mois (lus au plus toutes les monthsTTL).
	months   map[string][]string
	monthsAt time.Time
	gists    map[string]cachedGist
	days     map[string]cachedDays
	rows     map[string]cachedRows
	journals map[string]cachedJournal
}

func newArchiver() *archiver {
	a := &archiver{wake: make(chan struct{}, 1)}
	a.reset()
	return a
}

// reset oublie les clés et les données lues (mot de passe ou compte changé).
func (a *archiver) reset() {
	a.keys = map[string]*archive.Key{}
	a.months, a.monthsAt = nil, time.Time{}
	a.gists = map[string]cachedGist{}
	a.days = map[string]cachedDays{}
	a.rows = map[string]cachedRows{}
	a.journals = map[string]cachedJournal{}
}

func (a *archiver) poke() {
	select {
	case a.wake <- struct{}{}:
	default:
	}
}

// forgetArchiveKeys est appelé quand le mot de passe ou le compte des sauvegardes change.
func (s *Service) forgetArchiveKeys() {
	if a := s.archives; a != nil {
		a.mu.Lock()
		a.reset()
		a.mu.Unlock()
	}
}

// archivesRunning indique un archivage en cours.
func (s *Service) archivesRunning() bool {
	a := s.archives
	if a == nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.running
}

// archiveAccess : ce qu'il faut pour lire ou écrire les archives (vide si indisponible).
type archiveAccess struct {
	enabled  bool
	password string
	token    string
	synced   map[string]int64
	gists    map[string]string
	last     int64
}

func (s *Service) archiveAccess() archiveAccess {
	b := s.backups
	if b == nil {
		return archiveAccess{}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	acc := archiveAccess{password: b.st.Password, synced: map[string]int64{}, gists: map[string]string{}}
	if !b.st.Enabled {
		acc.password = ""
	}
	if b.st.GitHub != nil {
		acc.token = b.st.GitHub.Token
	}
	if ar := b.st.Archive; ar != nil {
		acc.enabled = ar.Enabled
		maps.Copy(acc.synced, ar.Synced)
		maps.Copy(acc.gists, ar.Gists)
		acc.last = ar.Last
	}
	return acc
}

func (acc archiveAccess) usable() bool { return acc.password != "" && acc.token != "" }

// runArchives archive automatiquement jusqu'à l'annulation de ctx.
func (s *Service) runArchives(ctx context.Context) {
	a := s.archives
	if a == nil || s.backups == nil {
		return
	}
	timer := time.NewTimer(archiveStart)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		case <-a.wake:
		}
		if s.archiveDue() {
			_, _ = s.archiveNow(ctx)
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(archiveTick)
	}
}

func (s *Service) archiveDue() bool {
	acc := s.archiveAccess()
	if !acc.enabled || !acc.usable() || s.rotationPending() {
		return false
	}
	a := s.archives
	now := s.now()
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.running || (a.failed && now.Sub(a.attempt) < archiveRetry) {
		return false
	}
	return now.Sub(time.UnixMilli(acc.last)) >= archiveEvery
}

// ArchiveResult est le résultat d'un archivage.
type ArchiveResult struct {
	PCs     int      `json:"pcs"`
	Minutes int      `json:"minutes"`
	Files   int      `json:"files"`
	Skipped []string `json:"skipped,omitempty"`
}

// pcWork : mesures et journal d'un PC à archiver.
type pcWork struct {
	name   string
	rows   map[string][]protocol.MetricsRow // jour → minutes
	events []protocol.HistoryEvent
	last   int64
}

// archiveNow rattrape les mesures et le journal des PC joignables et les range dans les Gists.
func (s *Service) archiveNow(ctx context.Context) (ArchiveResult, error) {
	a := s.archives
	if a == nil || s.backups == nil {
		return ArchiveResult{}, errors.New("archives indisponibles")
	}
	acc := s.archiveAccess()
	switch {
	case !acc.enabled:
		return ArchiveResult{}, errors.New("archivage désactivé")
	case acc.password == "":
		return ArchiveResult{}, errors.New("activez d'abord la sauvegarde automatique (mot de passe des archives)")
	case acc.token == "":
		return ArchiveResult{}, errors.New("connectez d'abord GitHub dans la sauvegarde automatique")
	}
	now := s.now()
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return ArchiveResult{}, errors.New("archivage déjà en cours")
	}
	a.running, a.attempt = true, now
	a.mu.Unlock()
	// Changement du mot de passe en cours (vérifié après avoir pris la main : l'un attend l'autre).
	if s.rotationPending() {
		a.mu.Lock()
		a.running = false
		a.mu.Unlock()
		return ArchiveResult{}, errors.New("changement du mot de passe des sauvegardes en cours : l'archivage suivra")
	}
	s.notify()
	ctx, cancel := context.WithTimeout(ctx, archiveTimeout)
	defer cancel()
	if err := s.checkBackupPassword(ctx); errors.Is(err, errStalePassword) {
		a.mu.Lock()
		a.running, a.failed, a.err = false, true, err.Error()
		a.mu.Unlock()
		s.notify()
		return ArchiveResult{}, err
	}
	result, synced, err := s.archiveCollectAndUpload(ctx, acc)
	a.mu.Lock()
	a.running, a.failed = false, err != nil
	a.err = ""
	if err != nil {
		a.err = shareErrorText(err)
	}
	a.mu.Unlock()
	if err == nil {
		b := s.backups
		b.mu.Lock()
		if b.st.Archive != nil {
			ar := b.st.Archive
			if ar.Synced == nil {
				ar.Synced = map[string]int64{}
			}
			maps.Copy(ar.Synced, synced)
			ar.Gists = acc.gists
			ar.Last = now.UnixMilli()
			if err := b.saveLocked(); err != nil {
				diag.Error(areaBackup, "réglages des archives non enregistrés : %v", err)
			}
		}
		b.mu.Unlock()
		diag.Info(areaBackup, "archives : %d PC, %d minute(s), %d fichier(s) mis à jour%s", result.PCs, result.Minutes, result.Files,
			pick(len(result.Skipped) > 0, " ; ignorés : "+strings.Join(result.Skipped, ", "), ""))
	} else {
		diag.Warn(areaBackup, "archives : %v", err)
	}
	s.notify()
	return result, err
}

func (s *Service) archiveCollectAndUpload(ctx context.Context, acc archiveAccess) (ArchiveResult, map[string]int64, error) {
	var result ArchiveResult
	s.mu.Lock()
	devices := slices.Clone(s.cfg.Devices)
	s.mu.Unlock()
	work := map[string]*pcWork{} // MAC → travail
	synced := map[string]int64{}
	for _, d := range devices {
		if d.Agent == nil || !d.Agent.HasKey() || !d.HasHost() || d.MAC.IsZero() {
			continue
		}
		mac := d.MAC.String()
		w, err := s.collectPC(ctx, d, acc.synced[mac])
		if err != nil {
			if ctx.Err() != nil {
				return result, nil, ctx.Err()
			}
			result.Skipped = append(result.Skipped, fmt.Sprintf("%s (%s)", d.Name, describeAgentError(err)))
			continue
		}
		work[mac] = w
		synced[mac] = w.last
		result.PCs++
	}
	// Regroupement par mois : un Gist par mois, une seule écriture par Gist.
	months := map[string]bool{}
	for _, w := range work {
		for day := range w.rows {
			months[day[:7]] = true
		}
		for _, e := range w.events {
			months[time.Unix(e.T, 0).UTC().Format("2006-01")] = true
		}
	}
	for _, month := range slices.Sorted(maps.Keys(months)) {
		files, minutes, err := s.archiveMonth(ctx, acc, month, work)
		if err != nil {
			return result, nil, err
		}
		result.Files += files
		result.Minutes += minutes
	}
	return result, synced, nil
}

// collectPC lit les mesures (depuis la dernière minute archivée) et le journal d'un PC.
func (s *Service) collectPC(ctx context.Context, d model.Device, since int64) (*pcWork, error) {
	exchange := func(f func(ctx context.Context) error) error {
		ctx, cancel := context.WithTimeout(ctx, archiveAgentTimeout)
		defer cancel()
		return f(ctx)
	}
	var list protocol.Metrics
	if err := exchange(func(ctx context.Context) (err error) {
		list, err = s.agent.Metrics(ctx, d.Host, *d.Agent, "")
		return err
	}); err != nil {
		return nil, err
	}
	w := &pcWork{name: d.Name, rows: map[string][]protocol.MetricsRow{}, last: since}
	from := ""
	if since > 0 {
		from = time.Unix(since, 0).UTC().Format("2006-01-02")
	}
	for _, day := range list.Days {
		if day < from || !validDay(day) {
			continue
		}
		var m protocol.Metrics
		if err := exchange(func(ctx context.Context) (err error) {
			m, err = s.agent.Metrics(ctx, d.Host, *d.Agent, day)
			return err
		}); err != nil {
			return nil, err
		}
		var rows []protocol.MetricsRow
		for _, r := range m.Rows {
			if r.T > since {
				rows = append(rows, r)
				w.last = max(w.last, r.T)
			}
		}
		if len(rows) > 0 {
			w.rows[day] = rows
		}
	}
	var h protocol.History
	if err := exchange(func(ctx context.Context) (err error) {
		h, err = s.agent.History(ctx, d.Host, *d.Agent)
		return err
	}); err == nil {
		w.events = h.Events
	}
	return w, nil
}

// archiveMonth fusionne le travail d'un mois dans son Gist ; renvoie le nombre de fichiers écrits
// et de minutes ajoutées.
func (s *Service) archiveMonth(ctx context.Context, acc archiveAccess, month string, work map[string]*pcWork) (int, int, error) {
	gistID, key, files, err := s.monthGist(ctx, acc, month, true)
	if err != nil {
		return 0, 0, err
	}
	updates := map[string]*string{}
	minutes := 0
	// Mesures, jour par jour.
	days := map[string]bool{}
	for _, w := range work {
		for day := range w.rows {
			if strings.HasPrefix(day, month) {
				days[day] = true
			}
		}
	}
	for _, day := range slices.Sorted(maps.Keys(days)) {
		name := archive.DayFile(day)
		content := archive.Day{Day: day}
		if text, ok := files[name]; ok {
			if err := key.Open(name, text, &content); err != nil {
				diag.Warn(areaBackup, "archives : %s illisible, réécrit (%v)", name, err)
				content = archive.Day{Day: day}
			}
		}
		changed := false
		for _, mac := range slices.Sorted(maps.Keys(work)) {
			w := work[mac]
			if rows := w.rows[day]; len(rows) > 0 {
				var c bool
				content, c = content.AddRows(mac, w.name, rows)
				changed = changed || c
				if c {
					minutes += len(rows)
				}
			}
		}
		if changed {
			text, err := key.Seal(name, content)
			if err != nil {
				return 0, 0, err
			}
			updates[name] = &text
		}
	}
	// Journal du mois.
	name := archive.JournalFile(month)
	journal := archive.Journal{Month: month}
	if text, ok := files[name]; ok {
		if err := key.Open(name, text, &journal); err != nil {
			diag.Warn(areaBackup, "archives : %s illisible, réécrit (%v)", name, err)
			journal = archive.Journal{Month: month}
		}
	}
	changed := false
	for _, mac := range slices.Sorted(maps.Keys(work)) {
		var c bool
		journal, c = journal.AddEvents(mac, work[mac].name, work[mac].events)
		changed = changed || c
	}
	if changed {
		text, err := key.Seal(name, journal)
		if err != nil {
			return 0, 0, err
		}
		updates[name] = &text
	}
	if len(updates) == 0 {
		return 0, 0, nil
	}
	if err := s.backups.opts.GitHub.UpdateGist(ctx, acc.token, gistID, updates); err != nil {
		return 0, 0, err
	}
	// Données du Gist mises à jour : relues au besoin par l'écran « Mesures ».
	a := s.archives
	a.mu.Lock()
	delete(a.gists, gistID)
	a.mu.Unlock()
	return len(updates), minutes, nil
}

// monthGist renvoie le Gist d'archives d'un mois que le mot de passe ouvre, sa clé et ses fichiers ;
// create : le crée s'il n'existe pas (sinon ErrNotFound).
func (s *Service) monthGist(ctx context.Context, acc archiveAccess, month string, create bool) (string, *archive.Key, map[string]string, error) {
	gh := s.backups.opts.GitHub
	tried := map[string]bool{}
	try := func(id string) (*archive.Key, map[string]string, error) {
		tried[id] = true
		files, err := s.gistFiles(ctx, acc.token, id)
		if err != nil {
			return nil, nil, err
		}
		text, ok := files[archive.ManifestFile]
		if !ok {
			return nil, nil, errors.New("manifeste absent")
		}
		m, err := archive.ParseManifest(text)
		if err != nil {
			return nil, nil, err
		}
		if m.Month != month {
			return nil, nil, errors.New("Gist d'un autre mois")
		}
		key, err := s.archiveKey(id, m, acc.password)
		return key, files, err
	}
	if id := acc.gists[month]; id != "" {
		key, files, err := try(id)
		if err == nil {
			return id, key, files, nil
		}
		if ctx.Err() != nil || share.IsTransient(err) {
			return "", nil, nil, err
		}
		delete(acc.gists, month)
	}
	ids, err := s.archiveMonths(ctx, acc.token, false)
	if err != nil {
		return "", nil, nil, err
	}
	for _, id := range ids[month] {
		if tried[id] {
			continue
		}
		key, files, err := try(id)
		if err == nil {
			acc.gists[month] = id
			return id, key, files, nil
		}
		if ctx.Err() != nil || share.IsTransient(err) {
			return "", nil, nil, err
		}
		diag.Info(areaBackup, "archives %s : Gist %s ignoré (%v)", month, shortID(id), err)
	}
	if !create {
		return "", nil, nil, share.ErrNotFound
	}
	m, key, err := archive.NewManifest(month, acc.password)
	if err != nil {
		return "", nil, nil, err
	}
	id, err := gh.CreateGist(ctx, acc.token, archive.Description(month), map[string]string{
		archive.ManifestFile: m.JSON(),
		archive.ReadmeFile:   archive.Readme,
	})
	if err != nil {
		return "", nil, nil, err
	}
	diag.Info(areaBackup, "archives : Gist %s créé pour %s", shortID(id), month)
	a := s.archives
	a.mu.Lock()
	a.keys[id] = key
	if a.months != nil {
		a.months[month] = append(a.months[month], id)
	}
	a.mu.Unlock()
	acc.gists[month] = id
	return id, key, map[string]string{archive.ManifestFile: m.JSON()}, nil
}

// archiveKey dérive (une fois par session) la clé d'un Gist d'archives.
func (s *Service) archiveKey(id string, m archive.Manifest, password string) (*archive.Key, error) {
	a := s.archives
	a.mu.Lock()
	key := a.keys[id]
	a.mu.Unlock()
	if key != nil {
		return key, nil
	}
	key, err := m.Unlock(password)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	a.keys[id] = key
	a.mu.Unlock()
	return key, nil
}

// archiveMonths liste les Gists d'archives du compte, par mois.
func (s *Service) archiveMonths(ctx context.Context, token string, refresh bool) (map[string][]string, error) {
	a := s.archives
	a.mu.Lock()
	if a.months != nil && !refresh && s.now().Sub(a.monthsAt) < monthsTTL {
		out := maps.Clone(a.months)
		a.mu.Unlock()
		return out, nil
	}
	a.mu.Unlock()
	list, err := s.backups.opts.GitHub.ListGists(ctx, token, archive.DescriptionPrefix)
	if err != nil {
		return nil, err
	}
	months := map[string][]string{}
	for _, g := range list {
		if month := archive.MonthOf(g.Description); month != "" {
			months[month] = append(months[month], g.ID)
		}
	}
	a.mu.Lock()
	a.months, a.monthsAt = months, s.now()
	a.mu.Unlock()
	return maps.Clone(months), nil
}

// gistFiles lit les fichiers d'un Gist d'archives (gardés en mémoire quelques minutes).
func (s *Service) gistFiles(ctx context.Context, token, id string) (map[string]string, error) {
	a := s.archives
	a.mu.Lock()
	if c, ok := a.gists[id]; ok && s.now().Sub(c.at) < gistTTL {
		a.mu.Unlock()
		return c.files, nil
	}
	a.mu.Unlock()
	list, err := s.backups.opts.GitHub.ReadGist(ctx, token, id, archive.MaxFileBytes)
	if err != nil {
		return nil, err
	}
	files := map[string]string{}
	for _, f := range list {
		files[f.Name] = f.Content
	}
	a.mu.Lock()
	a.gists[id] = cachedGist{files: files, at: s.now()}
	a.mu.Unlock()
	return files, nil
}

func validDay(day string) bool {
	_, ok := archive.DayStart(day)
	return ok && len(day) == 10
}

func describeAgentError(err error) string {
	switch agentclient.CodeOf(err) {
	case agentclient.Unreachable, agentclient.Refused, agentclient.UnknownHost:
		return "injoignable"
	case agentclient.Rejected:
		return "agent antérieur à 1.8.0"
	case agentclient.Unauthorized:
		return "clé refusée"
	}
	return err.Error()
}

// --- Réglages ---

func (s *Service) archiveEnable(enabled bool) error {
	b, err := s.requireBackups()
	if err != nil {
		return err
	}
	b.mu.Lock()
	if enabled && (!b.st.Enabled || b.st.Password == "") {
		b.mu.Unlock()
		return errors.New("activez d'abord la sauvegarde automatique : les archives sont chiffrées par son mot de passe")
	}
	if enabled && (b.st.GitHub == nil || b.st.GitHub.Token == "") {
		b.mu.Unlock()
		return errors.New("connectez d'abord GitHub dans la sauvegarde automatique : les archives y sont rangées")
	}
	if b.st.Archive == nil {
		b.st.Archive = &backup.ArchiveSettings{}
	}
	b.st.Archive.Enabled = enabled
	if enabled {
		b.st.Archive.Last = 0 // premier archivage tout de suite
	}
	err = b.saveLocked()
	b.mu.Unlock()
	if err != nil {
		return err
	}
	diag.Info(areaBackup, "archives %s", pick(enabled, "activées", "désactivées"))
	if enabled {
		s.archives.poke()
	}
	s.notify()
	return nil
}

// ArchiveView est l'état des archives pour l'interface.
type ArchiveView struct {
	Enabled bool   `json:"enabled"`
	Running bool   `json:"running"`
	Last    int64  `json:"last,omitempty"`
	Error   string `json:"error,omitempty"`
}

func (s *Service) archiveView() *ArchiveView {
	a := s.archives
	if a == nil || s.backups == nil {
		return nil
	}
	acc := s.archiveAccess()
	a.mu.Lock()
	defer a.mu.Unlock()
	return &ArchiveView{Enabled: acc.enabled, Running: a.running, Last: acc.last, Error: a.err}
}

// describeArchives résume les archives pour le rapport de diagnostic (sans secret).
func (s *Service) describeArchives(line func(string, ...any)) {
	if s.archives == nil {
		return
	}
	acc := s.archiveAccess()
	a := s.archives
	a.mu.Lock()
	defer a.mu.Unlock()
	line("Archives : activées %t, dernière réussite %s, %d PC suivis, %d mois%s", acc.enabled, millisText(acc.last),
		len(acc.synced), len(acc.gists), errorSuffix(a.err))
}

// --- Écran « Mesures » ---

// MetricsPoint est un point des graphiques (moyenne et maximum sur sa durée).
type MetricsPoint struct {
	T          int64    `json:"t"`
	CPUTemp    *float64 `json:"ct,omitempty"`
	CPUTempMax *float64 `json:"ctx,omitempty"`
	GPUTemp    *float64 `json:"gt,omitempty"`
	GPUTempMax *float64 `json:"gtx,omitempty"`
	CPULoad    *float64 `json:"cl,omitempty"`
	CPULoadMax *float64 `json:"clx,omitempty"`
	GPULoad    *float64 `json:"gl,omitempty"`
	GPULoadMax *float64 `json:"glx,omitempty"`
}

// MetricsRangeView : mesures d'une période pour l'écran « Mesures ».
type MetricsRangeView struct {
	From int64 `json:"from"`
	To   int64 `json:"to"`
	// Step : durée d'un point (secondes).
	Step   int64          `json:"step"`
	Points []MetricsPoint `json:"points"`
	// Summary : moyenne et maximum de la période.
	Summary MetricsPoint `json:"summary"`
	Minutes int          `json:"minutes"`
	// Agent / Archive : sources utilisées ; Notes : ce qui manque (PC injoignable, archives absentes…).
	Agent   bool     `json:"agent"`
	Archive bool     `json:"archive"`
	Notes   []string `json:"notes,omitempty"`
}

// metricsRange lit les mesures d'un PC entre from et to (millisecondes).
func (s *Service) metricsRange(id string, fromMs, toMs int64, points int) (MetricsRangeView, error) {
	d, ok := s.device(id)
	if !ok {
		return MetricsRangeView{}, errors.New("PC introuvable")
	}
	from, to := fromMs/1000, toMs/1000
	if to <= from || to-from > maxRangeDays*86400 {
		return MetricsRangeView{}, errors.New("période invalide")
	}
	points = min(max(points, 10), maxChartPoints)
	ctx, cancel := context.WithTimeout(context.Background(), viewTimeout)
	defer cancel()
	view := MetricsRangeView{From: fromMs, To: toMs, Points: []MetricsPoint{}}
	notes := map[string]bool{}
	agentDays, agentErr := s.agentDays(ctx, d)
	if agentErr != nil && d.Agent != nil && d.Agent.HasKey() {
		notes[agentNote(agentErr)] = true
	}
	_, own := s.ownDevice(id)
	acc := s.archiveAccess()
	var rows []protocol.MetricsRow
	for day := time.Unix(from, 0).UTC().Truncate(24 * time.Hour); day.Unix() < to; day = day.AddDate(0, 0, 1) {
		name := day.Format("2006-01-02")
		if slices.Contains(agentDays, name) {
			r, err := s.agentDay(ctx, d, name)
			if err == nil {
				rows = append(rows, r...)
				view.Agent = true
				continue
			}
			notes[agentNote(err)] = true
		}
		if own && acc.usable() && !d.MAC.IsZero() {
			r, found, err := s.archiveDay(ctx, acc, d.MAC.String(), name)
			switch {
			case err != nil:
				notes["Archives illisibles : "+shareErrorText(err)] = true
			case found:
				rows = append(rows, r...)
				view.Archive = true
			}
		}
	}
	if own && !acc.usable() {
		notes["Archives GitHub non configurées : seules les mesures gardées par le PC (90 jours) sont disponibles."] = true
	}
	view.Step, view.Points, view.Summary, view.Minutes = downsample(rows, from, to, points)
	for n := range notes {
		if n != "" {
			view.Notes = append(view.Notes, n)
		}
	}
	slices.Sort(view.Notes)
	return view, nil
}

func agentNote(err error) string {
	switch agentclient.CodeOf(err) {
	case agentclient.Unreachable, agentclient.Refused, agentclient.UnknownHost:
		return "PC injoignable : mesures lues dans les archives GitHub."
	case agentclient.Rejected:
		return "Agent antérieur à 1.8.0 : mettez-le à jour pour enregistrer les mesures en continu."
	case agentclient.NoKey:
		return ""
	}
	return "Agent : " + err.Error()
}

// agentDays renvoie les jours enregistrés par l'agent d'un PC (gardés une minute).
func (s *Service) agentDays(ctx context.Context, d model.Device) ([]string, error) {
	if d.Agent == nil || !d.Agent.HasKey() || !d.HasHost() {
		return nil, &agentclient.Error{Code: agentclient.NoKey}
	}
	a := s.archives
	a.mu.Lock()
	if c, ok := a.days[d.ID]; ok && s.now().Sub(c.at) < agentDaysTTL {
		a.mu.Unlock()
		return c.days, c.err
	}
	a.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, archiveAgentTimeout)
	defer cancel()
	m, err := s.agent.Metrics(ctx, d.Host, *d.Agent, "")
	a.mu.Lock()
	a.days[d.ID] = cachedDays{days: m.Days, err: err, at: s.now()}
	a.mu.Unlock()
	return m.Days, err
}

// agentDay lit un jour de mesures sur l'agent (jours passés gardés dix minutes, jour en cours 30 s).
func (s *Service) agentDay(ctx context.Context, d model.Device, day string) ([]protocol.MetricsRow, error) {
	cacheKey := "agent|" + d.ID + "|" + day
	ttl := pastDayTTL
	if day == s.now().UTC().Format("2006-01-02") {
		ttl = todayTTL
	}
	a := s.archives
	a.mu.Lock()
	if c, ok := a.rows[cacheKey]; ok && s.now().Sub(c.at) < ttl {
		a.mu.Unlock()
		return c.rows, nil
	}
	a.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, archiveAgentTimeout)
	defer cancel()
	m, err := s.agent.Metrics(ctx, d.Host, *d.Agent, day)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	a.rows[cacheKey] = cachedRows{rows: m.Rows, at: s.now()}
	a.mu.Unlock()
	return m.Rows, nil
}

// archiveDay lit un jour de mesures d'un PC dans les archives (tous les Gists du mois que le mot de
// passe ouvre). found est faux si le jour n'est pas archivé.
func (s *Service) archiveDay(ctx context.Context, acc archiveAccess, mac, day string) ([]protocol.MetricsRow, bool, error) {
	months, err := s.archiveMonths(ctx, acc.token, false)
	if err != nil {
		return nil, false, err
	}
	merged := archive.Day{Day: day}
	found := false
	for _, id := range months[day[:7]] {
		key, files, err := s.openGist(ctx, acc, id, day[:7])
		if err != nil {
			continue // autre mot de passe, ou Gist abîmé : ignoré
		}
		text, ok := files[archive.DayFile(day)]
		if !ok {
			continue
		}
		var content archive.Day
		if err := key.Open(archive.DayFile(day), text, &content); err != nil {
			continue
		}
		if rows := content.Rows(mac); len(rows) > 0 {
			merged, _ = merged.AddRows(mac, "", rows)
			found = true
		}
	}
	return merged.Rows(mac), found, nil
}

// openGist ouvre un Gist d'archives du mois (clé et fichiers).
func (s *Service) openGist(ctx context.Context, acc archiveAccess, id, month string) (*archive.Key, map[string]string, error) {
	files, err := s.gistFiles(ctx, acc.token, id)
	if err != nil {
		return nil, nil, err
	}
	m, err := archive.ParseManifest(files[archive.ManifestFile])
	if err != nil {
		return nil, nil, err
	}
	if m.Month != month {
		return nil, nil, errors.New("Gist d'un autre mois")
	}
	key, err := s.archiveKey(id, m, acc.password)
	return key, files, err
}

// metricsJournal renvoie le journal d'un PC entre from et to (millisecondes) : celui de l'agent et
// celui des archives.
func (s *Service) metricsJournal(id string, fromMs, toMs int64) (any, error) {
	d, ok := s.device(id)
	if !ok {
		return nil, errors.New("PC introuvable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), viewTimeout)
	defer cancel()
	var raw []protocol.HistoryEvent
	if d.Agent != nil && d.Agent.HasKey() && d.HasHost() {
		if events, err := s.agentJournal(ctx, d); err == nil {
			raw = append(raw, events...)
		}
	}
	_, own := s.ownDevice(id)
	if acc := s.archiveAccess(); own && acc.usable() && !d.MAC.IsZero() {
		months, err := s.archiveMonths(ctx, acc.token, false)
		if err == nil {
			for t := time.UnixMilli(fromMs).UTC(); ; t = t.AddDate(0, 1, 0) {
				month := t.Format("2006-01")
				if month > time.UnixMilli(toMs).UTC().Format("2006-01") {
					break
				}
				for _, gid := range months[month] {
					key, files, err := s.openGist(ctx, acc, gid, month)
					if err != nil {
						continue
					}
					var j archive.Journal
					if text, ok := files[archive.JournalFile(month)]; ok && key.Open(archive.JournalFile(month), text, &j) == nil {
						raw = append(raw, j.Events(d.MAC.String())...)
					}
				}
				t = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
			}
		}
	}
	// Agent et archives : chaque évènement une fois, avec la cause la plus précise.
	var merged []protocol.HistoryEvent
	for _, e := range raw {
		if e.T*1000 >= fromMs && e.T*1000 < toMs {
			merged, _ = protocol.MergeEvent(merged, e)
		}
	}
	items := []HistoryItem{}
	for _, e := range merged {
		if ev, ok := history.FromAgent(d.ID, e); ok {
			items = append(items, itemOf(ev, d.Name))
		}
	}
	slices.SortStableFunc(items, func(a, b HistoryItem) int { return compareInt(b.Time, a.Time) })
	return map[string]any{"events": items}, nil
}

func (s *Service) agentJournal(ctx context.Context, d model.Device) ([]protocol.HistoryEvent, error) {
	a := s.archives
	a.mu.Lock()
	if c, ok := a.journals[d.ID]; ok && s.now().Sub(c.at) < agentDaysTTL {
		a.mu.Unlock()
		return c.events, nil
	}
	a.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, archiveAgentTimeout)
	defer cancel()
	h, err := s.agent.History(ctx, d.Host, *d.Agent)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	a.journals[d.ID] = cachedJournal{events: h.Events, at: s.now()}
	a.mu.Unlock()
	return h.Events, nil
}

func compareInt(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// metricsMonths renvoie les mois archivés sur GitHub (du plus récent au plus ancien).
func (s *Service) metricsMonths(refresh bool) (any, error) {
	acc := s.archiveAccess()
	if !acc.usable() || s.archives == nil {
		return map[string]any{"months": []string{}, "configured": false}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), viewTimeout)
	defer cancel()
	months, err := s.archiveMonths(ctx, acc.token, refresh)
	if err != nil {
		return nil, err
	}
	list := slices.Sorted(maps.Keys(months))
	slices.Reverse(list)
	return map[string]any{"months": list, "configured": true}, nil
}

// --- Réduction des mesures pour les graphiques ---

type agg struct {
	sum, weight, max float64
	n                int
}

func (g *agg) add(avg, maxValue *float64, weight int) {
	if avg == nil {
		return
	}
	w := float64(max(weight, 1))
	g.sum += *avg * w
	g.weight += w
	m := *avg
	if maxValue != nil {
		m = *maxValue
	}
	if g.n == 0 || m > g.max {
		g.max = m
	}
	g.n++
}

func (g agg) values() (*float64, *float64) {
	if g.n == 0 {
		return nil, nil
	}
	avg := math.Round(g.sum/g.weight*10) / 10
	mx := math.Round(g.max*10) / 10
	return &avg, &mx
}

type pointAgg struct{ ct, gt, cl, gl agg }

func (p *pointAgg) add(r protocol.MetricsRow) {
	p.ct.add(r.CPUTemp, r.CPUTempMax, r.N)
	p.gt.add(r.GPUTemp, r.GPUTempMax, r.N)
	p.cl.add(r.CPULoad, r.CPULoadMax, r.N)
	p.gl.add(r.GPULoad, r.GPULoadMax, r.N)
}

func (p pointAgg) point(t int64) MetricsPoint {
	pt := MetricsPoint{T: t}
	pt.CPUTemp, pt.CPUTempMax = p.ct.values()
	pt.GPUTemp, pt.GPUTempMax = p.gt.values()
	pt.CPULoad, pt.CPULoadMax = p.cl.values()
	pt.GPULoad, pt.GPULoadMax = p.gl.values()
	return pt
}

// downsample regroupe les minutes de [from, to[ (secondes) en au plus points points (moyenne
// pondérée par le nombre de relevés, maximum des maximums) ; renvoie aussi le résumé de la période.
func downsample(rows []protocol.MetricsRow, from, to int64, points int) (int64, []MetricsPoint, MetricsPoint, int) {
	step := max(int64(60), (to-from+int64(points)-1)/int64(points))
	step = (step + 59) / 60 * 60
	buckets := map[int64]*pointAgg{}
	var total pointAgg
	seen := map[int64]bool{}
	for _, r := range rows {
		if r.T < from || r.T >= to || seen[r.T] {
			continue
		}
		seen[r.T] = true
		i := (r.T - from) / step
		b := buckets[i]
		if b == nil {
			b = &pointAgg{}
			buckets[i] = b
		}
		b.add(r)
		total.add(r)
	}
	out := make([]MetricsPoint, 0, len(buckets))
	for _, i := range slices.Sorted(maps.Keys(buckets)) {
		out = append(out, buckets[i].point((from+i*step)*1000))
	}
	return step, out, total.point(from * 1000), len(seen)
}

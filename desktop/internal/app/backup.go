package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/slaynaw/wakeonlan/desktop/internal/backup"
	"github.com/slaynaw/wakeonlan/desktop/internal/config"
	"github.com/slaynaw/wakeonlan/desktop/internal/diag"
	"github.com/slaynaw/wakeonlan/desktop/internal/model"
	"github.com/slaynaw/wakeonlan/desktop/internal/share"
)

// Sauvegardes automatiques (docs/SAUVEGARDE.md) : après chaque changement (PC, réglages, partage) et
// au moins une fois par jour, une sauvegarde complète chiffrée par le mot de passe des sauvegardes est
// écrite dans le Gist secret du compte GitHub et/ou dans le dossier choisi (Keep versions par appareil).

// BackupOptions active les sauvegardes automatiques (nil : indisponibles).
type BackupOptions struct {
	Store *backup.Store
	// GitHub : connexion et Gists (même application OAuth que le partage).
	GitHub *share.GitHub
	// Kind et Name : type et nom de cet appareil, pour le nom des fichiers (« windows », nom du PC).
	Kind, Name string
}

const (
	// backupDelay : délai après le dernier changement (plusieurs modifications, une sauvegarde).
	backupDelay = 20 * time.Second
	backupDaily = 24 * time.Hour
	// backupRetry : délai avant un nouvel essai après un échec.
	backupRetry   = 10 * time.Minute
	backupTick    = 10 * time.Second
	backupStart   = 30 * time.Second
	backupTimeout = 90 * time.Second
	// backupListTTL : durée de conservation en mémoire de la liste lue pour une restauration.
	backupListTTL  = 10 * time.Minute
	backupGistNote = "# Patronus – sauvegardes chiffrées\n\nSauvegardes automatiques de l'application Patronus " +
		"(https://github.com/slaynAW/Patronus), chiffrées par un mot de passe : illisibles sans lui.\n"
)

type backups struct {
	opts *BackupOptions
	wake chan struct{}

	mu       sync.Mutex
	st       backup.Settings
	loadErr  string
	dirty    bool
	changed  time.Time
	running  bool
	attempt  time.Time
	failed   bool
	errGH    string
	errDir   string
	paused   string
	login    *shareLogin
	listing  map[string]string
	listedAt time.Time
	// stale : mot de passe changé sur un autre appareil (sauvegardes et archives arrêtées) ;
	// checked : Gist dont le mot de passe a été vérifié pendant cette session.
	stale   bool
	checked string
	// Changement du mot de passe en cours (voir backup_rotate.go).
	rotating   bool
	rotateStep string
	rotateErr  string
	rotateAt   time.Time
}

func newBackups(opts *BackupOptions) *backups {
	if opts == nil || opts.Store == nil {
		return nil
	}
	b := &backups{opts: opts, wake: make(chan struct{}, 1)}
	st, err := opts.Store.Load()
	if err != nil {
		b.loadErr = err.Error()
		diag.Error(areaData, "sauvegardes : %v", err)
	}
	b.st = st
	return b
}

func (b *backups) poke() {
	select {
	case b.wake <- struct{}{}:
	default:
	}
}

// saveLocked enregistre les réglages (verrou b.mu tenu).
func (b *backups) saveLocked() error { return b.opts.Store.Save(b.st) }

func (b *backups) hasTarget() bool {
	return b.st.GitHub != nil && b.st.GitHub.Gist != "" || b.st.Folder != ""
}

// markBackupDirty demande une sauvegarde (données modifiées).
func (s *Service) markBackupDirty() {
	if b := s.backups; b != nil {
		b.mu.Lock()
		b.dirty, b.changed = true, s.now()
		b.mu.Unlock()
		b.poke()
	}
}

// pauseBackups suspend les sauvegardes automatiques (données illisibles au démarrage : une
// sauvegarde automatique remplacerait la dernière bonne sauvegarde du jour).
func (s *Service) pauseBackups(reason string) {
	if b := s.backups; b != nil {
		b.mu.Lock()
		b.paused = reason
		b.mu.Unlock()
	}
}

// runBackups déclenche les sauvegardes automatiques jusqu'à l'annulation de ctx.
func (s *Service) runBackups(ctx context.Context) {
	b := s.backups
	if b == nil {
		return
	}
	timer := time.NewTimer(backupStart)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		case <-b.wake:
		}
		if s.rotationDue() {
			if err := s.runRotation(ctx); err != nil {
				diag.Warn(areaBackup, "changement du mot de passe interrompu, nouvel essai dans %s : %v", rotateRetry, err)
			}
		}
		if s.backupDue() {
			_, _ = s.backupNow(ctx, false)
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(backupTick)
	}
}

// backupDue indique qu'une sauvegarde automatique doit être faite maintenant.
func (s *Service) backupDue() bool {
	b := s.backups
	now := s.now()
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.st.Enabled || b.st.Password == "" || b.running || b.paused != "" || !b.hasTarget() || b.st.Rotation != nil || b.stale {
		return false
	}
	if b.failed && now.Sub(b.attempt) < backupRetry {
		return false
	}
	if b.dirty && now.Sub(b.changed) >= backupDelay {
		return true
	}
	last := int64(0)
	if b.st.GitHub != nil && b.st.GitHub.Gist != "" {
		last = b.st.LastGitHub
	}
	if b.st.Folder != "" && (last == 0 || b.st.LastFolder < last) {
		last = b.st.LastFolder
	}
	return now.Sub(time.UnixMilli(last)) >= backupDaily
}

// BackupResult est le résultat d'une sauvegarde.
type BackupResult struct {
	GitHub  string   `json:"github,omitempty"`
	Folder  string   `json:"folder,omitempty"`
	Skipped string   `json:"skipped,omitempty"`
	Errors  []string `json:"errors,omitempty"`
}

// backupNow sauvegarde vers chaque destination. manual : demandé par l'utilisateur (lève la pause).
func (s *Service) backupNow(ctx context.Context, manual bool) (BackupResult, error) {
	b := s.backups
	if b == nil {
		return BackupResult{}, errors.New("sauvegardes indisponibles")
	}
	now := s.now()
	b.mu.Lock()
	if !b.st.Enabled || b.st.Password == "" {
		b.mu.Unlock()
		return BackupResult{}, errors.New("sauvegarde automatique désactivée")
	}
	if !b.hasTarget() {
		b.mu.Unlock()
		return BackupResult{}, errors.New("choisissez d'abord où sauvegarder (GitHub ou un dossier)")
	}
	if b.running {
		b.mu.Unlock()
		return BackupResult{}, errors.New("sauvegarde déjà en cours")
	}
	if b.st.Rotation != nil {
		b.mu.Unlock()
		return BackupResult{}, errors.New("changement du mot de passe en cours : la sauvegarde suivra")
	}
	if b.stale {
		b.mu.Unlock()
		return BackupResult{}, errStalePassword
	}
	if manual {
		b.paused = ""
	}
	b.running, b.dirty, b.attempt = true, false, now
	b.mu.Unlock()
	s.notify()
	defer func() {
		b.mu.Lock()
		b.running = false
		b.mu.Unlock()
		s.notify()
	}()

	// Mot de passe changé sur un autre appareil ? Puis identifiant anonyme (fichiers renommés).
	if err := s.checkBackupPassword(ctx); errors.Is(err, errStalePassword) {
		return BackupResult{}, err
	} else if err != nil {
		diag.Warn(areaBackup, "vérification du mot de passe sur GitHub impossible : %v", err)
	}
	s.migrateDevice(ctx)

	b.mu.Lock()
	password, device, folder := b.st.Password, b.st.Device, b.st.Folder
	var gh backup.GitHub
	if b.st.GitHub != nil {
		gh = *b.st.GitHub
	}
	uploaded := slices.Clone(b.st.Uploaded)
	b.mu.Unlock()

	text, info, err := s.exportText(password, now)
	if err != nil {
		s.backupFailed(err.Error(), err.Error())
		return BackupResult{}, err
	}
	if !manual && info.devices == 0 && !info.sharing {
		// Rien à sauvegarder (données perdues ?) : ne pas remplacer une bonne sauvegarde.
		diag.Warn(areaBackup, "sauvegarde automatique ignorée : aucun PC ni partage")
		return BackupResult{Skipped: "aucun PC ni partage à sauvegarder"}, nil
	}
	name := backup.FileName(device, now)
	var result BackupResult

	var errGH, errDir string
	if gh.Gist != "" {
		content := string(text)
		files := map[string]*string{name: &content}
		names := append(slices.DeleteFunc(uploaded, func(n string) bool { return n == name }), name)
		old := backup.Outdated(names, device, backup.Keep)
		for _, n := range old {
			files[n] = nil
		}
		ctx, cancel := context.WithTimeout(ctx, backupTimeout)
		err := b.opts.GitHub.UpdateGist(ctx, gh.Token, gh.Gist, files)
		cancel()
		if err != nil {
			errGH = shareErrorText(err)
			if errors.Is(err, share.ErrNotFound) {
				errGH = "Gist des sauvegardes introuvable (supprimé ?) : reconnectez GitHub pour en créer un."
			}
			result.Errors = append(result.Errors, "GitHub : "+errGH)
			diag.Warn(areaBackup, "sauvegarde GitHub impossible : %v", err)
		} else {
			result.GitHub = name
			uploaded = slices.DeleteFunc(names, func(n string) bool { return slices.Contains(old, n) })
			diag.Info(areaBackup, "sauvegarde GitHub : %s (%d PC, %d octets, %d ancienne(s) version(s) supprimée(s))", name, info.devices, len(text), len(old))
		}
	}
	if folder != "" {
		path, err := backup.WriteFolder(folder, device, name, text)
		if err != nil {
			errDir = err.Error()
			result.Errors = append(result.Errors, "Dossier : "+errDir)
			diag.Warn(areaBackup, "sauvegarde dans le dossier impossible : %v", err)
		} else {
			result.Folder = path
			diag.Info(areaBackup, "sauvegarde dans le dossier : %s (%d PC)", path, info.devices)
		}
	}

	b.mu.Lock()
	b.errGH, b.errDir = errGH, errDir
	b.failed = len(result.Errors) > 0
	if result.GitHub != "" {
		b.st.LastGitHub = now.UnixMilli()
		b.st.Uploaded = uploaded
	}
	if result.Folder != "" {
		b.st.LastFolder = now.UnixMilli()
	}
	if err := b.saveLocked(); err != nil {
		diag.Error(areaBackup, "réglages des sauvegardes non enregistrés : %v", err)
	}
	b.mu.Unlock()
	if len(result.Errors) > 0 && result.GitHub == "" && result.Folder == "" {
		return result, errors.New(strings.Join(result.Errors, " ; "))
	}
	return result, nil
}

func (s *Service) backupFailed(errGH, errDir string) {
	b := s.backups
	b.mu.Lock()
	b.failed = true
	if b.st.GitHub != nil {
		b.errGH = errGH
	}
	if b.st.Folder != "" {
		b.errDir = errDir
	}
	b.mu.Unlock()
	diag.Error(areaBackup, "sauvegarde impossible : %s", errGH)
}

// --- Réglages ---

func (s *Service) requireBackups() (*backups, error) {
	if s.backups == nil {
		return nil, errors.New("sauvegardes indisponibles")
	}
	return s.backups, nil
}

// backupEnable active les sauvegardes automatiques avec ce mot de passe.
func (s *Service) backupEnable(password string) error {
	b, err := s.requireBackups()
	if err != nil {
		return err
	}
	if model.UTF16Len(password) < config.MinPasswordLength {
		return fmt.Errorf("Mot de passe trop court (%d caractères au moins)", config.MinPasswordLength)
	}
	b.mu.Lock()
	if b.st.Password != password {
		// Nouveau mot de passe : les archives repartent dans des Gists qu'il ouvre (rattrapage complet).
		b.st.Archive.ResetProgress()
	}
	b.st.Enabled, b.st.Password = true, password
	if b.st.Device == "" {
		b.st.Device = backup.DeviceID(b.opts.Kind)
	}
	err = b.saveLocked()
	b.dirty, b.changed = true, time.Time{}
	b.mu.Unlock()
	if err != nil {
		return err
	}
	diag.Info(areaBackup, "sauvegarde automatique activée (appareil %s)", b.st.Device)
	s.forgetArchiveKeys()
	b.poke()
	s.notify()
	return nil
}

// backupDisable désactive les sauvegardes et oublie le mot de passe (les sauvegardes existantes restent).
func (s *Service) backupDisable() error {
	b, err := s.requireBackups()
	if err != nil {
		return err
	}
	b.mu.Lock()
	if b.st.Rotation != nil {
		b.mu.Unlock()
		return errors.New("changement du mot de passe en cours : attendez qu'il se termine")
	}
	b.st.Enabled, b.st.Password, b.stale, b.checked = false, "", false, ""
	if b.st.Archive != nil {
		b.st.Archive.Enabled = false
	}
	err = b.saveLocked()
	b.mu.Unlock()
	s.forgetArchiveKeys()
	diag.Info(areaBackup, "sauvegarde automatique désactivée (archives arrêtées)")
	s.notify()
	return err
}

// backupPickFolder choisit le dossier des sauvegardes, puis sauvegarde.
func (s *Service) backupPickFolder() (any, error) {
	b, err := s.requireBackups()
	if err != nil {
		return nil, err
	}
	path, err := s.platform.PickFolder("Dossier des sauvegardes Patronus")
	if errors.Is(err, ErrCancelled) {
		return map[string]any{"cancelled": true}, nil
	}
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	b.st.Folder, b.errDir = path, ""
	err = b.saveLocked()
	b.dirty, b.changed = true, time.Time{}
	b.mu.Unlock()
	if err != nil {
		return nil, err
	}
	diag.Info(areaBackup, "dossier des sauvegardes : %s", path)
	b.poke()
	s.notify()
	return map[string]any{"path": path}, nil
}

// backupRemoveFolder retire le dossier des destinations (les fichiers restent).
func (s *Service) backupRemoveFolder() error {
	b, err := s.requireBackups()
	if err != nil {
		return err
	}
	b.mu.Lock()
	b.st.Folder, b.st.LastFolder, b.errDir = "", 0, ""
	err = b.saveLocked()
	b.mu.Unlock()
	s.notify()
	return err
}

// --- GitHub ---

// backupConnect connecte GitHub pour les sauvegardes : compte du partage s'il est connecté sur cet
// appareil, sinon connexion par code (comme le partage). Le Gist des sauvegardes est retrouvé ou créé.
// Si GitHub refuse le jeton du partage (expiré ou révoqué), le partage l'oublie et la connexion par
// code prend le relais : son nouveau jeton servira aussi au partage (même compte), voir shareAdoptToken.
func (s *Service) backupConnect() (any, error) {
	b, err := s.requireBackups()
	if err != nil {
		return nil, err
	}
	var shareUser string // compte du partage à reconnecter par la même occasion
	if sh := s.sharing; sh != nil {
		sh.mu.Lock()
		o := sh.state.Owner
		var token, user string
		if o != nil && o.User != "" {
			token, user = o.Token, o.User
		}
		sh.mu.Unlock()
		if token != "" {
			err := s.backupAttach(context.Background(), token, user, nil)
			if err == nil {
				return map[string]any{"connected": true, "user": user}, nil
			}
			if !errors.Is(err, share.ErrUnauthorized) {
				return nil, err
			}
			diag.Warn(areaBackup, "jeton GitHub du partage refusé (@%s) : connexion par code", user)
			s.shareTokenRejected(token, err)
		}
		if user != "" {
			shareUser = user
		}
	}
	gh := b.opts.GitHub
	if gh == nil || gh.ClientID == "" {
		return nil, errors.New("connexion GitHub non configurée dans cette version")
	}
	b.mu.Lock()
	if b.login != nil && b.login.cancel != nil {
		b.login.cancel()
	}
	b.login = nil
	b.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	dc, err := gh.StartLogin(ctx)
	if err != nil {
		cancel()
		return nil, err
	}
	login := &shareLogin{code: dc.UserCode, uri: dc.VerificationURI, cancel: cancel}
	b.mu.Lock()
	b.login = login
	b.mu.Unlock()
	s.notify()
	diag.Info(areaBackup, "connexion GitHub des sauvegardes : code affiché")
	go s.backupFinishLogin(ctx, login, dc)
	r := map[string]any{"code": dc.UserCode, "uri": dc.VerificationURI}
	if shareUser != "" {
		r["shareUser"] = shareUser
	}
	return r, nil
}

func (s *Service) backupFinishLogin(ctx context.Context, login *shareLogin, dc share.DeviceCode) {
	b := s.backups
	gh := b.opts.GitHub
	defer login.cancel()
	fail := func(err error) {
		diag.Warn(areaBackup, "connexion GitHub des sauvegardes : %v", err)
		b.mu.Lock()
		if b.login == login {
			login.err = err.Error()
		}
		b.mu.Unlock()
		s.notify()
	}
	token, err := gh.WaitLogin(ctx, dc)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			fail(err)
		}
		return
	}
	var user string
	if err := gh.Retry(ctx, func() (err error) {
		user, err = gh.User(ctx, token)
		return err
	}); err != nil {
		fail(err)
		return
	}
	b.mu.Lock()
	current := b.login == login
	b.mu.Unlock()
	if !current {
		return
	}
	if err := s.backupAttach(ctx, token, user, login); err != nil {
		fail(err)
		return
	}
	s.shareAdoptToken(token, user)
}

// backupAttach enregistre le compte GitHub et retrouve (ou crée) le Gist des sauvegardes. La
// connexion par code login (s'il y en a une) se termine en même temps : l'état ne montre jamais le
// compte connecté avec le code encore affiché.
func (s *Service) backupAttach(ctx context.Context, token, user string, login *shareLogin) error {
	b := s.backups
	gh := b.opts.GitHub
	ctx, cancel := context.WithTimeout(ctx, backupTimeout)
	defer cancel()
	var gist string
	err := gh.Retry(ctx, func() (err error) {
		gist, err = gh.FindGist(ctx, token, backup.GistDescription)
		return err
	})
	if err == nil && gist == "" {
		err = gh.Retry(ctx, func() (err error) {
			gist, err = gh.CreateGist(ctx, token, backup.GistDescription, map[string]string{"LISEZMOI.md": backupGistNote})
			return err
		})
	}
	if err != nil {
		return err
	}
	b.mu.Lock()
	if b.st.GitHub == nil || b.st.GitHub.User != user {
		b.st.Archive.ResetProgress() // autre compte : archives à rattraper dans ses Gists
	}
	b.st.GitHub = &backup.GitHub{Token: token, User: user, Gist: gist}
	b.errGH, b.listing = "", nil
	err = b.saveLocked()
	if err == nil && login != nil && b.login == login {
		b.login = nil
	}
	b.dirty, b.changed = true, time.Time{}
	b.mu.Unlock()
	if err != nil {
		return err
	}
	diag.Info(areaBackup, "GitHub connecté pour les sauvegardes : @%s, gist %s", user, shortID(gist))
	s.forgetArchiveKeys()
	b.poke()
	s.notify()
	return nil
}

func (s *Service) backupCancelLogin() {
	if b := s.backups; b != nil {
		b.mu.Lock()
		if b.login != nil {
			b.login.cancel()
			b.login = nil
		}
		b.mu.Unlock()
		s.notify()
	}
}

// backupDisconnect oublie le compte GitHub des sauvegardes (le Gist et ses sauvegardes restent).
func (s *Service) backupDisconnect() error {
	b, err := s.requireBackups()
	if err != nil {
		return err
	}
	b.mu.Lock()
	if b.st.Rotation != nil {
		b.mu.Unlock()
		return errors.New("changement du mot de passe en cours : attendez qu'il se termine")
	}
	b.st.GitHub, b.st.LastGitHub, b.st.Uploaded, b.errGH, b.listing = nil, 0, nil, "", nil
	b.stale, b.checked = false, ""
	b.st.Archive.ResetProgress()
	if b.st.Archive != nil {
		b.st.Archive.Enabled = false
	}
	err = b.saveLocked()
	b.mu.Unlock()
	s.forgetArchiveKeys()
	diag.Info(areaBackup, "GitHub déconnecté des sauvegardes")
	s.notify()
	return err
}

// --- Restauration depuis GitHub ---

// BackupEntry est une sauvegarde disponible sur GitHub.
type BackupEntry struct {
	backup.Entry
	// Mine : sauvegarde de cet appareil.
	Mine bool `json:"mine"`
}

// backupList liste les sauvegardes du Gist (de la plus récente à la plus ancienne).
func (s *Service) backupList() (any, error) {
	b, err := s.requireBackups()
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	var gh backup.GitHub
	if b.st.GitHub != nil {
		gh = *b.st.GitHub
	}
	device := b.st.Device
	b.mu.Unlock()
	if gh.Token == "" {
		return map[string]any{"connected": false}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), backupTimeout)
	defer cancel()
	files, err := b.opts.GitHub.ReadGist(ctx, gh.Token, gh.Gist, config.MaxImportBytes)
	if errors.Is(err, share.ErrNotFound) {
		return nil, errors.New("Gist des sauvegardes introuvable : reconnectez GitHub")
	}
	if err != nil {
		return nil, err
	}
	contents := map[string]string{}
	var names []string
	sizes := map[string]int{}
	for _, f := range files {
		contents[f.Name] = f.Content
		sizes[f.Name] = f.Size
		names = append(names, f.Name)
	}
	entries := []BackupEntry{}
	for _, e := range backup.Sorted(names) {
		e.Size = sizes[e.Name]
		entries = append(entries, BackupEntry{Entry: e, Mine: e.Device == device})
	}
	b.mu.Lock()
	b.listing, b.listedAt = contents, s.now()
	b.mu.Unlock()
	diag.Info(areaBackup, "restauration : %d sauvegarde(s) sur GitHub", len(entries))
	return map[string]any{"connected": true, "user": gh.User, "entries": entries}, nil
}

// backupRestore ouvre une sauvegarde de la liste : l'import habituel prend le relais (mot de passe,
// puis confirmation).
func (s *Service) backupRestore(name string) (any, error) {
	b, err := s.requireBackups()
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	content, ok := b.listing[name]
	fresh := s.now().Sub(b.listedAt) < backupListTTL
	b.mu.Unlock()
	if !ok || !fresh {
		return nil, errors.New("liste des sauvegardes expirée : rouvrez-la")
	}
	diag.Info(areaBackup, "restauration de %s", name)
	return s.importFile([]byte(content))
}

// --- État affiché ---

// BackupView est l'état des sauvegardes pour l'interface (sans mot de passe ni jeton).
type BackupView struct {
	Available bool   `json:"available"`
	CanLogin  bool   `json:"canLogin"`
	Enabled   bool   `json:"enabled"`
	Running   bool   `json:"running"`
	Paused    string `json:"paused,omitempty"`
	Error     string `json:"error,omitempty"`
	// GitHub : compte connecté (nil : aucun) ; Folder : dossier choisi (nil : aucun).
	GitHub *BackupTargetView `json:"github,omitempty"`
	Folder *BackupTargetView `json:"folder,omitempty"`
	Login  *LoginView        `json:"login,omitempty"`
	// Archive : archives des mesures et du journal des PC.
	Archive *ArchiveView `json:"archive,omitempty"`
	// Rotation : changement du mot de passe en cours ; Stale : mot de passe changé sur un autre
	// appareil (nouveau mot de passe à saisir).
	Rotation *RotationView `json:"rotation,omitempty"`
	Stale    bool          `json:"stale,omitempty"`
}

// BackupTargetView est une destination des sauvegardes.
type BackupTargetView struct {
	// Label : compte GitHub ou chemin du dossier.
	Label string `json:"label"`
	Last  int64  `json:"last,omitempty"`
	Error string `json:"error,omitempty"`
}

func (s *Service) backupView() BackupView {
	b := s.backups
	if b == nil {
		return BackupView{}
	}
	archiveView := s.archiveView()
	b.mu.Lock()
	defer b.mu.Unlock()
	v := BackupView{
		Available: true, CanLogin: b.opts.GitHub != nil && b.opts.GitHub.ClientID != "",
		Enabled: b.st.Enabled, Running: b.running, Paused: b.paused, Error: b.loadErr,
	}
	if s.sharing != nil && !v.CanLogin {
		v.CanLogin = s.sharing.gh.ClientID != ""
	}
	if gh := b.st.GitHub; gh != nil {
		v.GitHub = &BackupTargetView{Label: "@" + gh.User, Last: b.st.LastGitHub, Error: b.errGH}
	}
	if b.st.Folder != "" {
		v.Folder = &BackupTargetView{Label: b.st.Folder, Last: b.st.LastFolder, Error: b.errDir}
	}
	if l := b.login; l != nil {
		v.Login = &LoginView{Code: l.code, URI: l.uri, Error: l.err}
	}
	v.Archive = archiveView
	v.Rotation, v.Stale = b.rotationView(), b.stale
	return v
}

// describeBackups résume les sauvegardes pour le rapport de diagnostic (sans secret).
func (s *Service) describeBackups(line func(string, ...any)) {
	b := s.backups
	if b == nil {
		line("Sauvegardes automatiques indisponibles dans cette version.")
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	line("Activées %t, mot de passe %s, appareil %s, en cours %t%s%s", b.st.Enabled, pick(b.st.Password != "", "présent", "absent"),
		pick(b.st.Device != "", b.st.Device, "-"), b.running, pick(b.paused != "", ", EN PAUSE : "+b.paused, ""), errorSuffix(b.loadErr))
	if b.st.LegacyDevice != "" {
		line("Ancien identifiant (avec le nom de l'appareil) : fichiers à renommer")
	}
	if r := b.st.Rotation; r != nil {
		line("Changement du mot de passe en cours depuis %s, %d étape(s) faite(s), en cours %t%s", millisText(r.Started), len(r.Done), b.rotating, errorSuffix(b.rotateErr))
	}
	if b.stale {
		line("MOT DE PASSE CHANGÉ SUR UN AUTRE APPAREIL : nouveau mot de passe à saisir")
	}
	if gh := b.st.GitHub; gh != nil {
		line("GitHub : @%s, gist %s, jeton %s, dernière réussite %s, %d version(s) de cet appareil%s", gh.User, shortID(gh.Gist),
			pick(gh.Token != "", "présent", "absent"), millisText(b.st.LastGitHub), len(b.st.Uploaded), errorSuffix(b.errGH))
	} else {
		line("GitHub : non connecté")
	}
	if b.st.Folder != "" {
		line("Dossier : %s, dernière réussite %s%s", b.st.Folder, millisText(b.st.LastFolder), errorSuffix(b.errDir))
	} else {
		line("Dossier : aucun")
	}
}

// describeBackupsAndArchives : sauvegardes puis archives (verrous pris l'un après l'autre).
func (s *Service) describeBackupsAndArchives(line func(string, ...any)) {
	s.describeBackups(line)
	s.describeArchives(line)
}

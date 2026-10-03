package app

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/slaynaw/wakeonlan/desktop/internal/archive"
	"github.com/slaynaw/wakeonlan/desktop/internal/backup"
	"github.com/slaynaw/wakeonlan/desktop/internal/config"
	"github.com/slaynaw/wakeonlan/desktop/internal/diag"
	"github.com/slaynaw/wakeonlan/desktop/internal/model"
	"github.com/slaynaw/wakeonlan/desktop/internal/share"
)

// Changement du mot de passe des sauvegardes et identifiant anonyme de l'appareil (docs/SAUVEGARDE.md).
//
//   - Changer le mot de passe rechiffre tout ce que l'ancien ouvre : sauvegardes de tous les appareils
//     (Gist et dossier) et archives des mesures. Chaque Gist est recopié dans un Gist neuf, vérifié,
//     puis l'ancien est supprimé : son historique (versions chiffrées par l'ancien mot de passe)
//     disparaît avec lui. Rien n'est perdu ; un changement interrompu reprend où il s'était arrêté.
//   - Les autres appareils s'en aperçoivent (leurs sauvegardes ne s'ouvrent plus avec leur mot de
//     passe) : sauvegardes et archives s'y arrêtent jusqu'à la saisie du nouveau mot de passe.
//   - Les fichiers portaient le nom de l'appareil avant la 1.9.0 : ils sont renommés avec un
//     identifiant anonyme, dans un Gist recopié pour que l'ancien nom disparaisse de l'historique.

const (
	rotateTimeout = 30 * time.Minute
	rotateRetry   = 2 * time.Minute
	// gistBatchBytes : volume envoyé par requête lors de la recopie d'un Gist.
	gistBatchBytes = 900 << 10
	// gistCopyLimit : taille maximale d'un fichier recopié (au-delà, la recopie échoue sans rien perdre).
	gistCopyLimit = 16 << 20
)

// errStalePassword : le mot de passe a été changé sur un autre appareil.
var errStalePassword = errors.New("le mot de passe des sauvegardes a été changé sur un autre appareil : saisissez le nouveau mot de passe")

// RotationView est l'état d'un changement de mot de passe pour l'interface.
type RotationView struct {
	Running bool   `json:"running"`
	Step    string `json:"step,omitempty"`
	Error   string `json:"error,omitempty"`
}

// --- Recopie d'un Gist ---

// gistBatches répartit les fichiers (triés par nom) en lots d'environ gistBatchBytes.
func gistBatches(files map[string]string) []map[string]string {
	var out []map[string]string
	current, size := map[string]string{}, 0
	for _, name := range slices.Sorted(maps.Keys(files)) {
		if len(current) > 0 && size+len(files[name]) > gistBatchBytes {
			out = append(out, current)
			current, size = map[string]string{}, 0
		}
		current[name] = files[name]
		size += len(files[name])
	}
	if len(current) > 0 {
		out = append(out, current)
	}
	return out
}

// replaceGist recopie files dans un Gist secret neuf (même description), vérifie la copie, puis
// supprime l'ancien Gist : son historique disparaît avec lui. Un remplaçant déjà créé par une recopie
// interrompue est réutilisé. Renvoie l'identifiant du nouveau Gist.
func (s *Service) replaceGist(ctx context.Context, token, oldID, description string, files map[string]string) (string, error) {
	b := s.backups
	gh := b.opts.GitHub
	if len(files) == 0 {
		return "", errors.New("Gist vide : rien à recopier")
	}
	b.mu.Lock()
	newID := b.st.Replacing[oldID]
	b.mu.Unlock()
	batches := gistBatches(files)
	var extra []string
	if newID != "" {
		existing, err := gh.ReadGist(ctx, token, newID, gistCopyLimit)
		switch {
		case errors.Is(err, share.ErrNotFound):
			newID = ""
		case err != nil:
			return "", err
		default:
			for _, f := range existing {
				if _, ok := files[f.Name]; !ok {
					extra = append(extra, f.Name)
				}
			}
		}
	}
	if newID == "" {
		id, err := gh.CreateGist(ctx, token, description, batches[0])
		if err != nil {
			return "", err
		}
		newID = id
		b.mu.Lock()
		if b.st.Replacing == nil {
			b.st.Replacing = map[string]string{}
		}
		b.st.Replacing[oldID] = newID
		err = b.saveLocked()
		b.mu.Unlock()
		if err != nil {
			return "", err
		}
		batches = batches[1:]
		diag.Info(areaBackup, "recopie du Gist %s dans %s (%d fichier(s))", shortID(oldID), shortID(newID), len(files))
	}
	for i, batch := range batches {
		update := map[string]*string{}
		for name, content := range batch {
			update[name] = &content
		}
		if i == 0 {
			for _, name := range extra {
				update[name] = nil
			}
		}
		if err := gh.UpdateGist(ctx, token, newID, update); err != nil {
			return "", err
		}
	}
	// Vérification : la copie doit être complète et identique avant de supprimer l'original.
	copied, err := gh.ReadGistAll(ctx, token, newID, gistCopyLimit)
	if err != nil {
		return "", err
	}
	got := map[string]string{}
	for _, f := range copied {
		got[f.Name] = f.Content
	}
	for name, content := range files {
		if c, ok := got[name]; !ok || strings.TrimSpace(c) != strings.TrimSpace(content) {
			return "", fmt.Errorf("copie du Gist incomplète (%s) : l'original est gardé", name)
		}
	}
	if len(got) != len(files) {
		return "", errors.New("copie du Gist différente de l'original : l'original est gardé")
	}
	if err := gh.DeleteGist(ctx, token, oldID); err != nil && !errors.Is(err, share.ErrNotFound) {
		return "", err
	}
	b.mu.Lock()
	delete(b.st.Replacing, oldID)
	if len(b.st.Replacing) == 0 {
		b.st.Replacing = nil
	}
	err = b.saveLocked()
	b.mu.Unlock()
	if err != nil {
		diag.Warn(areaBackup, "Gist %s remplacé par %s, réglages non enregistrés : %v", shortID(oldID), shortID(newID), err)
	}
	diag.Info(areaBackup, "Gist %s remplacé par %s (ancien supprimé avec son historique)", shortID(oldID), shortID(newID))
	return newID, nil
}

// --- Gist des sauvegardes ---

// backupGistFiles lit le Gist des sauvegardes ; s'il a disparu (recopié par un autre appareil),
// celui du compte est retrouvé par sa description et enregistré. id vide : aucun Gist sur le compte.
func (s *Service) backupGistFiles(ctx context.Context, token, gist string, strict bool) (string, []share.GistFile, error) {
	b := s.backups
	gh := b.opts.GitHub
	read := func(id string) ([]share.GistFile, error) {
		if strict {
			return gh.ReadGistAll(ctx, token, id, gistCopyLimit)
		}
		return gh.ReadGist(ctx, token, id, config.MaxImportBytes)
	}
	files, err := read(gist)
	if !errors.Is(err, share.ErrNotFound) {
		return gist, files, err
	}
	found, err := gh.FindGist(ctx, token, backup.GistDescription)
	if err != nil || found == "" || found == gist {
		return "", nil, err
	}
	if files, err = read(found); err != nil {
		return "", nil, err
	}
	b.mu.Lock()
	if b.st.GitHub != nil && b.st.GitHub.Gist == gist {
		b.st.GitHub.Gist = found
		b.st.Uploaded = nil
		for _, f := range files {
			if e, ok := backup.Parse(f.Name); ok && e.Device == b.st.Device {
				b.st.Uploaded = append(b.st.Uploaded, f.Name)
			}
		}
		if err := b.saveLocked(); err != nil {
			diag.Warn(areaBackup, "nouveau Gist des sauvegardes non enregistré : %v", err)
		}
	}
	b.mu.Unlock()
	diag.Info(areaBackup, "Gist des sauvegardes %s introuvable, %s retrouvé sur le compte", shortID(gist), shortID(found))
	return found, files, nil
}

// latestOwn renvoie la sauvegarde la plus récente de cet appareil (ancien identifiant compris).
func latestOwn(files []share.GistFile, device, legacy string) (share.GistFile, bool) {
	var names []string
	byName := map[string]share.GistFile{}
	for _, f := range files {
		names = append(names, f.Name)
		byName[f.Name] = f
	}
	for _, e := range backup.Sorted(names) {
		if e.Device == device || legacy != "" && e.Device == legacy {
			return byName[e.Name], true
		}
	}
	return share.GistFile{}, false
}

// checkBackupPassword vérifie (une fois par session et par Gist) que le mot de passe enregistré ouvre
// encore la dernière sauvegarde de cet appareil sur GitHub. Sinon, il a été changé sur un autre
// appareil : sauvegardes et archives s'arrêtent jusqu'à la saisie du nouveau (errStalePassword).
func (s *Service) checkBackupPassword(ctx context.Context) error {
	b := s.backups
	b.mu.Lock()
	if b.stale {
		b.mu.Unlock()
		return errStalePassword
	}
	gh := b.st.GitHub
	if gh == nil || gh.Gist == "" || gh.Token == "" || b.st.Password == "" || b.checked == gh.Gist {
		b.mu.Unlock()
		return nil
	}
	token, gist, password, device, legacy := gh.Token, gh.Gist, b.st.Password, b.st.Device, b.st.LegacyDevice
	b.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, backupTimeout)
	defer cancel()
	id, files, err := s.backupGistFiles(ctx, token, gist, false)
	if err != nil {
		return err
	}
	if id == "" {
		return nil
	}
	stale := false
	if own, ok := latestOwn(files, device, legacy); ok {
		var cfgErr *config.Error
		if err := config.CheckPassword([]byte(own.Content), password); errors.As(err, &cfgErr) && cfgErr.Reason == config.WrongPassword {
			stale = true
		}
	}
	b.mu.Lock()
	if stale {
		b.stale, b.errGH = true, errStalePassword.Error()
	} else {
		b.checked = id
	}
	b.mu.Unlock()
	if stale {
		diag.Warn(areaBackup, "mot de passe des sauvegardes changé sur un autre appareil : sauvegardes et archives arrêtées")
		s.notify()
		return errStalePassword
	}
	return nil
}

// rebuildBackupGist recopie le Gist des sauvegardes dans un Gist neuf : sauvegardes que oldPassword
// ouvre rechiffrées par newPassword (toutes les sauvegardes si les deux sont égaux : simple recopie),
// fichiers de l'ancien identifiant legacy renommés. Renvoie le nouveau Gist ("" : aucun Gist).
func (s *Service) rebuildBackupGist(ctx context.Context, token, gist, oldPassword, newPassword, legacy, device string) (string, error) {
	id, files, err := s.backupGistFiles(ctx, token, gist, true)
	if err != nil || id == "" {
		return "", err
	}
	present := map[string]bool{}
	for _, f := range files {
		present[f.Name] = true
	}
	out := map[string]string{"LISEZMOI.md": backupGistNote}
	reencrypted, kept, renamed := 0, 0, 0
	for _, f := range files {
		name, content := f.Name, f.Content
		if legacy != "" {
			if to, ok := backup.Renamed(name, legacy, device); ok {
				if present[to] {
					continue // sauvegarde du même jour déjà faite sous le nouvel identifiant
				}
				name = to
				renamed++
			}
		}
		if _, ok := backup.Parse(f.Name); ok && oldPassword != newPassword {
			next, err := config.Reencrypt([]byte(content), oldPassword, newPassword)
			if err == nil {
				content = string(next)
				reencrypted++
			} else {
				kept++ // autre mot de passe (autre appareil) ou déjà rechiffrée : gardée telle quelle
			}
		}
		out[name] = content
	}
	if oldPassword == newPassword && renamed == 0 {
		return id, nil // rien à renommer : Gist gardé tel quel
	}
	newID, err := s.replaceGist(ctx, token, id, backup.GistDescription, out)
	if err != nil {
		return "", err
	}
	diag.Info(areaBackup, "Gist des sauvegardes recopié : %d rechiffrée(s), %d gardée(s) telle(s) quelle(s), %d renommée(s)", reencrypted, kept, renamed)
	b := s.backups
	b.mu.Lock()
	if b.st.GitHub != nil && (b.st.GitHub.Gist == gist || b.st.GitHub.Gist == id) {
		b.st.GitHub.Gist = newID
		var uploaded []string
		for name := range out {
			if e, ok := backup.Parse(name); ok && e.Device == device {
				uploaded = append(uploaded, name)
			}
		}
		sort.Strings(uploaded)
		b.st.Uploaded = uploaded
	}
	b.checked = newID
	err = b.saveLocked()
	b.mu.Unlock()
	return newID, err
}

// --- Identifiant anonyme ---

// migrateFolder renomme les sauvegardes de l'ancien identifiant dans le dossier.
func migrateFolder(dir, legacy, device string) error {
	names, err := backup.FolderBackups(dir)
	if err != nil {
		return err
	}
	present := map[string]bool{}
	for _, n := range names {
		present[n] = true
	}
	for _, n := range names {
		to, ok := backup.Renamed(n, legacy, device)
		if !ok {
			continue
		}
		if present[to] {
			_ = os.Remove(filepath.Join(dir, n))
			continue
		}
		if err := os.Rename(filepath.Join(dir, n), filepath.Join(dir, to)); err != nil {
			return err
		}
	}
	return nil
}

// migrateDevice remplace l'identifiant de cet appareil (avec son nom, avant la 1.9.0) par un
// identifiant anonyme et renomme ses sauvegardes (Gist recopié, dossier). Rien n'est perdu ; un échec
// est retenté à la sauvegarde suivante.
func (s *Service) migrateDevice(ctx context.Context) {
	b := s.backups
	b.mu.Lock()
	if b.st.Device != "" && !backup.Anonymous(b.st.Device) {
		if b.st.LegacyDevice == "" {
			b.st.LegacyDevice = b.st.Device
		}
		b.st.Device = backup.DeviceID(b.opts.Kind)
		for i, n := range b.st.Uploaded {
			if to, ok := backup.Renamed(n, b.st.LegacyDevice, b.st.Device); ok {
				b.st.Uploaded[i] = to
			}
		}
		if err := b.saveLocked(); err != nil {
			diag.Warn(areaBackup, "nouvel identifiant non enregistré : %v", err)
		}
		diag.Info(areaBackup, "identifiant anonyme de l'appareil : %s (fichiers à renommer)", b.st.Device)
	}
	legacy, device, password, folder := b.st.LegacyDevice, b.st.Device, b.st.Password, b.st.Folder
	var token, gist string
	if gh := b.st.GitHub; gh != nil {
		token, gist = gh.Token, gh.Gist
	}
	b.mu.Unlock()
	if legacy == "" {
		return
	}
	ok := true
	if folder != "" {
		if err := migrateFolder(folder, legacy, device); err != nil {
			ok = false
			diag.Warn(areaBackup, "renommage des sauvegardes du dossier impossible : %v", err)
		}
	}
	if token != "" && gist != "" {
		ctx, cancel := context.WithTimeout(ctx, rotateTimeout)
		_, err := s.rebuildBackupGist(ctx, token, gist, password, password, legacy, device)
		cancel()
		if err != nil {
			ok = false
			diag.Warn(areaBackup, "renommage des sauvegardes sur GitHub impossible, nouvel essai plus tard : %v", err)
		}
	}
	if ok {
		b.mu.Lock()
		b.st.LegacyDevice = ""
		err := b.saveLocked()
		b.mu.Unlock()
		if err == nil {
			diag.Info(areaBackup, "sauvegardes renommées : le nom de l'appareil n'apparaît plus")
		}
	}
}

// --- Changement du mot de passe ---

// backupChangePassword démarre le changement du mot de passe (current : mot de passe actuel).
func (s *Service) backupChangePassword(current, password string) error {
	b, err := s.requireBackups()
	if err != nil {
		return err
	}
	if model.UTF16Len(password) < config.MinPasswordLength {
		return fmt.Errorf("Mot de passe trop court (%d caractères au moins)", config.MinPasswordLength)
	}
	b.mu.Lock()
	stored := b.st.Password
	switch {
	case !b.st.Enabled || stored == "":
		err = errors.New("sauvegarde automatique désactivée")
	case b.st.Rotation != nil:
		err = errors.New("changement du mot de passe déjà en cours")
	case b.stale:
		err = errStalePassword
	case b.running:
		err = errors.New("sauvegarde en cours : réessayez dans un instant")
	case subtle.ConstantTimeCompare([]byte(current), []byte(stored)) != 1:
		err = errors.New("Mot de passe actuel incorrect")
	case password == stored:
		err = errors.New("Le nouveau mot de passe est identique à l'actuel")
	}
	if err != nil {
		b.mu.Unlock()
		return err
	}
	if s.archivesRunning() {
		b.mu.Unlock()
		return errors.New("archivage en cours : réessayez dans un instant")
	}
	err = s.startRotationLocked(stored, password)
	b.mu.Unlock()
	if err != nil {
		return err
	}
	diag.Info(areaBackup, "changement du mot de passe des sauvegardes demandé")
	s.afterRotationStart()
	return nil
}

// backupUpdatePassword enregistre le nouveau mot de passe choisi sur un autre appareil (vérifié sur la
// dernière sauvegarde de cet appareil), puis rechiffre ce qui ne l'est pas encore (dossier…).
func (s *Service) backupUpdatePassword(password string) error {
	b, err := s.requireBackups()
	if err != nil {
		return err
	}
	b.mu.Lock()
	gh := b.st.GitHub
	if !b.stale || gh == nil || b.st.Rotation != nil {
		b.mu.Unlock()
		return errors.New("aucun nouveau mot de passe attendu")
	}
	token, gist, device, legacy := gh.Token, gh.Gist, b.st.Device, b.st.LegacyDevice
	b.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), backupTimeout)
	defer cancel()
	_, files, err := s.backupGistFiles(ctx, token, gist, false)
	if err != nil {
		return err
	}
	if own, ok := latestOwn(files, device, legacy); ok {
		if err := config.CheckPassword([]byte(own.Content), password); err != nil {
			return errors.New("Mot de passe incorrect : saisissez celui choisi sur l'autre appareil")
		}
	}
	b.mu.Lock()
	err = s.startRotationLocked(b.st.Password, password)
	b.mu.Unlock()
	if err != nil {
		return err
	}
	diag.Info(areaBackup, "nouveau mot de passe des sauvegardes saisi (changé sur un autre appareil)")
	s.afterRotationStart()
	return nil
}

// startRotationLocked enregistre le changement de mot de passe (verrou b.mu tenu).
func (s *Service) startRotationLocked(old, password string) error {
	b := s.backups
	b.st.Rotation = &backup.Rotation{Old: old, Started: s.now().UnixMilli()}
	b.st.Password = password
	b.stale, b.checked, b.errGH, b.rotateErr, b.rotateAt = false, "", "", "", time.Time{}
	return b.saveLocked()
}

func (s *Service) afterRotationStart() {
	s.forgetArchiveKeys()
	s.notify()
	go func() {
		if err := s.runRotation(context.Background()); err != nil {
			diag.Warn(areaBackup, "changement du mot de passe interrompu, reprise automatique : %v", err)
		}
	}()
}

// rotationPending indique un changement de mot de passe à terminer, ou un nouveau mot de passe
// attendu : sauvegardes et archives attendent.
func (s *Service) rotationPending() bool {
	b := s.backups
	if b == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.st.Rotation != nil || b.stale
}

// rotationDue indique qu'un changement interrompu doit être repris maintenant.
func (s *Service) rotationDue() bool {
	b := s.backups
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.st.Rotation != nil && b.st.Password != "" && !b.rotating && (b.rotateErr == "" || s.now().Sub(b.rotateAt) >= rotateRetry)
}

// runRotation poursuit le changement du mot de passe (étapes déjà faites sautées).
func (s *Service) runRotation(ctx context.Context) (err error) {
	b := s.backups
	b.mu.Lock()
	r := b.st.Rotation
	if r == nil || b.rotating || b.st.Password == "" {
		b.mu.Unlock()
		return nil
	}
	b.rotating, b.rotateErr = true, ""
	old, password := r.Old, b.st.Password
	done := maps.Clone(r.Done)
	if done == nil {
		done = map[string]bool{}
	}
	var token, gist string
	if gh := b.st.GitHub; gh != nil {
		token, gist = gh.Token, gh.Gist
	}
	folder, legacy, device := b.st.Folder, b.st.LegacyDevice, b.st.Device
	b.mu.Unlock()
	s.notify()
	defer func() {
		b.mu.Lock()
		b.rotating, b.rotateStep, b.rotateAt = false, "", s.now()
		if err != nil {
			b.rotateErr = "Changement du mot de passe interrompu, reprise automatique : " + shareErrorText(err)
		}
		b.mu.Unlock()
		s.notify()
	}()
	ctx, cancel := context.WithTimeout(ctx, rotateTimeout)
	defer cancel()

	step := func(text string) {
		b.mu.Lock()
		b.rotateStep = text
		b.mu.Unlock()
		s.notify()
	}
	markDone := func(keys ...string) error {
		b.mu.Lock()
		defer b.mu.Unlock()
		if b.st.Rotation == nil {
			return errors.New("changement du mot de passe annulé")
		}
		if b.st.Rotation.Done == nil {
			b.st.Rotation.Done = map[string]bool{}
		}
		for _, k := range keys {
			b.st.Rotation.Done[k] = true
			done[k] = true
		}
		return b.saveLocked()
	}

	if token != "" && gist != "" && !done[backup.RotationBackups] {
		step("sauvegardes sur GitHub")
		if _, err := s.rebuildBackupGist(ctx, token, gist, old, password, legacy, device); err != nil {
			return err
		}
		if err := markDone(backup.RotationBackups); err != nil {
			return err
		}
	}
	if token != "" {
		step("archives des mesures")
		gh := b.opts.GitHub
		list, err := gh.ListGists(ctx, token, archive.DescriptionPrefix)
		if err != nil {
			return err
		}
		sort.Slice(list, func(i, j int) bool { return list[i].Description < list[j].Description })
		for _, g := range list {
			month := archive.MonthOf(g.Description)
			if month == "" || done[g.ID] {
				continue
			}
			step("archives de " + month)
			read, err := gh.ReadGistAll(ctx, token, g.ID, gistCopyLimit)
			if errors.Is(err, share.ErrNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			files := map[string]string{}
			for _, f := range read {
				files[f.Name] = f.Content
			}
			out, unreadable, err := archive.Reencrypt(files, old, password)
			if err != nil {
				// Autre mot de passe, déjà rechiffré, ou pas une archive : laissé tel quel.
				if !errors.Is(err, archive.ErrWrongPassword) {
					diag.Warn(areaBackup, "archives %s : Gist %s laissé tel quel (%v)", month, shortID(g.ID), err)
				}
				if err := markDone(g.ID); err != nil {
					return err
				}
				continue
			}
			if len(unreadable) > 0 {
				diag.Warn(areaBackup, "archives %s : %d fichier(s) abîmé(s) recopié(s) tel(s) quel(s)", month, len(unreadable))
			}
			newID, err := s.replaceGist(ctx, token, g.ID, g.Description, out)
			if err != nil {
				return err
			}
			b.mu.Lock()
			if a := b.st.Archive; a != nil {
				for m, id := range a.Gists {
					if id == g.ID {
						a.Gists[m] = newID
					}
				}
			}
			b.mu.Unlock()
			if err := markDone(g.ID, newID); err != nil {
				return err
			}
			diag.Info(areaBackup, "archives %s rechiffrées (Gist %s)", month, shortID(newID))
		}
	}
	if folder != "" && !done[backup.RotationFolder] {
		step("dossier des sauvegardes")
		if legacy != "" {
			if err := migrateFolder(folder, legacy, device); err != nil {
				return err
			}
		}
		names, err := backup.FolderBackups(folder)
		if err != nil {
			return err
		}
		count := 0
		for _, name := range names {
			content, err := os.ReadFile(filepath.Join(folder, name))
			if err != nil {
				return err
			}
			next, err := config.Reencrypt(content, old, password)
			if err != nil {
				continue // autre mot de passe ou déjà rechiffrée
			}
			if err := backup.ReplaceFile(folder, name, next); err != nil {
				return err
			}
			count++
		}
		diag.Info(areaBackup, "dossier : %d sauvegarde(s) rechiffrée(s)", count)
		if err := markDone(backup.RotationFolder); err != nil {
			return err
		}
	}

	b.mu.Lock()
	b.st.Rotation = nil
	b.rotated = s.now().UnixMilli()
	if legacy != "" && b.st.LegacyDevice == legacy && token != "" {
		b.st.LegacyDevice = "" // fichiers renommés avec la recopie du Gist et du dossier
	}
	// Archives : tout ce que les agents gardent est fusionné de nouveau (rien de ce qu'un autre appareil
	// aurait écrit pendant la recopie n'est perdu).
	b.st.Archive.ResetProgress()
	b.dirty, b.changed = true, time.Time{}
	err = b.saveLocked()
	b.mu.Unlock()
	if err != nil {
		return err
	}
	s.forgetArchiveKeys()
	diag.Info(areaBackup, "changement du mot de passe des sauvegardes terminé")
	b.poke()
	if a := s.archives; a != nil {
		a.poke()
	}
	return nil
}

// rotationView : état du changement de mot de passe (verrou b.mu tenu).
func (b *backups) rotationView() *RotationView {
	if b.st.Rotation == nil {
		return nil
	}
	return &RotationView{Running: b.rotating, Step: b.rotateStep, Error: b.rotateErr}
}

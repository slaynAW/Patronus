package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/slaynaw/wakeonlan/desktop/internal/model"
	"github.com/slaynaw/wakeonlan/desktop/internal/share"
)

// Partage des PC entre personnes (voir docs/PARTAGE.md).

// ShareOptions active le partage (nil : désactivé).
type ShareOptions struct {
	Store  *share.Store
	GitHub *share.GitHub
}

const (
	// Rythme de vérification des partages reçus.
	shareSyncPending  = 30 * time.Second // demande envoyée il y a moins de sharePendingFast
	sharePendingFast  = 30 * time.Minute
	shareSyncWaiting  = 5 * time.Minute // demande plus ancienne, ou accès retiré
	shareSyncActive   = 10 * time.Minute
	shareTick         = 15 * time.Second
	sharePublishRetry = 2 * time.Minute
	shareTimeout      = 30 * time.Second
	shareGistNote     = "# Wake On LAN – partage chiffré\n\nFichiers d'accès chiffrés de l'application Wake On LAN " +
		"(https://github.com/slaynAW/WakeOnLan). Chacun n'est lisible que par l'appareil auquel il est destiné.\n"
)

type shareLogin struct {
	code   string
	uri    string
	err    string
	cancel context.CancelFunc
}

type sharer struct {
	store *share.Store
	gh    *share.GitHub
	wake  chan struct{}

	mu          sync.Mutex
	state       share.State
	loadErr     string
	login       *shareLogin
	dirty       bool
	publishing  bool
	publishErr  string
	lastPublish time.Time
	syncErr     map[string]string
	lastSync    map[string]time.Time
}

func newSharer(opts *ShareOptions) *sharer {
	if opts == nil || opts.Store == nil || opts.GitHub == nil {
		return nil
	}
	sh := &sharer{
		store: opts.Store, gh: opts.GitHub, wake: make(chan struct{}, 1),
		dirty: true, syncErr: map[string]string{}, lastSync: map[string]time.Time{},
	}
	st, err := opts.Store.Load()
	if err != nil {
		sh.loadErr = err.Error()
	}
	sh.state = st
	return sh
}

func (sh *sharer) poke() {
	select {
	case sh.wake <- struct{}{}:
	default:
	}
}

// saveLocked enregistre l'état (verrou sh.mu tenu).
func (sh *sharer) saveLocked() error { return sh.store.Save(sh.state) }

// --- PC reçus, vus par le reste de l'application ---

// sharedDevice est un PC reçu d'une autre personne, avec son identifiant local.
type sharedDevice struct {
	device    model.Device
	owner     string
	ownerName string
}

// sharedDevices renvoie les PC reçus (accès actifs), identifiants locaux compris.
func (s *Service) sharedDevices() []sharedDevice {
	sh := s.sharing
	if sh == nil {
		return nil
	}
	sh.mu.Lock()
	defer sh.mu.Unlock()
	var out []sharedDevice
	for _, a := range sh.state.Received {
		if !a.Active {
			continue
		}
		for _, d := range a.Devices {
			d = d.Clone()
			d.ID = share.SharedID(a.Owner, d.ID)
			out = append(out, sharedDevice{device: d, owner: a.Owner, ownerName: a.OwnerName})
		}
	}
	return out
}

// allDevices renvoie les PC de l'appareil puis les PC reçus.
func (s *Service) allDevices() []model.Device {
	shared := s.sharedDevices()
	s.mu.Lock()
	out := slices.Clone(s.cfg.Devices)
	s.mu.Unlock()
	for _, d := range shared {
		out = append(out, d.device)
	}
	return out
}

// sharedChanged met à jour la surveillance et l'historique après un changement des PC reçus.
func (s *Service) sharedChanged() {
	ids := map[string]bool{}
	for _, d := range s.allDevices() {
		ids[d.ID] = true
	}
	s.keepHistory(ids)
	s.updateMonitor()
	s.notify()
}

// markShareDirty demande la republication (PC de l'appareil modifiés).
func (s *Service) markShareDirty() {
	if sh := s.sharing; sh != nil {
		sh.mu.Lock()
		sh.dirty = true
		sh.mu.Unlock()
		sh.poke()
	}
}

// --- Boucle de fond : publication et vérification des partages reçus ---

func (s *Service) runShare(ctx context.Context) {
	sh := s.sharing
	if sh == nil {
		return
	}
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		case <-sh.wake:
		}
		s.shareTickOnce(ctx)
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(shareTick)
	}
}

func (s *Service) shareTickOnce(ctx context.Context) {
	sh := s.sharing
	now := s.now()
	sh.mu.Lock()
	o := sh.state.Owner
	publish := o != nil && o.Token != "" && o.Gist != "" && !sh.publishing &&
		(sh.dirty || len(o.Withdrawn) > 0) && (sh.publishErr == "" || now.Sub(sh.lastPublish) >= sharePublishRetry)
	var due []string
	for _, a := range sh.state.Received {
		interval := shareSyncActive
		switch {
		case !a.Active && !a.Removed && now.Sub(time.UnixMilli(a.Requested)) < sharePendingFast:
			interval = shareSyncPending
		case !a.Active:
			interval = shareSyncWaiting
		}
		if last, ok := sh.lastSync[a.Owner]; !ok || now.Sub(last) >= interval {
			due = append(due, a.Owner)
		}
	}
	sh.mu.Unlock()
	if publish {
		_ = s.publishShares(ctx)
	}
	for _, owner := range due {
		s.syncAccess(ctx, owner)
	}
}

// publishShares publie les fichiers d'accès qui ont changé.
func (s *Service) publishShares(ctx context.Context) error {
	sh := s.sharing
	sh.mu.Lock()
	if sh.state.Owner == nil || sh.state.Owner.Token == "" || sh.state.Owner.Gist == "" {
		sh.mu.Unlock()
		return nil
	}
	owner := sh.state.Clone().Owner
	sh.publishing, sh.dirty = true, false
	sh.mu.Unlock()
	s.notify()

	s.mu.Lock()
	devices := slices.Clone(s.cfg.Devices)
	s.mu.Unlock()
	pub, err := owner.Prepare(devices, s.now(), false)
	if err == nil && !pub.Empty() {
		ctx, cancel := context.WithTimeout(ctx, shareTimeout)
		err = sh.gh.UpdateGist(ctx, owner.Token, owner.Gist, pub.Files)
		cancel()
	}
	sh.mu.Lock()
	sh.publishing, sh.lastPublish = false, s.now()
	switch {
	case err == nil:
		sh.publishErr = ""
		if o := sh.state.Owner; o != nil && o.Key == owner.Key && !pub.Empty() {
			o.Commit(pub)
			err = sh.saveLocked()
		}
	case errors.Is(err, share.ErrNotFound):
		sh.publishErr = "Espace de partage introuvable sur GitHub (Gist supprimé ?) : arrêtez puis réactivez le partage."
	default:
		sh.publishErr = err.Error()
		sh.dirty = true
	}
	sh.mu.Unlock()
	s.notify()
	return err
}

// syncAccess lit le Gist d'un partage reçu et applique son contenu.
func (s *Service) syncAccess(ctx context.Context, owner string) {
	sh := s.sharing
	sh.mu.Lock()
	i := sh.state.Find(owner)
	if i < 0 || sh.state.DeviceKey == "" {
		sh.mu.Unlock()
		return
	}
	access := sh.state.Received[i]
	access.Devices = slices.Clone(access.Devices)
	deviceKey := sh.state.DeviceKey
	sh.lastSync[owner] = s.now()
	sh.mu.Unlock()

	devicePriv, err := share.ParseDevicePrivate(deviceKey)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, shareTimeout)
	snap, err := sh.gh.FetchGist(ctx, access.Gist, access.ETag)
	cancel()
	result := share.Unchanged
	switch {
	case errors.Is(err, share.ErrNotFound):
		// Gist supprimé : même effet qu'un accès retiré.
		result, err = access.Apply(map[string]string{}, share.DevicePublic(devicePriv), nil, s.now())
		access.ETag = ""
	case err != nil:
	case snap.NotModified:
		access.Synced = s.now().UnixMilli()
	default:
		access.ETag = snap.ETag
		result, err = access.Apply(snap.Files, share.DevicePublic(devicePriv),
			func() (string, error) { return deviceKey, nil }, s.now())
	}

	sh.mu.Lock()
	if err != nil {
		sh.syncErr[owner] = shareErrorText(err)
	} else {
		delete(sh.syncErr, owner)
	}
	if j := sh.state.Find(owner); j >= 0 {
		// Seul le contenu reçu change ; le reste (nom de la demande…) a pu être modifié entre-temps.
		cur := &sh.state.Received[j]
		cur.Active, cur.Removed, cur.Revision, cur.Devices = access.Active, access.Removed, access.Revision, access.Devices
		cur.OwnerName, cur.Synced, cur.ETag = access.OwnerName, access.Synced, access.ETag
		_ = sh.saveLocked()
	}
	sh.mu.Unlock()
	if result != share.Unchanged {
		s.sharedChanged()
	} else {
		s.notify()
	}
}

func shareErrorText(err error) string {
	switch {
	case errors.Is(err, share.ErrSignature):
		return "Fichier refusé : il n'est pas signé par la personne qui partage."
	case errors.Is(err, share.ErrOlder):
		return "Fichier refusé : version plus ancienne que celle déjà reçue."
	case errors.Is(err, share.ErrRecipient):
		return "Fichier refusé : il est destiné à un autre appareil."
	case errors.Is(err, share.ErrInvalid):
		return "Fichier d'accès invalide : " + err.Error()
	}
	return err.Error()
}

// --- Actions : je partage mes PC ---

func (s *Service) requireSharing() (*sharer, error) {
	if s.sharing == nil {
		return nil, errors.New("partage indisponible")
	}
	return s.sharing, nil
}

// shareLogin démarre (ou reprend) la connexion à GitHub de la personne qui partage.
func (s *Service) shareLogin(name string) (any, error) {
	sh, err := s.requireSharing()
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	sh.mu.Lock()
	if name == "" && sh.state.Owner != nil {
		name = sh.state.Owner.Name
	}
	if sh.login != nil && sh.login.cancel != nil {
		sh.login.cancel()
	}
	sh.login = nil
	sh.mu.Unlock()
	if !share.ValidName(name) {
		return nil, fmt.Errorf("Nom invalide (1 à %d caractères)", share.MaxNameLength)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	dc, err := sh.gh.StartLogin(ctx)
	if err != nil {
		cancel()
		return nil, err
	}
	login := &shareLogin{code: dc.UserCode, uri: dc.VerificationURI, cancel: cancel}
	sh.mu.Lock()
	sh.login = login
	sh.mu.Unlock()
	s.notify()
	go s.finishLogin(ctx, login, dc, name)
	return map[string]any{"code": dc.UserCode, "uri": dc.VerificationURI}, nil
}

func (s *Service) finishLogin(ctx context.Context, login *shareLogin, dc share.DeviceCode, name string) {
	sh := s.sharing
	defer login.cancel()
	fail := func(err error) {
		sh.mu.Lock()
		if sh.login == login {
			login.err = err.Error()
		}
		sh.mu.Unlock()
		s.notify()
	}
	token, err := sh.gh.WaitLogin(ctx, dc)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			fail(err)
		}
		return
	}
	user, err := sh.gh.User(ctx, token)
	if err != nil {
		fail(err)
		return
	}
	sh.mu.Lock()
	o := sh.state.Owner
	if o != nil && o.Gist != "" && o.User != "" && !strings.EqualFold(o.User, user) {
		sh.mu.Unlock()
		fail(fmt.Errorf("Connectez-vous avec le compte GitHub @%s, qui contient vos partages (compte utilisé : @%s)", o.User, user))
		return
	}
	var gist string
	if o != nil {
		gist = o.Gist
	}
	sh.mu.Unlock()
	if gist == "" {
		gist, err = sh.gh.CreateGist(ctx, token, "Wake On LAN – partage chiffré", map[string]string{"LISEZMOI.md": shareGistNote})
		if err != nil {
			fail(err)
			return
		}
	}
	sh.mu.Lock()
	if sh.login != login {
		sh.mu.Unlock()
		return
	}
	if sh.state.Owner == nil {
		key, err := share.NewOwnerKey()
		if err != nil {
			sh.mu.Unlock()
			fail(err)
			return
		}
		sh.state.Owner = &share.Owner{Key: share.EncodePrivate(share.OwnerPrivateBytes(key)), People: []share.Person{}}
	}
	o = sh.state.Owner
	o.Name, o.Token, o.User, o.Gist = name, token, user, gist
	sh.login, sh.dirty, sh.publishErr = nil, true, ""
	err = sh.saveLocked()
	sh.mu.Unlock()
	if err != nil {
		fail(err)
		return
	}
	sh.poke()
	s.notify()
}

func (s *Service) shareCancelLogin() {
	if sh := s.sharing; sh != nil {
		sh.mu.Lock()
		if sh.login != nil {
			sh.login.cancel()
			sh.login = nil
		}
		sh.mu.Unlock()
		s.notify()
	}
}

// ownerPublic renvoie la clé publique de la personne qui partage (verrou sh.mu tenu).
func ownerPublicLocked(sh *sharer) (string, error) {
	o := sh.state.Owner
	if o == nil {
		return "", errors.New("le partage n'est pas activé")
	}
	key, err := share.ParseOwnerPrivate(o.Key)
	if err != nil {
		return "", err
	}
	return share.OwnerPublic(key), nil
}

// shareInvite renvoie le lien et le QR code d'invitation.
func (s *Service) shareInvite() (any, error) {
	sh, err := s.requireSharing()
	if err != nil {
		return nil, err
	}
	sh.mu.Lock()
	pub, err := ownerPublicLocked(sh)
	var inv share.Invite
	if err == nil {
		o := sh.state.Owner
		if o.Gist == "" || o.User == "" {
			err = errors.New("connectez-vous d'abord à GitHub")
		}
		inv = share.Invite{Name: o.Name, Owner: pub, User: o.User, Gist: o.Gist}
	}
	sh.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return linkView(inv.Link(), "")
}

func linkView(link, code string) (map[string]any, error) {
	svg, err := share.QRCodeSVG(link)
	if err != nil {
		return nil, err
	}
	v := map[string]any{"link": link, "qr": svg}
	if code != "" {
		v["code"] = code
	}
	return v, nil
}

// ownDevicesBrief résume les PC de l'appareil pour le choix des droits.
func (s *Service) ownDevicesBrief() []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []map[string]any{}
	for _, d := range s.cfg.Devices {
		out = append(out, map[string]any{"id": d.ID, "name": d.Name, "canShutdown": d.CanShutdown() && d.Agent.HasKey()})
	}
	return out
}

// shareReadRequest lit la demande d'accès d'une personne.
func (s *Service) shareReadRequest(text string) (any, error) {
	sh, err := s.requireSharing()
	if err != nil {
		return nil, err
	}
	if share.Kind(text) == "invite" {
		return map[string]any{"ok": false, "error": "C'est un lien d'invitation : la personne invitée doit l'ouvrir dans son application, puis vous renvoyer sa demande."}, nil
	}
	req, err := share.ParseRequest(text)
	if err != nil {
		return map[string]any{"ok": false, "error": "Ce n'est pas une demande d'accès valide."}, nil
	}
	sh.mu.Lock()
	pub, err := ownerPublicLocked(sh)
	var existing map[string]share.Right
	if err == nil {
		if i := sh.state.Owner.Person(req.Device); i >= 0 {
			existing = sh.state.Owner.People[i].Rights
		}
	}
	sh.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if req.Owner != share.KeyID(pub) {
		return map[string]any{"ok": false, "error": "Cette demande répond à l'invitation d'une autre personne."}, nil
	}
	return map[string]any{
		"ok": true, "name": req.Name, "device": req.Device,
		"code":    share.VerificationCode(pub, req.Device),
		"devices": s.ownDevicesBrief(), "rights": existing,
	}, nil
}

// shareGrant autorise une personne (ou modifie ses droits).
func (s *Service) shareGrant(device, name string, rights map[string]string) (any, error) {
	sh, err := s.requireSharing()
	if err != nil {
		return nil, err
	}
	if !share.ValidDeviceKey(device) || !share.ValidName(name) {
		return nil, errors.New("demande invalide")
	}
	s.mu.Lock()
	granted := map[string]share.Right{}
	for id, r := range rights {
		right := share.Right(r)
		d, ok := s.cfg.Device(id)
		if !ok || !right.Valid() {
			continue
		}
		if right == share.RightFull && !(d.CanShutdown() && d.Agent.HasKey()) {
			right = share.RightWake
		}
		granted[id] = right
	}
	s.mu.Unlock()
	if len(granted) == 0 {
		return nil, errors.New("Choisissez au moins un PC à partager")
	}
	sh.mu.Lock()
	o := sh.state.Owner
	if o == nil {
		sh.mu.Unlock()
		return nil, errors.New("le partage n'est pas activé")
	}
	err = o.Grant(share.Person{Name: name, Device: device, Added: s.now().UnixMilli(), Rights: granted})
	if err == nil {
		sh.dirty = true
		err = sh.saveLocked()
	}
	sh.mu.Unlock()
	if err != nil {
		return nil, err
	}
	sh.poke()
	s.notify()
	return map[string]any{"ok": true}, nil
}

// shareRevoke retire l'accès d'une personne.
func (s *Service) shareRevoke(device string) error {
	sh, err := s.requireSharing()
	if err != nil {
		return err
	}
	sh.mu.Lock()
	if o := sh.state.Owner; o != nil && o.Withdraw(device) {
		sh.dirty = true
		err = sh.saveLocked()
	}
	sh.mu.Unlock()
	sh.poke()
	s.notify()
	return err
}

// shareStop arrête de partager : le Gist est supprimé (tous les accès disparaissent) et la clé oubliée.
func (s *Service) shareStop() error {
	sh, err := s.requireSharing()
	if err != nil {
		return err
	}
	sh.mu.Lock()
	o := sh.state.Owner
	sh.mu.Unlock()
	if o == nil {
		return nil
	}
	if o.Token != "" && o.Gist != "" {
		ctx, cancel := context.WithTimeout(context.Background(), shareTimeout)
		err := sh.gh.DeleteGist(ctx, o.Token, o.Gist)
		cancel()
		if err != nil && !errors.Is(err, share.ErrNotFound) {
			return fmt.Errorf("Impossible de supprimer l'espace de partage : %w", err)
		}
	}
	sh.mu.Lock()
	sh.state.Owner = nil
	sh.publishErr = ""
	err = sh.saveLocked()
	sh.mu.Unlock()
	s.notify()
	return err
}

// --- Actions : je reçois les PC d'une autre personne ---

// shareReadInvite lit une invitation (avant de demander l'accès).
func (s *Service) shareReadInvite(text string) (any, error) {
	sh, err := s.requireSharing()
	if err != nil {
		return nil, err
	}
	if share.Kind(text) == "request" {
		return map[string]any{"ok": false, "error": "C'est une demande d'accès : c'est la personne qui partage qui doit l'ajouter."}, nil
	}
	inv, err := share.ParseInvite(text)
	if err != nil {
		return map[string]any{"ok": false, "error": "Ce n'est pas une invitation valide."}, nil
	}
	sh.mu.Lock()
	mine, _ := ownerPublicLocked(sh)
	existing := sh.state.Find(inv.Owner) >= 0
	sh.mu.Unlock()
	if inv.Owner == mine {
		return map[string]any{"ok": false, "error": "C'est votre propre invitation."}, nil
	}
	return map[string]any{"ok": true, "name": inv.Name, "user": inv.User, "existing": existing}, nil
}

// shareRequest enregistre la demande d'accès et renvoie le lien et le QR code à transmettre.
func (s *Service) shareRequest(text, myName string) (any, error) {
	sh, err := s.requireSharing()
	if err != nil {
		return nil, err
	}
	inv, err := share.ParseInvite(text)
	if err != nil {
		return nil, errors.New("Invitation invalide")
	}
	myName = strings.TrimSpace(myName)
	if !share.ValidName(myName) {
		return nil, fmt.Errorf("Indiquez votre nom (1 à %d caractères)", share.MaxNameLength)
	}
	sh.mu.Lock()
	if sh.state.DeviceKey == "" {
		key, err := share.NewDeviceKey()
		if err != nil {
			sh.mu.Unlock()
			return nil, err
		}
		sh.state.DeviceKey = share.EncodePrivate(key.Bytes())
	}
	if i := sh.state.Find(inv.Owner); i >= 0 {
		a := &sh.state.Received[i]
		a.MyName, a.OwnerName, a.User, a.Gist = myName, inv.Name, inv.User, inv.Gist
		if !a.Active {
			a.Requested = s.now().UnixMilli()
		}
	} else {
		sh.state.Received = append(sh.state.Received, share.Access{
			Owner: inv.Owner, OwnerName: inv.Name, User: inv.User, Gist: inv.Gist, MyName: myName,
			Requested: s.now().UnixMilli(), Devices: []model.Device{},
		})
	}
	delete(sh.lastSync, inv.Owner)
	err = sh.saveLocked()
	sh.mu.Unlock()
	if err != nil {
		return nil, err
	}
	sh.poke()
	s.notify()
	return s.shareRequestView(inv.Owner)
}

// shareRequestView renvoie le lien, le QR code et le code de vérification d'une demande.
func (s *Service) shareRequestView(owner string) (map[string]any, error) {
	sh, err := s.requireSharing()
	if err != nil {
		return nil, err
	}
	sh.mu.Lock()
	i := sh.state.Find(owner)
	var req share.Request
	var devicePub, ownerName string
	if i >= 0 && sh.state.DeviceKey != "" {
		if key, err := share.ParseDevicePrivate(sh.state.DeviceKey); err == nil {
			devicePub = share.DevicePublic(key)
			a := sh.state.Received[i]
			req = share.Request{Name: a.MyName, Device: devicePub, Owner: share.KeyID(owner)}
			ownerName = a.OwnerName
		}
	}
	sh.mu.Unlock()
	if devicePub == "" {
		return nil, errors.New("demande introuvable")
	}
	v, err := linkView(req.Link(), share.VerificationCode(owner, devicePub))
	if err != nil {
		return nil, err
	}
	v["ownerName"] = ownerName
	v["owner"] = owner
	return v, nil
}

// shareLeave supprime un partage reçu (ou une demande) de cet appareil.
func (s *Service) shareLeave(owner string) error {
	sh, err := s.requireSharing()
	if err != nil {
		return err
	}
	sh.mu.Lock()
	if i := sh.state.Find(owner); i >= 0 {
		sh.state.Received = slices.Delete(sh.state.Received, i, i+1)
		delete(sh.syncErr, owner)
		delete(sh.lastSync, owner)
		err = sh.saveLocked()
	}
	sh.mu.Unlock()
	s.sharedChanged()
	return err
}

// shareSyncNow vérifie immédiatement les partages reçus.
func (s *Service) shareSyncNow() {
	if sh := s.sharing; sh != nil {
		sh.mu.Lock()
		clear(sh.lastSync)
		sh.mu.Unlock()
		sh.poke()
	}
}

// --- État affiché ---

// ShareView est l'état du partage pour l'interface (sans aucune clé privée ni jeton).
type ShareView struct {
	Available bool         `json:"available"`
	CanLogin  bool         `json:"canLogin"`
	Error     string       `json:"error,omitempty"`
	Login     *LoginView   `json:"login,omitempty"`
	Owner     *OwnerView   `json:"owner,omitempty"`
	Received  []AccessView `json:"received"`
}

// LoginView est une connexion GitHub en cours.
type LoginView struct {
	Code  string `json:"code"`
	URI   string `json:"uri"`
	Error string `json:"error,omitempty"`
}

// OwnerView décrit le côté « je partage ».
type OwnerView struct {
	Name       string       `json:"name"`
	User       string       `json:"user"`
	Connected  bool         `json:"connected"`
	Publishing bool         `json:"publishing"`
	Error      string       `json:"error,omitempty"`
	People     []PersonView `json:"people"`
}

// PersonView est une personne autorisée.
type PersonView struct {
	Name      string                 `json:"name"`
	Device    string                 `json:"device"`
	Added     int64                  `json:"added"`
	Rights    map[string]share.Right `json:"rights"`
	Devices   []string               `json:"devices"`
	Published bool                   `json:"published"`
}

// AccessView est un partage reçu ou demandé.
type AccessView struct {
	Owner     string   `json:"owner"`
	OwnerName string   `json:"ownerName"`
	User      string   `json:"user"`
	MyName    string   `json:"myName"`
	Active    bool     `json:"active"`
	Removed   bool     `json:"removed"`
	Devices   []string `json:"devices"`
	Requested int64    `json:"requested"`
	Synced    int64    `json:"synced"`
	Error     string   `json:"error,omitempty"`
}

func (s *Service) shareView() ShareView {
	sh := s.sharing
	if sh == nil {
		return ShareView{Received: []AccessView{}}
	}
	s.mu.Lock()
	names := map[string]string{}
	for _, d := range s.cfg.Devices {
		names[d.ID] = d.Name
	}
	order := make([]string, 0, len(s.cfg.Devices))
	for _, d := range s.cfg.Devices {
		order = append(order, d.ID)
	}
	s.mu.Unlock()

	sh.mu.Lock()
	defer sh.mu.Unlock()
	v := ShareView{Available: true, CanLogin: sh.gh.ClientID != "", Error: sh.loadErr, Received: []AccessView{}}
	if l := sh.login; l != nil {
		v.Login = &LoginView{Code: l.code, URI: l.uri, Error: l.err}
	}
	if o := sh.state.Owner; o != nil {
		ov := &OwnerView{Name: o.Name, User: o.User, Connected: o.Token != "" && o.Gist != "",
			Publishing: sh.publishing, Error: sh.publishErr, People: []PersonView{}}
		for _, p := range o.People {
			pv := PersonView{Name: p.Name, Device: p.Device, Added: p.Added, Rights: p.Rights, Devices: []string{}}
			for _, id := range order {
				if p.Rights[id].Valid() {
					pv.Devices = append(pv.Devices, names[id])
				}
			}
			_, pv.Published = o.Published[p.Device]
			ov.People = append(ov.People, pv)
		}
		v.Owner = ov
	}
	for _, a := range sh.state.Received {
		av := AccessView{Owner: a.Owner, OwnerName: a.OwnerName, User: a.User, MyName: a.MyName,
			Active: a.Active, Removed: a.Removed, Requested: a.Requested, Synced: a.Synced,
			Error: sh.syncErr[a.Owner], Devices: []string{}}
		for _, d := range a.Devices {
			av.Devices = append(av.Devices, d.Name)
		}
		v.Received = append(v.Received, av)
	}
	return v
}

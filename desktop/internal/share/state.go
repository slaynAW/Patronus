package share

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/slaynaw/wakeonlan/desktop/internal/model"
)

// État du partage conservé sur l'appareil (chiffré au repos, voir Store).

// Right est le droit accordé sur un PC partagé.
type Right string

const (
	// RightWake : démarrer et voir l'état (la clé de l'agent n'est pas transmise).
	RightWake Right = "wake"
	// RightFull : démarrer, éteindre, redémarrer, mettre en veille.
	RightFull Right = "full"
)

// Valid indique un droit connu.
func (r Right) Valid() bool { return r == RightWake || r == RightFull }

// MaxPeople borne le nombre de personnes autorisées.
const MaxPeople = 50

// State regroupe les deux rôles d'un appareil : partager ses PC et recevoir ceux des autres.
type State struct {
	Version int `json:"version"`
	// Owner est présent quand l'appareil partage ses PC.
	Owner *Owner `json:"owner,omitempty"`
	// DeviceKey est la clé privée de réception de cet appareil (créée à la première demande).
	DeviceKey string `json:"deviceKey,omitempty"`
	// Received liste les partages demandés ou reçus.
	Received []Access `json:"received"`
}

// Owner est le côté « je partage mes PC ».
type Owner struct {
	// Key est la clé privée de signature.
	Key  string `json:"key"`
	Name string `json:"name"`
	// Token est le jeton GitHub (droit « gist ») ; jamais exporté.
	Token string `json:"token,omitempty"`
	User  string `json:"user,omitempty"`
	Gist  string `json:"gist,omitempty"`
	// Revision est la dernière révision publiée.
	Revision int64    `json:"revision"`
	People   []Person `json:"people"`
	// Published associe chaque appareil autorisé à l'empreinte du dernier contenu publié pour lui.
	Published map[string]string `json:"published,omitempty"`
	// Withdrawn liste les clés des appareils dont le fichier reste à supprimer du Gist.
	Withdrawn []string `json:"withdrawn,omitempty"`
}

// Person est une personne (un appareil) autorisée.
type Person struct {
	Name string `json:"name"`
	// Device est la clé publique de réception de son appareil.
	Device string `json:"device"`
	Added  int64  `json:"added"`
	// Rights associe l'identifiant de chaque PC partagé au droit accordé.
	Rights map[string]Right `json:"rights"`
}

// Access est un partage reçu (ou demandé) d'une autre personne.
type Access struct {
	// Owner est la clé publique de signature de la personne qui partage (épinglée à l'invitation).
	Owner     string `json:"owner"`
	OwnerName string `json:"ownerName"`
	User      string `json:"user"`
	Gist      string `json:"gist"`
	// MyName est le nom indiqué dans la demande.
	MyName string `json:"myName"`
	// Active : accès accordé ; sinon demande en attente (ou accès retiré, voir Removed).
	Active    bool           `json:"active"`
	Removed   bool           `json:"removed,omitempty"`
	Revision  int64          `json:"revision"`
	Devices   []model.Device `json:"devices"`
	Requested int64          `json:"requested"`
	// Synced est l'heure de la dernière vérification réussie ; ETag évite de relire un Gist inchangé.
	Synced int64  `json:"synced,omitempty"`
	ETag   string `json:"etag,omitempty"`
}

const stateVersion = 1

// NewState renvoie un état vide.
func NewState() State { return State{Version: stateVersion, Received: []Access{}} }

// Clone renvoie une copie indépendante.
func (s State) Clone() State {
	data, _ := json.Marshal(s)
	var c State
	_ = json.Unmarshal(data, &c)
	if c.Received == nil {
		c.Received = []Access{}
	}
	return c
}

// --- Côté « je partage » ---

// Person renvoie l'index d'une personne (clé de réception), -1 si absente.
func (o *Owner) Person(device string) int {
	return slices.IndexFunc(o.People, func(p Person) bool { return p.Device == device })
}

// Content construit ce que reçoit une personne : ses PC autorisés, dans l'ordre de la configuration,
// sans la clé de l'agent pour un accès « démarrer ».
func (o *Owner) Content(p Person, devices []model.Device) Content {
	c := Content{OwnerName: o.Name, RecipientName: p.Name, Devices: []model.Device{}}
	for _, d := range devices {
		right, ok := p.Rights[d.ID]
		if !ok || !right.Valid() {
			continue
		}
		d = d.Clone()
		if right == RightWake {
			d.Agent = nil
		}
		c.Devices = append(c.Devices, d)
	}
	return c
}

// Fingerprint est l'empreinte d'un contenu (pour ne republier que ce qui a changé).
func Fingerprint(c Content) string {
	data, _ := json.Marshal(c)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Publication est un lot de fichiers à écrire dans le Gist.
type Publication struct {
	// Files : contenu à écrire, ou nil pour supprimer.
	Files map[string]*string
	// Fingerprints : empreintes à enregistrer une fois la publication réussie.
	Fingerprints map[string]string
	Revision     int64
}

// Empty indique qu'il n'y a rien à publier.
func (p Publication) Empty() bool { return len(p.Files) == 0 }

// Prepare prépare la publication des fichiers qui ont changé (ou de tous si all).
func (o *Owner) Prepare(devices []model.Device, now time.Time, all bool) (Publication, error) {
	key, err := ParseOwnerPrivate(o.Key)
	if err != nil {
		return Publication{}, err
	}
	pub := Publication{Files: map[string]*string{}, Fingerprints: map[string]string{}}
	revision := max(o.Revision+1, now.UnixMilli())
	for _, p := range o.People {
		c := o.Content(p, devices)
		fp := Fingerprint(c)
		pub.Fingerprints[p.Device] = fp
		if !all && o.Published[p.Device] == fp {
			continue
		}
		data, err := Seal(key, p.Device, revision, now.UTC().Format(time.RFC3339), c)
		if err != nil {
			return Publication{}, err
		}
		text := string(data)
		pub.Files[FileName(p.Device)] = &text
	}
	for _, device := range o.Withdrawn {
		if o.Person(device) < 0 {
			pub.Files[FileName(device)] = nil
		}
	}
	if len(pub.Files) > 0 {
		pub.Revision = revision
	}
	return pub, nil
}

// Commit enregistre une publication réussie. L'état a pu changer pendant l'envoi : seules les
// personnes toujours autorisées et les suppressions effectivement publiées sont prises en compte.
func (o *Owner) Commit(p Publication) {
	if p.Revision > o.Revision {
		o.Revision = p.Revision
	}
	if o.Published == nil {
		o.Published = map[string]string{}
	}
	for device, fp := range p.Fingerprints {
		if o.Person(device) >= 0 {
			o.Published[device] = fp
		}
	}
	o.Withdrawn = slices.DeleteFunc(o.Withdrawn, func(device string) bool {
		f, ok := p.Files[FileName(device)]
		return ok && f == nil
	})
}

// Grant autorise (ou met à jour) une personne ; son fichier sera publié à la prochaine publication.
func (o *Owner) Grant(p Person) error {
	if i := o.Person(p.Device); i >= 0 {
		p.Added = o.People[i].Added
		o.People[i] = p
	} else {
		if len(o.People) >= MaxPeople {
			return fmt.Errorf("%d personnes au maximum", MaxPeople)
		}
		o.People = append(o.People, p)
	}
	o.Withdrawn = slices.DeleteFunc(o.Withdrawn, func(d string) bool { return d == p.Device })
	delete(o.Published, p.Device)
	return nil
}

// Withdraw retire une personne ; son fichier sera supprimé à la prochaine publication.
func (o *Owner) Withdraw(device string) bool {
	i := o.Person(device)
	if i < 0 {
		return false
	}
	o.People = slices.Delete(o.People, i, i+1)
	if !slices.Contains(o.Withdrawn, device) {
		o.Withdrawn = append(o.Withdrawn, device)
	}
	delete(o.Published, device)
	return true
}

// --- Côté « je reçois » ---

// ErrUnknownInvite signale une invitation déjà utilisée ou absente.
var ErrUnknownInvite = errors.New("partage inconnu")

// Find renvoie l'index d'un partage reçu (clé de la personne qui partage), -1 si absent.
func (s *State) Find(owner string) int {
	return slices.IndexFunc(s.Received, func(a Access) bool { return a.Owner == owner })
}

// SyncResult est le résultat de la lecture d'un Gist pour un partage reçu.
type SyncResult int

const (
	Unchanged SyncResult = iota
	Updated
	Granted
	Withdrawn
)

// Apply applique le contenu d'un Gist à un partage reçu. files est nil si le Gist n'existe plus.
func (a *Access) Apply(files map[string]string, device string, key func() (string, error), now time.Time) (SyncResult, error) {
	a.Synced = now.UnixMilli()
	text, ok := files[FileName(device)]
	if !ok {
		if a.Active {
			a.Active, a.Removed, a.Devices = false, true, []model.Device{}
			return Withdrawn, nil
		}
		return Unchanged, nil
	}
	priv, err := key()
	if err != nil {
		return Unchanged, err
	}
	dk, err := ParseDevicePrivate(priv)
	if err != nil {
		return Unchanged, err
	}
	opened, err := Open([]byte(text), a.Owner, dk, a.Revision)
	if err != nil {
		return Unchanged, err
	}
	wasActive := a.Active
	changed := opened.Revision != a.Revision || !wasActive
	a.Active, a.Removed, a.Revision = true, false, opened.Revision
	a.OwnerName = opened.Content.OwnerName
	a.Devices = opened.Content.Devices
	switch {
	case !wasActive:
		return Granted, nil
	case changed:
		return Updated, nil
	}
	return Unchanged, nil
}

// SharedID est l'identifiant local d'un PC reçu : stable et distinct des PC de l'appareil.
func SharedID(owner, deviceID string) string {
	sum := sha256.Sum256([]byte(owner + "/" + deviceID))
	return "s-" + hex.EncodeToString(sum[:16])
}

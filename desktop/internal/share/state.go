package share

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol2"
	"github.com/slaynaw/wakeonlan/desktop/internal/model"
)

// État du partage conservé sur l'appareil (chiffré au repos, voir Store).

// Right est le droit accordé sur un PC partagé.
type Right string

const (
	// RightWake : démarrer et voir l'état. L'agent n'est pas joint : ni clé, ni enregistrement.
	RightWake Right = "wake"
	// RightView : démarrer et consulter (état, journal, mesures, fiche du PC). Partage v2 seulement :
	// l'agent enregistre l'appareil de la personne avec le rôle « wake ».
	RightView Right = "view"
	// RightFull : démarrer, éteindre, redémarrer, mettre en veille. Partage v2 : rôle « control » ;
	// partage v1 : la clé de l'agent est transmise.
	RightFull Right = "full"
)

// Valid indique un droit connu.
func (r Right) Valid() bool { return r == RightWake || r == RightView || r == RightFull }

// AgentRole renvoie le rôle wolagent/2 donné à l'appareil de la personne (partage v2), ou "" pour
// « démarrer », qui ne joint pas l'agent.
func (r Right) AgentRole() string {
	switch r {
	case RightView:
		return protocol2.RoleWake.String()
	case RightFull:
		return protocol2.RoleControl.String()
	}
	return ""
}

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
	// Pairings suit, par personne et par PC partagé en v2, l'invitation créée sur l'agent ou
	// l'enregistrement de son appareil (docs/PARTAGE.md, « Partage v2 »). Jamais exporté.
	Pairings []Pairing `json:"pairings,omitempty"`
	// Revocations liste les appareils à retirer des PC (accès retiré ou réduit) : à appliquer dès
	// que cet appareil joint le PC en administrateur. Gardée dans l'export complet.
	Revocations []Revocation `json:"revocations,omitempty"`
}

// Person est une personne (un appareil) autorisée.
type Person struct {
	Name string `json:"name"`
	// Device est la clé publique de réception de son appareil.
	Device string `json:"device"`
	// Identity est l'empreinte wolagent/2 de son appareil, reçue dans sa demande (paramètre « id ») :
	// vide pour une demande d'une application qui ne connaît que le partage v1.
	Identity string `json:"identity,omitempty"`
	Added    int64  `json:"added"`
	// Rights associe l'identifiant de chaque PC partagé au droit accordé.
	Rights map[string]Right `json:"rights"`
}

// Pairing est l'appairage wolagent/2 d'une personne sur un PC (partage v2).
type Pairing struct {
	// Person est la clé de réception de la personne (Person.Device) ; Device, l'identifiant du PC.
	Person string `json:"person"`
	Device string `json:"device"`
	// Identity est l'empreinte wolagent/2 de l'appareil de la personne ; Role, le rôle de l'invitation.
	Identity string `json:"identity"`
	Role     string `json:"role"`
	// Invite, Code et Expires (secondes Unix) décrivent l'invitation en attente. Le code est secret :
	// jamais exporté ni écrit dans les journaux.
	Invite  string `json:"invite,omitempty"`
	Code    string `json:"code,omitempty"`
	Expires int64  `json:"expires,omitempty"`
	// Registered : l'appareil de la personne figure dans le registre du PC.
	Registered bool `json:"registered,omitempty"`
}

// String masque le code de l'invitation.
func (p Pairing) String() string {
	code := ""
	if p.Code != "" {
		code = "***"
	}
	return fmt.Sprintf("Pairing(device=%s, role=%s, invite=%s, code=%s, expires=%d, registered=%t)",
		p.Device, p.Role, p.Invite, code, p.Expires, p.Registered)
}

// Revocation est un appareil à retirer d'un PC : invitations réservées à son empreinte à annuler,
// puis enregistrement à supprimer.
type Revocation struct {
	// Device est l'identifiant du PC ; Identity, l'empreinte wolagent/2 de l'appareil à retirer.
	Device   string `json:"device"`
	Identity string `json:"identity"`
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

// Content construit ce que reçoit une personne : ses PC autorisés, dans l'ordre de la configuration
// (règles : docs/PARTAGE.md, « Partage v2 ») :
//   - « démarrer » : le PC sans agent ;
//   - partage v2 (demande avec empreinte, agent joint par la sienne) : l'empreinte de l'agent, sans
//     clé, une fois l'invitation créée ou l'appareil enregistré, et l'invitation tant qu'elle est
//     valable ; avant, le PC sans agent ;
//   - partage v1 : la clé de l'agent pour « démarrer et éteindre », jamais son empreinte.
//
// now sert à écarter les invitations expirées.
func (o *Owner) Content(p Person, devices []model.Device, now time.Time) Content {
	c := Content{OwnerName: o.Name, RecipientName: p.Name, Identity: p.Identity, Devices: []model.Device{}}
	for _, d := range devices {
		right, ok := p.Rights[d.ID]
		if !ok || !right.Valid() {
			continue
		}
		d = d.Clone()
		switch {
		case right == RightWake || d.Agent == nil:
			d.Agent = nil
		case p.Identity != "" && d.Agent.HasPin():
			pairing := o.pairing(p, d.ID)
			if pairing == nil {
				d.Agent = nil
				break
			}
			d.Agent = &model.AgentSettings{Port: d.Agent.Port, Pin: d.Agent.Pin}
			if pairing.Code != "" && pairing.Expires > now.Unix() {
				c.Invites = append(c.Invites, AgentInvite{Device: d.ID, Code: pairing.Code, Role: pairing.Role, Expires: pairing.Expires})
			}
		case right == RightFull && d.CanShutdown() && d.Agent.HasKey():
			d.Agent = &model.AgentSettings{Port: d.Agent.Port, Key: d.Agent.Key}
		default:
			d.Agent = nil
		}
		c.Devices = append(c.Devices, d)
	}
	return c
}

// pairing renvoie l'appairage v2 de la personne sur un PC, pour son empreinte actuelle (nil sinon).
func (o *Owner) pairing(p Person, device string) *Pairing {
	i := slices.IndexFunc(o.Pairings, func(x Pairing) bool {
		return x.Person == p.Device && x.Device == device && x.Identity == p.Identity
	})
	if i < 0 {
		return nil
	}
	return &o.Pairings[i]
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
		c := o.Content(p, devices, now)
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
// Son appareil est retiré des PC qu'il ne doit plus joindre par l'agent : droit réduit à « démarrer »
// ou retiré, ou demande arrivée avec une autre empreinte.
func (o *Owner) Grant(p Person) error {
	if i := o.Person(p.Device); i >= 0 {
		old := o.People[i]
		p.Added = old.Added
		o.People[i] = p
		o.revokeUnwanted(old)
	} else {
		if len(o.People) >= MaxPeople {
			return fmt.Errorf("%d personnes au maximum", MaxPeople)
		}
		o.People = append(o.People, p)
	}
	// Un retrait en attente ne doit pas défaire un accès accordé de nouveau.
	o.Revocations = slices.DeleteFunc(o.Revocations, func(r Revocation) bool { return o.wants(r.Device, r.Identity) })
	o.Withdrawn = slices.DeleteFunc(o.Withdrawn, func(d string) bool { return d == p.Device })
	delete(o.Published, p.Device)
	return nil
}

// Withdraw retire une personne ; son fichier sera supprimé à la prochaine publication, et son
// appareil retiré des PC où il a pu être enregistré.
func (o *Owner) Withdraw(device string) bool {
	i := o.Person(device)
	if i < 0 {
		return false
	}
	old := o.People[i]
	o.People = slices.Delete(o.People, i, i+1)
	o.revokeUnwanted(old)
	if !slices.Contains(o.Withdrawn, device) {
		o.Withdrawn = append(o.Withdrawn, device)
	}
	delete(o.Published, device)
	return true
}

// wants indique qu'une personne autorisée doit joindre ce PC par l'agent avec cette empreinte.
func (o *Owner) wants(device, identity string) bool {
	return slices.ContainsFunc(o.People, func(p Person) bool {
		return p.Identity == identity && p.Rights[device].AgentRole() != ""
	})
}

// revokeUnwanted suit la modification (ou le retrait) d'une personne, dont old est l'état précédent :
// son appareil est mis en file de retrait des PC qu'il ne doit plus joindre, et ses appairages devenus
// inutiles sont oubliés. Un PC accordé en v2 est retiré même sans appairage connu (sauvegarde
// restaurée) : retirer un appareil absent ne change rien.
func (o *Owner) revokeUnwanted(old Person) {
	if old.Identity != "" {
		for device, right := range old.Rights {
			if right.AgentRole() != "" && !o.wants(device, old.Identity) {
				o.revoke(device, old.Identity)
			}
		}
	}
	var current *Person
	if i := o.Person(old.Device); i >= 0 {
		current = &o.People[i]
	}
	o.Pairings = slices.DeleteFunc(o.Pairings, func(x Pairing) bool {
		if x.Person != old.Device || (current != nil && current.Identity == x.Identity && current.Rights[x.Device].AgentRole() != "") {
			return false
		}
		if !o.wants(x.Device, x.Identity) {
			o.revoke(x.Device, x.Identity)
		}
		return true
	})
	slices.SortFunc(o.Revocations, func(a, b Revocation) int {
		return cmp.Or(cmp.Compare(a.Device, b.Device), cmp.Compare(a.Identity, b.Identity))
	})
}

// revoke met en file le retrait d'un appareil d'un PC (une seule fois).
func (o *Owner) revoke(device, identity string) {
	r := Revocation{Device: device, Identity: identity}
	if !slices.Contains(o.Revocations, r) {
		o.Revocations = append(o.Revocations, r)
	}
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

// --- Sauvegarde (export complet protégé par mot de passe) ---

// ExportedOwner est le côté « je partage » tel qu'il figure dans l'export chiffré (« sharing ») :
// sans le jeton GitHub, à reconnecter sur le nouvel appareil. Même format qu'Android.
type ExportedOwner struct {
	Key       string   `json:"key"`
	PublicKey string   `json:"publicKey"`
	Name      string   `json:"name"`
	User      string   `json:"user"`
	Gist      string   `json:"gist"`
	Revision  int64    `json:"revision"`
	People    []Person `json:"people"`
	// Revocations : retraits pas encore appliqués sur les PC (partage v2). Les appairages, eux, ne sont
	// pas exportés : l'application les retrouve dans le registre des PC.
	Revocations []Revocation `json:"revocations,omitempty"`
}

// Export prépare la sauvegarde du côté « je partage ».
func (o *Owner) Export() (ExportedOwner, error) {
	key, err := ParseOwnerPrivate(o.Key)
	if err != nil {
		return ExportedOwner{}, err
	}
	people := slices.Clone(o.People)
	if people == nil {
		people = []Person{}
	}
	return ExportedOwner{Key: o.Key, PublicKey: OwnerPublic(key), Name: o.Name, User: o.User, Gist: o.Gist, Revision: o.Revision,
		People: people, Revocations: slices.Clone(o.Revocations)}, nil
}

// Owner vérifie une sauvegarde et renvoie l'état correspondant (connexion GitHub à refaire).
func (e ExportedOwner) Owner() (*Owner, error) {
	key, err := ParseOwnerPrivate(e.Key)
	if err != nil || OwnerPublic(key) != e.PublicKey || !ValidName(e.Name) || len(e.People) > MaxPeople {
		return nil, ErrInvalid
	}
	if (e.User != "" && !loginPattern.MatchString(e.User)) || (e.Gist != "" && !gistPattern.MatchString(e.Gist)) {
		return nil, ErrInvalid
	}
	for _, p := range e.People {
		if !ValidName(p.Name) || !ValidDeviceKey(p.Device) || (p.Identity != "" && !validIdentity(p.Identity)) {
			return nil, ErrInvalid
		}
		for _, r := range p.Rights {
			if !r.Valid() {
				return nil, ErrInvalid
			}
		}
	}
	for _, r := range e.Revocations {
		if r.Device == "" || !validIdentity(r.Identity) {
			return nil, ErrInvalid
		}
	}
	return &Owner{Key: e.Key, Name: e.Name, User: e.User, Gist: e.Gist, Revision: e.Revision, People: slices.Clone(e.People),
		Revocations: slices.Clone(e.Revocations)}, nil
}

// validIdentity vérifie une empreinte wolagent/2 d'appareil (B64, sous sa forme canonique).
func validIdentity(s string) bool {
	pin, err := protocol2.ParsePin(s)
	return err == nil && pin.String() == s
}

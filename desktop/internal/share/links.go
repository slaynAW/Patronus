package share

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

// Liens échangés entre les deux personnes (QR code ou message) ; aucun ne contient de secret.
//
//	wolshare://invite?v=1&name=…&owner=…&user=…&gist=…   (de la personne qui partage)
//	wolshare://request?v=1&name=…&device=…&owner=…       (de la personne qui demande l'accès)
const Scheme = "wolshare"

var (
	loginPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})$`)
	gistPattern  = regexp.MustCompile(`^[0-9a-f]{20,40}$`)
	idPattern    = regexp.MustCompile(`^[0-9a-f]{32}$`)
)

// ErrLink signale un lien de partage illisible.
var ErrLink = errors.New("lien de partage invalide")

// Invite est l'invitation affichée par la personne qui partage.
type Invite struct {
	// Name est le nom de la personne qui partage.
	Name string `json:"name"`
	// Owner est sa clé publique de signature.
	Owner string `json:"owner"`
	// User et Gist désignent l'espace de stockage GitHub des fichiers d'accès.
	User string `json:"user"`
	Gist string `json:"gist"`
}

// Request est la demande d'accès renvoyée par la personne invitée.
type Request struct {
	Name string `json:"name"`
	// Device est la clé publique de réception de son appareil.
	Device string `json:"device"`
	// Owner est l'identifiant (KeyID) de la clé de la personne invitante : une demande ne sert qu'à elle.
	Owner string `json:"owner"`
}

// Link renvoie le lien de l'invitation.
func (i Invite) Link() string {
	return build("invite", url.Values{"name": {i.Name}, "owner": {i.Owner}, "user": {i.User}, "gist": {i.Gist}})
}

// Link renvoie le lien de la demande.
func (r Request) Link() string {
	return build("request", url.Values{"name": {r.Name}, "device": {r.Device}, "owner": {r.Owner}})
}

func build(kind string, q url.Values) string {
	q.Set("v", "1")
	return Scheme + "://" + kind + "?" + q.Encode()
}

// Validate vérifie chaque champ d'une invitation.
func (i Invite) Validate() error {
	if !ValidName(i.Name) || !ValidOwnerKey(i.Owner) || !loginPattern.MatchString(i.User) || !gistPattern.MatchString(i.Gist) {
		return ErrLink
	}
	return nil
}

// Validate vérifie chaque champ d'une demande.
func (r Request) Validate() error {
	if !ValidName(r.Name) || !ValidDeviceKey(r.Device) || !idPattern.MatchString(r.Owner) {
		return ErrLink
	}
	return nil
}

// ParseInvite lit un lien d'invitation (collé ou scanné).
func ParseInvite(text string) (Invite, error) {
	q, err := parse(text, "invite")
	if err != nil {
		return Invite{}, err
	}
	i := Invite{Name: q.Get("name"), Owner: q.Get("owner"), User: q.Get("user"), Gist: q.Get("gist")}
	return i, i.Validate()
}

// ParseRequest lit un lien de demande d'accès.
func ParseRequest(text string) (Request, error) {
	q, err := parse(text, "request")
	if err != nil {
		return Request{}, err
	}
	r := Request{Name: q.Get("name"), Device: q.Get("device"), Owner: q.Get("owner")}
	return r, r.Validate()
}

// Kind renvoie le type d'un lien de partage (« invite », « request ») ou "" s'il n'en est pas un.
func Kind(text string) string {
	u, err := url.Parse(strings.TrimSpace(text))
	if err != nil || !strings.EqualFold(u.Scheme, Scheme) {
		return ""
	}
	switch strings.ToLower(u.Host) {
	case "invite", "request":
		return strings.ToLower(u.Host)
	}
	return ""
}

func parse(text, kind string) (url.Values, error) {
	text = strings.TrimSpace(text)
	if len(text) > 2048 || Kind(text) != kind {
		return nil, ErrLink
	}
	u, err := url.Parse(text)
	if err != nil {
		return nil, ErrLink
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return nil, ErrLink
	}
	if q.Get("v") != "1" {
		return nil, errors.New("lien de partage d'une version plus récente : mettez l'application à jour")
	}
	for _, values := range q {
		if len(values) != 1 {
			return nil, ErrLink
		}
	}
	return q, nil
}

// Package devices tient le registre wolagent/2 de l'agent (docs/PROTOCOLE-V2.md, §6 et §8) : les
// appareils autorisés, chacun avec son rôle, et les invitations en attente, dans devices.json à côté
// de la configuration. Des invitations, seul le condensé du code est gardé.
package devices

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol2"
)

// FileName est le nom du registre, dans le dossier de configuration.
const FileName = "devices.json"

// Moyens d'ajout d'un appareil (Device.Via).
const (
	ViaPair    = "pair"
	ViaMigrate = "migrate"
	ViaLocal   = "local"
)

// ByLocal désigne, dans Device.By et Invite.By, une opération faite sur le PC.
const ByLocal = "local"

// seenPrecision : la dernière connexion d'un appareil n'est écrite sur le disque que si elle a avancé
// d'au moins ce délai, pour ne pas réécrire le registre à chaque connexion.
const seenPrecision = 10 * time.Minute

var (
	ErrAlreadyPaired = errors.New("appareil déjà enregistré")
	ErrUnknownDevice = errors.New("appareil introuvable")
	ErrLastAdmin     = errors.New("c'est le dernier administrateur du PC")
	// ErrPairingFailed couvre tous les refus d'un code (invalide, expiré, déjà utilisé, réservé à un
	// autre appareil) : la même réponse dans tous les cas (§6.3).
	ErrPairingFailed  = errors.New("code d'appairage refusé")
	ErrTooManyInvites = fmt.Errorf("%d invitations en attente au plus", protocol2.MaxPendingInvites)
	ErrUnknownInvite  = errors.New("invitation introuvable")
)

// Device est un appareil autorisé.
type Device struct {
	Pin   protocol2.Pin  `json:"device"`
	Name  string         `json:"name"`
	Role  protocol2.Role `json:"role"`
	Added int64          `json:"added"`
	Via   string         `json:"via"`
	// By est l'empreinte de l'administrateur à l'origine de l'invitation, ByLocal, ou vide (migration).
	By   string `json:"by,omitempty"`
	Seen int64  `json:"seen,omitempty"`
}

// Invite est une invitation en attente.
type Invite struct {
	ID string `json:"id"`
	// Digest est le condensé SHA-256 du code normalisé, en hexadécimal.
	Digest  string         `json:"digest"`
	Role    protocol2.Role `json:"role"`
	Name    string         `json:"name,omitempty"`
	Expires int64          `json:"expires"`
	// Device réserve l'invitation à une identité.
	Device *protocol2.Pin `json:"device,omitempty"`
	By     string         `json:"by,omitempty"`
}

type state struct {
	Devices []Device `json:"devices"`
	Invites []Invite `json:"invites"`
}

// Registry est le registre, sûr pour un usage concurrent.
type Registry struct {
	mu    sync.Mutex
	path  string
	now   func() time.Time
	st    state
	saved map[protocol2.Pin]int64 // dernière connexion telle qu'écrite sur le disque
}

// Path renvoie l'emplacement du registre dans le dossier dir.
func Path(dir string) string { return filepath.Join(dir, FileName) }

// Open lit le registre (vide s'il n'existe pas encore). Un registre illisible est une erreur : le
// remplacer par un registre vide retirerait tous les appareils en silence.
func Open(path string, now func() time.Time) (*Registry, error) {
	r := &Registry{path: path, now: now, saved: map[protocol2.Pin]int64{}}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return r, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &r.st); err != nil {
		return nil, fmt.Errorf("registre des appareils illisible (%s) : %w", path, err)
	}
	seen := map[protocol2.Pin]bool{}
	for _, d := range r.st.Devices {
		if seen[d.Pin] || d.Role.String() == "" {
			return nil, fmt.Errorf("registre des appareils incohérent (%s)", path)
		}
		seen[d.Pin] = true
		r.saved[d.Pin] = d.Seen
	}
	return r, nil
}

// Lookup cherche un appareil par son empreinte.
func (r *Registry) Lookup(pin protocol2.Pin) (Device, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	i := r.index(pin)
	if i < 0 {
		return Device{}, false
	}
	return r.st.Devices[i], true
}

// Devices renvoie les appareils, dans l'ordre de leur ajout.
func (r *Registry) Devices() []Device {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.st.Devices)
}

// Seen note la connexion d'un appareil. Elle n'est écrite sur le disque que de temps en temps.
func (r *Registry) Seen(pin protocol2.Pin) {
	r.mu.Lock()
	defer r.mu.Unlock()
	i := r.index(pin)
	if i < 0 {
		return
	}
	now := r.now().Unix()
	r.st.Devices[i].Seen = now
	if now-r.saved[pin] >= int64(seenPrecision.Seconds()) {
		_ = r.commit(r.st) // simple information : un échec d'écriture n'empêche rien
	}
}

// Add enregistre un appareil (ajout sur le PC, migration).
func (r *Registry) Add(d Device) (Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.index(d.Pin) >= 0 {
		return Device{}, ErrAlreadyPaired
	}
	if d.Role.String() == "" {
		return Device{}, fmt.Errorf("rôle invalide")
	}
	if d.Added == 0 {
		d.Added = r.now().Unix()
	}
	next := r.clone()
	next.Devices = append(next.Devices, d)
	return d, r.commit(next)
}

// Remove retire un appareil. Le dernier administrateur ne peut être retiré qu'avec force (sur le PC,
// après confirmation).
func (r *Registry) Remove(pin protocol2.Pin, force bool) (Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	i := r.index(pin)
	if i < 0 {
		return Device{}, ErrUnknownDevice
	}
	d := r.st.Devices[i]
	if d.Role == protocol2.RoleAdmin && r.admins() == 1 && !force {
		return Device{}, ErrLastAdmin
	}
	next := r.clone()
	next.Devices = slices.Delete(next.Devices, i, i+1)
	return d, r.commit(next)
}

// SetRole change le rôle d'un appareil. Rétrograder le dernier administrateur demande force.
func (r *Registry) SetRole(pin protocol2.Pin, role protocol2.Role, force bool) (Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	i := r.index(pin)
	if i < 0 {
		return Device{}, ErrUnknownDevice
	}
	if role.String() == "" {
		return Device{}, fmt.Errorf("rôle invalide")
	}
	d := r.st.Devices[i]
	if d.Role == protocol2.RoleAdmin && role != protocol2.RoleAdmin && r.admins() == 1 && !force {
		return Device{}, ErrLastAdmin
	}
	next := r.clone()
	next.Devices[i].Role = role
	return next.Devices[i], r.commit(next)
}

// Rename change le nom d'un appareil (déjà nettoyé par l'appelant).
func (r *Registry) Rename(pin protocol2.Pin, name string) (Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	i := r.index(pin)
	if i < 0 {
		return Device{}, ErrUnknownDevice
	}
	next := r.clone()
	next.Devices[i].Name = name
	return next.Devices[i], r.commit(next)
}

// CreateInvite crée une invitation à usage unique et renvoie son code, qui n'est gardé nulle part.
func (r *Registry) CreateInvite(role protocol2.Role, name string, ttl time.Duration, device *protocol2.Pin, by string) (Invite, string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if role.String() == "" {
		return Invite{}, "", fmt.Errorf("rôle invalide")
	}
	next := r.clone()
	next.Invites = r.pending()
	if len(next.Invites) >= protocol2.MaxPendingInvites {
		return Invite{}, "", ErrTooManyInvites
	}
	id := make([]byte, 8)
	_, _ = rand.Read(id) // ne renvoie jamais d'erreur
	code := protocol2.NewCode()
	inv := Invite{
		ID:      base64.RawURLEncoding.EncodeToString(id),
		Digest:  digest(code),
		Role:    role,
		Name:    name,
		Expires: r.now().Add(ttl).Unix(),
		Device:  device,
		By:      by,
	}
	next.Invites = append(next.Invites, inv)
	if err := r.commit(next); err != nil {
		return Invite{}, "", err
	}
	return inv, code, nil
}

// Invites renvoie les invitations en attente (les expirées sont écartées).
func (r *Registry) Invites() []Invite {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.pending()
}

// CancelInvite annule une invitation.
func (r *Registry) CancelInvite(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	next := r.clone()
	next.Invites = r.pending()
	i := slices.IndexFunc(next.Invites, func(inv Invite) bool { return inv.ID == id })
	if i < 0 {
		return ErrUnknownInvite
	}
	next.Invites = slices.Delete(next.Invites, i, i+1)
	return r.commit(next)
}

// CancelInvites annule toutes les invitations (appairage suspendu après trop d'échecs, §3.3).
func (r *Registry) CancelInvites() (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := len(r.pending())
	if len(r.st.Invites) == 0 {
		return 0, nil
	}
	next := r.clone()
	next.Invites = nil
	return n, r.commit(next)
}

// Redeem utilise une invitation (§6.3) : le code doit être valide, en cours de validité, et ne pas être
// réservé à une autre identité. L'appareil est alors enregistré avec le rôle et, s'il a été choisi par
// l'administrateur, le nom de l'invitation ; l'invitation est consommée. Tous les refus donnent
// ErrPairingFailed ; un appareil déjà enregistré reçoit ErrAlreadyPaired sans consommer l'invitation.
func (r *Registry) Redeem(code string, pin protocol2.Pin, name string) (Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.index(pin) >= 0 {
		return Device{}, ErrAlreadyPaired
	}
	normalized, err := protocol2.NormalizeCode(code)
	if err != nil {
		return Device{}, ErrPairingFailed
	}
	want, _ := hex.DecodeString(digest(normalized))
	invites := r.pending()
	found := -1
	for i, inv := range invites { // comparaison en temps constant, sur toutes les invitations
		got, _ := hex.DecodeString(inv.Digest)
		if hmac.Equal(got, want) {
			found = i
		}
	}
	if found < 0 || invites[found].Device != nil && *invites[found].Device != pin {
		return Device{}, ErrPairingFailed
	}
	inv := invites[found]
	if inv.Name != "" {
		name = inv.Name
	}
	d := Device{Pin: pin, Name: name, Role: inv.Role, Added: r.now().Unix(), Via: ViaPair, By: inv.By}
	next := r.clone()
	next.Devices = append(next.Devices, d)
	next.Invites = slices.Delete(invites, found, found+1)
	return d, r.commit(next)
}

func digest(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

func (r *Registry) index(pin protocol2.Pin) int {
	return slices.IndexFunc(r.st.Devices, func(d Device) bool { return d.Pin == pin })
}

func (r *Registry) admins() int {
	n := 0
	for _, d := range r.st.Devices {
		if d.Role == protocol2.RoleAdmin {
			n++
		}
	}
	return n
}

// pending renvoie une copie des invitations encore valables.
func (r *Registry) pending() []Invite {
	now := r.now().Unix()
	var out []Invite
	for _, inv := range r.st.Invites {
		if now < inv.Expires {
			out = append(out, inv)
		}
	}
	return out
}

func (r *Registry) clone() state {
	return state{Devices: slices.Clone(r.st.Devices), Invites: slices.Clone(r.st.Invites)}
}

// commit écrit next sur le disque puis l'adopte : en cas d'échec, rien ne change (une révocation qui
// ne serait pas enregistrée reviendrait au prochain démarrage).
func (r *Registry) commit(next state) error {
	if next.Devices == nil {
		next.Devices = []Device{}
	}
	if next.Invites == nil {
		next.Invites = []Invite{}
	}
	raw, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(r.path), 0o700); err != nil {
		return err
	}
	tmp := r.path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, r.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	r.st = next
	for _, d := range next.Devices {
		r.saved[d.Pin] = d.Seen
	}
	return nil
}

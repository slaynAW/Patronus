package devices

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol2"
)

type clock struct{ t time.Time }

func (c *clock) Now() time.Time { return c.t }

func open(t *testing.T) (*Registry, *clock, string) {
	t.Helper()
	c := &clock{t: time.Date(2026, 10, 12, 15, 0, 0, 0, time.UTC)}
	path := filepath.Join(t.TempDir(), FileName)
	r, err := Open(path, c.Now)
	if err != nil {
		t.Fatal(err)
	}
	return r, c, path
}

func pin(label string) protocol2.Pin { return protocol2.PinOf([]byte(label)) }

func TestDevicesAndLastAdmin(t *testing.T) {
	r, _, path := open(t)
	admin, phone := pin("admin"), pin("téléphone")
	if _, err := r.Add(Device{Pin: admin, Name: "PC portable", Role: protocol2.RoleAdmin, Via: ViaLocal, By: ByLocal}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Add(Device{Pin: phone, Name: "Pixel 8", Role: protocol2.RoleControl, Via: ViaMigrate}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Add(Device{Pin: phone, Name: "doublon", Role: protocol2.RoleView}); !errors.Is(err, ErrAlreadyPaired) {
		t.Errorf("doublon : %v", err)
	}
	// Dernier administrateur : ni retiré ni rétrogradé sans confirmation.
	if _, err := r.Remove(admin, false); !errors.Is(err, ErrLastAdmin) {
		t.Errorf("retrait du dernier administrateur : %v", err)
	}
	if _, err := r.SetRole(admin, protocol2.RoleView, false); !errors.Is(err, ErrLastAdmin) {
		t.Errorf("rétrogradation du dernier administrateur : %v", err)
	}
	if d, err := r.SetRole(phone, protocol2.RoleAdmin, false); err != nil || d.Role != protocol2.RoleAdmin {
		t.Fatalf("promotion : %+v %v", d, err)
	}
	if _, err := r.SetRole(admin, protocol2.RoleView, false); err != nil {
		t.Errorf("rétrogradation avec un autre administrateur : %v", err)
	}
	if d, err := r.Rename(phone, "Pixel de Léa"); err != nil || d.Name != "Pixel de Léa" {
		t.Errorf("renommage : %+v %v", d, err)
	}
	if _, err := r.Remove(phone, true); err != nil {
		t.Errorf("retrait confirmé sur le PC : %v", err)
	}
	if _, err := r.Remove(phone, false); !errors.Is(err, ErrUnknownDevice) {
		t.Errorf("retrait d'un inconnu : %v", err)
	}
	// Relu à l'identique.
	again, err := Open(path, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if got := again.Devices(); len(got) != 1 || got[0].Pin != admin || got[0].Role != protocol2.RoleView {
		t.Errorf("relecture : %+v", got)
	}
	if info, err := os.Stat(path); err == nil && info.Mode().Perm()&0o077 != 0 && os.PathSeparator == '/' {
		t.Errorf("droits du registre : %v", info.Mode())
	}
}

func TestInvites(t *testing.T) {
	r, c, path := open(t)
	inv, code, err := r.CreateInvite(protocol2.RoleControl, "Pixel de Léa", 10*time.Minute, nil, ByLocal)
	if err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(path); strings.Contains(string(raw), code) {
		t.Fatal("le code est enregistré en clair")
	}
	// Mauvais code, mal saisi mais juste, puis déjà utilisé : même refus pour tous les échecs.
	if _, err := r.Redeem("0000-0000", pin("x"), "x"); !errors.Is(err, ErrPairingFailed) {
		t.Errorf("mauvais code : %v", err)
	}
	typed := strings.ToLower(protocol2.FormatCode(code))
	d, err := r.Redeem(typed, pin("phone"), "Pixel 8")
	if err != nil || d.Role != protocol2.RoleControl || d.Name != "Pixel de Léa" || d.Via != ViaPair || d.By != ByLocal {
		t.Fatalf("appairage : %+v %v", d, err)
	}
	if _, err := r.Redeem(code, pin("autre"), "x"); !errors.Is(err, ErrPairingFailed) {
		t.Errorf("code réutilisé : %v", err)
	}
	if len(r.Invites()) != 0 || inv.ID == "" {
		t.Errorf("invitation non consommée : %+v", r.Invites())
	}

	// Invitation réservée à un appareil ; un appareil déjà enregistré ne la consomme pas.
	only := pin("réservé")
	_, code, _ = r.CreateInvite(protocol2.RoleView, "", time.Hour, &only, pin("phone").String())
	if _, err := r.Redeem(code, pin("intrus"), "x"); !errors.Is(err, ErrPairingFailed) {
		t.Errorf("invitation réservée : %v", err)
	}
	if _, err := r.Redeem(code, pin("phone"), "x"); !errors.Is(err, ErrAlreadyPaired) || len(r.Invites()) != 1 {
		t.Errorf("appareil déjà enregistré : %v, %d invitation(s)", err, len(r.Invites()))
	}
	if d, err := r.Redeem(code, only, "Tablette"); err != nil || d.Name != "Tablette" || d.Role != protocol2.RoleView {
		t.Errorf("appareil réservé : %+v %v", d, err)
	}

	// Expiration, annulation, limite.
	_, code, _ = r.CreateInvite(protocol2.RoleControl, "", time.Minute, nil, ByLocal)
	c.t = c.t.Add(time.Minute)
	if _, err := r.Redeem(code, pin("tard"), "x"); !errors.Is(err, ErrPairingFailed) {
		t.Errorf("invitation expirée : %v", err)
	}
	for i := range protocol2.MaxPendingInvites {
		if _, _, err := r.CreateInvite(protocol2.RoleView, "", time.Hour, nil, ByLocal); err != nil {
			t.Fatalf("invitation %d : %v", i, err)
		}
	}
	if _, _, err := r.CreateInvite(protocol2.RoleView, "", time.Hour, nil, ByLocal); !errors.Is(err, ErrTooManyInvites) {
		t.Errorf("limite : %v", err)
	}
	first := r.Invites()[0]
	if err := r.CancelInvite(first.ID); err != nil || len(r.Invites()) != protocol2.MaxPendingInvites-1 {
		t.Errorf("annulation : %v", err)
	}
	if err := r.CancelInvite(first.ID); !errors.Is(err, ErrUnknownInvite) {
		t.Errorf("annulation en double : %v", err)
	}
	if n, err := r.CancelInvites(); err != nil || n != protocol2.MaxPendingInvites-1 || len(r.Invites()) != 0 {
		t.Errorf("annulation de toutes les invitations : %d %v", n, err)
	}
}

func TestSeenIsWrittenSparingly(t *testing.T) {
	r, c, path := open(t)
	p := pin("phone")
	if _, err := r.Add(Device{Pin: p, Name: "Pixel 8", Role: protocol2.RoleView, Via: ViaPair}); err != nil {
		t.Fatal(err)
	}
	r.Seen(p) // première connexion : enregistrée
	before, _ := os.ReadFile(path)
	if !strings.Contains(string(before), `"seen"`) {
		t.Fatal("première connexion non enregistrée")
	}
	c.t = c.t.Add(time.Minute)
	r.Seen(p)
	if d, _ := r.Lookup(p); d.Seen != c.t.Unix() {
		t.Errorf("dernière connexion en mémoire : %d", d.Seen)
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Error("registre réécrit pour une connexion rapprochée")
	}
	c.t = c.t.Add(seenPrecision)
	r.Seen(p)
	if again, _ := Open(path, c.Now); again.Devices()[0].Seen != c.t.Unix() {
		t.Error("dernière connexion non enregistrée")
	}
}

func TestOpenRejectsDamagedRegistry(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"illisible":       "{",
		"rôle inconnu":    `{"devices":[{"device":"` + pin("a").String() + `","name":"a","role":"owner","added":1,"via":"pair"}]}`,
		"empreinte":       `{"devices":[{"device":"abc","name":"a","role":"view","added":1,"via":"pair"}]}`,
		"appareil double": `{"devices":[{"device":"` + pin("a").String() + `","name":"a","role":"view","added":1,"via":"pair"},{"device":"` + pin("a").String() + `","name":"b","role":"view","added":1,"via":"pair"}]}`,
	} {
		path := filepath.Join(dir, name+".json")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Open(path, time.Now); err == nil {
			t.Errorf("%s : registre accepté", name)
		}
	}
}

// memStorage garde le registre en mémoire, comme le ferait le stockage chiffré du hub.
type memStorage struct {
	data    []byte
	loadErr error
	saveErr error
}

func (m *memStorage) Load() ([]byte, error) {
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	if m.data == nil {
		return nil, os.ErrNotExist
	}
	return m.data, nil
}

func (m *memStorage) Save(data []byte) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.data = append([]byte(nil), data...)
	return nil
}

func TestOpenStorage(t *testing.T) {
	c := &clock{t: time.Date(2026, 10, 12, 15, 0, 0, 0, time.UTC)}
	st := &memStorage{}
	r, err := OpenStorage(st, "hub", c.Now)
	if err != nil || len(r.Devices()) != 0 {
		t.Fatalf("registre vide : %v, %v", r, err)
	}
	if _, err := r.Add(Device{Pin: pin("admin"), Name: "Pixel 8", Role: protocol2.RoleAdmin, Via: ViaPair}); err != nil {
		t.Fatal(err)
	}
	again, err := OpenStorage(st, "hub", c.Now)
	if err != nil || len(again.Devices()) != 1 || again.Devices()[0].Name != "Pixel 8" {
		t.Fatalf("relecture : %v, %v", again.Devices(), err)
	}
	// Échec de l'enregistrement : rien ne change.
	st.saveErr = errors.New("disque plein")
	if _, err := r.Add(Device{Pin: pin("autre"), Name: "Tablette", Role: protocol2.RoleView, Via: ViaPair}); err == nil {
		t.Fatal("ajout non enregistré accepté")
	}
	if len(r.Devices()) != 1 {
		t.Fatalf("ajout gardé malgré l'échec : %v", r.Devices())
	}
	// Lecture impossible ou contenu illisible : erreur, jamais un registre vide.
	if _, err := OpenStorage(&memStorage{loadErr: errors.New("illisible")}, "hub", c.Now); err == nil {
		t.Error("erreur de lecture ignorée")
	}
	if _, err := OpenStorage(&memStorage{data: []byte("{")}, "hub", c.Now); err == nil || !strings.Contains(err.Error(), "hub") {
		t.Errorf("registre illisible : %v", err)
	}
}

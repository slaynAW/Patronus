package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/hubprotocol"
	"github.com/slaynaw/wakeonlan/agent/internal/devices"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/agents"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/archive"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/fleet"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/settings"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/store"
	"github.com/slaynaw/wakeonlan/agent/internal/identity"
	"github.com/slaynaw/wakeonlan/agent/protocol2"
)

// Fuzzing du répartiteur des requêtes : go test ./internal/hub/server -run '^$' -fuzz FuzzRequests. Sans
// -fuzz, seuls les exemples de départ sont rejoués.

// fuzzBase est l'état de départ de chaque essai : un hub avec un appareil de chaque rôle, une invitation
// jamais divulguée, un PC confié et un appareil sans agent. Ses fichiers chiffrés sont recopiés dans un
// dossier neuf à chaque essai, qui part donc toujours du même état.
type fuzzBase struct {
	key    store.Key
	hub    *identity.Identity
	files  map[string][]byte
	pins   map[protocol2.Role]protocol2.Pin
	secret string // code de l'invitation en attente, jamais envoyé à personne
}

var fuzzRoles = []protocol2.Role{protocol2.RoleView, protocol2.RoleWake, protocol2.RoleAdmin}

func newFuzzBase(tb testing.TB) *fuzzBase {
	tb.Helper()
	dir := tb.TempDir()
	must := func(err error) {
		tb.Helper()
		if err != nil {
			tb.Fatal(err)
		}
	}
	key, _, err := store.LoadOrCreateKey(filepath.Join(dir, "keys"))
	must(err)
	hub, _, err := identity.LoadOrCreate(filepath.Join(dir, "keys"), time.Now())
	must(err)
	data := filepath.Join(dir, "data")
	st, err := store.Open(data, key)
	must(err)
	registry, err := devices.OpenStorage(st.File("devices.db"), "devices.db", time.Now)
	must(err)
	b := &fuzzBase{key: key, hub: hub, files: map[string][]byte{}, pins: map[protocol2.Role]protocol2.Pin{}}
	for _, role := range fuzzRoles {
		pin := protocol2.PinOf([]byte("appareil " + role.String()))
		_, code, err := registry.CreateInvite(role, "", time.Hour, nil, devices.ByLocal)
		must(err)
		_, err = registry.Redeem(code, pin, "Appareil "+role.String())
		must(err)
		b.pins[role] = pin
	}
	_, b.secret, err = registry.CreateInvite(protocol2.RoleAdmin, "Invitation secrète", 7*24*time.Hour, nil, devices.ByLocal)
	must(err)
	pcs, err := fleet.Open(st.File(fleet.FileName), time.Now)
	must(err)
	agent := protocol2.PinOf([]byte("agent du PC"))
	_, err = pcs.Add(fleet.PC{Name: "PC Bureau", Host: "192.168.1.20", Port: protocol2.DefaultPort, MAC: "30:9C:23:01:02:03",
		Agent: &agent, Role: protocol2.RoleWake})
	must(err)
	_, err = pcs.Add(fleet.PC{Name: "Box", Host: "192.168.1.1", Probe: &hubprotocol.Probe{Kind: hubprotocol.ProbeTCP, Port: 80}})
	must(err)
	for _, name := range []string{"devices.db", fleet.FileName} {
		b.files[name], err = os.ReadFile(filepath.Join(data, name))
		must(err)
	}
	return b
}

// fuzzHub est le hub d'un essai.
type fuzzHub struct {
	srv      *Server
	registry *devices.Registry
	fleet    *fleet.Fleet
	prefs    *settings.Settings
	archive  *archive.Archive
	wakes    atomic.Int32
}

func (b *fuzzBase) start(t *testing.T) *fuzzHub {
	t.Helper()
	data := t.TempDir()
	for name, raw := range b.files {
		if err := os.WriteFile(filepath.Join(data, name), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	st, err := store.Open(data, b.key)
	if err != nil {
		t.Fatal(err)
	}
	h := &fuzzHub{}
	if h.registry, err = devices.OpenStorage(st.File("devices.db"), "devices.db", time.Now); err != nil {
		t.Fatal(err)
	}
	if h.fleet, err = fleet.Open(st.File(fleet.FileName), time.Now); err != nil {
		t.Fatal(err)
	}
	if h.prefs, err = settings.Open(st.File(settings.FileName)); err != nil {
		t.Fatal(err)
	}
	h.archive = archive.New(st, time.Now)
	// Aucun réseau permis, aucun nom résolu : un appairage auprès d'un agent échoue tout de suite, sans
	// que rien ne sorte du test.
	offline := &net.Resolver{PreferGo: true, Dial: func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("pas de réseau pendant le fuzzing")
	}}
	h.srv, err = New(Config{
		Name: "Hub NAS", Version: "2.0.0-fuzz", Identity: b.hub, Registry: h.registry, Fleet: h.fleet,
		Agents:  &agents.Dialer{Identity: b.hub, Resolver: offline},
		Archive: h.archive, Settings: h.prefs, Logger: log.New(io.Discard, "", 0),
		Wake: func(context.Context, string) (int, error) { h.wakes.Add(1); return 1, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// state résume ce qu'une requête peut modifier : appareils, invitations, PC, réglages, évènements du hub.
func (h *fuzzHub) state(t *testing.T) (managed, events string) {
	t.Helper()
	invites := h.registry.Invites()
	for i := range invites {
		invites[i].Expires = 0 // seule l'existence compte
	}
	raw, err := json.Marshal([]any{h.registry.Devices(), invites, h.fleet.List(), h.prefs.Get()})
	if err != nil {
		t.Fatal(err)
	}
	evs, _, err := h.archive.HubEvents(0)
	if err != nil {
		t.Fatal(err)
	}
	ev, _ := json.Marshal(evs)
	return string(raw), string(ev)
}

// FuzzRequests rejoue des requêtes quelconques (une par ligne, sur une même session) au nom d'une
// identité inconnue, d'un appareil de chaque rôle, ou de la commande patronus-hub sur le NAS. Quelles que
// soient les requêtes :
//   - le hub ne panique jamais et répond par un objet JSON, à l'identifiant de la requête ;
//   - une identité inconnue n'obtient rien ni ne change rien (sans le code d'une invitation) ;
//   - un appareil « view » ne change rien, et ne réveille rien ;
//   - un appareil « wake » ne change rien d'autre que le journal des réveils ;
//   - aucune réponse ne contient le code d'une invitation existante.
func FuzzRequests(f *testing.F) {
	base := newFuzzBase(f)
	for _, seed := range []struct {
		who  uint8
		reqs string
	}{
		{0, `{"id":1,"cmd":"hub.pair","code":"7KQ2-9MXP","name":"Intrus"}`},
		{0, `{"id":1,"cmd":"hub.status"}`},
		{1, `{"id":1,"cmd":"hub.status"}` + "\n" + `{"id":2,"cmd":"hub.events","since":0}`},
		{1, `{"id":1,"cmd":"devices.revoke","device":"` + base.pins[protocol2.RoleAdmin].String() + `"}`},
		{1, `{"id":1,"cmd":"hub.settings","retention":90}`},
		{2, `{"id":1,"cmd":"hub.wake","pc":"x"}` + "\n" + `{"id":2,"cmd":"hub.schedules.set","pc":"x","schedule":{"wake":{"on":true,"days":[1],"at":"07:00"}}}`},
		{2, `{"id":5,"cmd":"devices.invite","role":"admin"}`},
		{3, `{"id":1,"cmd":"devices.invite","role":"view","ttl":60}` + "\n" + `{"id":2,"cmd":"devices.invites"}`},
		{3, `{"id":1,"cmd":"hub.pcs.add","name":"PC","host":"pc.example","agent":"` + protocol2.PinOf([]byte("a")).String() + `","code":"7KQ29MXP"}`},
		{3, `{"id":1,"cmd":"hub.metrics","pc":"x","day":"../../keys"}` + "\n" + `{"id":2,"cmd":"hub.history","pc":"../x"}`},
		{3, `{"id":1,"cmd":"hub.settings","timezone":"../../../etc/passwd"}`},
		{4, `{"id":1,"cmd":"devices.revoke","device":"` + base.pins[protocol2.RoleAdmin].String() + `","force":true}`},
		{1, `{"id":2,"cmd":"hub.status"}` + "\n" + `{"id":1,"cmd":"hub.status"}`},
		{3, `[1,2,3]`},
		{3, `{"id":9223372036854775807,"cmd":"hub.status"}`},
	} {
		f.Add(seed.who, []byte(seed.reqs))
	}
	f.Fuzz(func(t *testing.T, who uint8, input []byte) {
		h := base.start(t)
		sess := &session{s: h.srv, ip: "192.168.1.50"}
		switch who % 5 {
		case 0:
			sess.pin = protocol2.PinOf([]byte("identité inconnue"))
		case 4:
			sess.pin, sess.local = base.hub.Pin, true
		default:
			sess.pin = base.pins[fuzzRoles[who%5-1]]
		}
		// Code de l'invitation en attente : la seule façon d'entrer. L'essai qui le contient n'est pas jugé
		// sur l'appairage (il est permis), seulement sur le reste.
		knowsCode := false
		if normalized, err := protocol2.NormalizeCode(base.secret); err == nil {
			knowsCode = bytes.Contains(bytes.ToUpper(bytes.ReplaceAll(input, []byte("-"), nil)), []byte(normalized))
		}
		lines := bytes.Split(input, []byte("\n"))
		if len(lines) > 8 {
			lines = lines[:8]
		}
		for _, line := range lines {
			if len(line) == 0 || len(line) > protocol2.MaxRequestBytes {
				continue
			}
			d, registered := sess.device()
			role := protocol2.Role(0)
			if registered {
				role = d.Role
			}
			beforeManaged, beforeEvents := h.state(t)
			wakesBefore := h.wakes.Load()
			lastID := sess.lastID

			raw, closing := sess.handle(line)

			var resp map[string]json.RawMessage
			if err := json.Unmarshal(raw, &resp); err != nil {
				t.Fatalf("réponse illisible à %q : %q", line, raw)
			}
			var req hubprotocol.Request // lue comme le hub la lit
			var id int64
			_ = json.Unmarshal(resp["id"], &id)
			if json.Unmarshal(line, &req) == nil && req.ID > lastID && req.ID <= protocol2.MaxRequestID && id != req.ID {
				t.Fatalf("réponse à %d au lieu de %d : %q", id, req.ID, raw)
			}
			var ok bool
			_ = json.Unmarshal(resp["ok"], &ok)
			if normalized, err := protocol2.NormalizeCode(base.secret); err == nil && !knowsCode && strings.Contains(strings.ReplaceAll(strings.ToUpper(string(raw)), "-", ""), normalized) {
				t.Fatalf("code d'une invitation dans la réponse : %q", raw)
			}

			afterManaged, afterEvents := h.state(t)
			switch {
			case !registered && !knowsCode:
				if ok || afterManaged != beforeManaged || afterEvents != beforeEvents || h.wakes.Load() != wakesBefore {
					t.Fatalf("identité inconnue : %q a changé l'état ou réussi (%q)", line, raw)
				}
			case registered && role == protocol2.RoleView:
				if afterManaged != beforeManaged || afterEvents != beforeEvents || h.wakes.Load() != wakesBefore {
					t.Fatalf("rôle view : %q a changé l'état (%q)", line, raw)
				}
			case registered && role == protocol2.RoleWake:
				if afterManaged != beforeManaged {
					t.Fatalf("rôle wake : %q a changé l'état (%q)", line, raw)
				}
			}
			if closing {
				return
			}
		}
	})
}

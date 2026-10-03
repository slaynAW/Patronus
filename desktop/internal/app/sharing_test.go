package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/desktop/internal/config"
	"github.com/slaynaw/wakeonlan/desktop/internal/history"
	"github.com/slaynaw/wakeonlan/desktop/internal/model"
	"github.com/slaynaw/wakeonlan/desktop/internal/netstate"
	"github.com/slaynaw/wakeonlan/desktop/internal/share"
	"github.com/slaynaw/wakeonlan/desktop/internal/status"
)

// gistServer imite GitHub : connexion par code (validée immédiatement), compte, Gists.
type gistServer struct {
	mu    sync.Mutex
	files map[string]map[string]string
	// descriptions : description de chaque Gist (listes du compte, sauvegardes).
	descriptions map[string]string
	seq          int
	// created : Gists créés (identifiants jamais réutilisés, même après une suppression).
	created int
}

func newGistServer(t *testing.T) (*gistServer, *httptest.Server) {
	t.Helper()
	g := &gistServer{files: map[string]map[string]string{}, descriptions: map[string]string{}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /login/device/code", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"device_code":"d","user_code":"WXYZ-1234","verification_uri":"https://github.com/login/device","expires_in":600,"interval":1}`)
	})
	mux.HandleFunc("POST /login/oauth/access_token", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"access_token":"tok","token_type":"bearer","scope":"gist"}`)
	})
	mux.HandleFunc("GET /user", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, `{"login":"hugo"}`) })
	mux.HandleFunc("POST /gists", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Description string `json:"description"`
			Public      bool   `json:"public"`
			Files       map[string]struct {
				Content string `json:"content"`
			} `json:"files"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		g.mu.Lock()
		if body.Public {
			t.Error("Gist public créé")
		}
		id := "abcdefabcdefabcdefabcdefabcdef12"
		if g.created > 0 {
			id = fmt.Sprintf("%032x", g.created+1)
		}
		g.created++
		g.descriptions[id] = body.Description
		g.files[id] = map[string]string{}
		for name, f := range body.Files {
			g.files[id][name] = f.Content
		}
		g.seq++
		g.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":"`+id+`"}`)
	})
	mux.HandleFunc("PATCH /gists/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var body struct {
			Files map[string]*struct {
				Content string `json:"content"`
			} `json:"files"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		g.mu.Lock()
		defer g.mu.Unlock()
		files := g.files[r.PathValue("id")]
		for name, f := range body.Files {
			if f == nil {
				delete(files, name)
			} else {
				files[name] = f.Content
			}
		}
		g.seq++
		_, _ = io.WriteString(w, `{}`)
	})
	mux.HandleFunc("DELETE /gists/{id}", func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		delete(g.files, r.PathValue("id"))
		g.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /gists", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		g.mu.Lock()
		defer g.mu.Unlock()
		list := []map[string]string{}
		for id := range g.files {
			list = append(list, map[string]string{"id": id, "description": g.descriptions[id]})
		}
		_ = json.NewEncoder(w).Encode(list)
	})
	mux.HandleFunc("GET /gists/{id}", func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		defer g.mu.Unlock()
		files, ok := g.files[r.PathValue("id")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		out := map[string]any{}
		for name, c := range files {
			out[name] = map[string]any{"content": c, "size": len(c)}
		}
		w.Header().Set("ETag", `"`+strings.Repeat("x", g.seq)+`"`)
		if r.Header.Get("If-None-Match") == w.Header().Get("ETag") {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"files": out})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return g, srv
}

func newShareService(t *testing.T, srv *httptest.Server, devices ...model.Device) *Service {
	t.Helper()
	dir := t.TempDir()
	store, err := config.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) > 0 {
		if err := store.Save(model.AppConfig{SchemaVersion: model.CurrentSchemaVersion, Devices: devices, Settings: model.DefaultSettings()}); err != nil {
			t.Fatal(err)
		}
	}
	shareStore, _ := share.NewStore(dir)
	gh := &share.GitHub{API: srv.URL, Web: srv.URL, ClientID: "client", Client: srv.Client(), PollUnit: time.Millisecond}
	return New(Options{
		Version: "test", Store: store, Platform: &fakePlatform{},
		Prober:   status.ProberFunc(func(context.Context, model.Device) status.ProbeResult { return status.ProbeResult{} }),
		NetState: func() (netstate.State, error) { return netstate.State{}, nil },
		Share:    &ShareOptions{Store: shareStore, GitHub: gh},
	})
}

func testDevice(t *testing.T, id, name, host string, agentKey string) model.Device {
	t.Helper()
	mac, _ := model.ParseMAC("AA:BB:CC:00:00:0" + id[len(id)-1:])
	d := model.Device{ID: id, Name: name, MAC: mac, Host: host, WolPort: 9, ProbePorts: []int{3389}}
	if agentKey != "" {
		d.Agent = &model.AgentSettings{Port: 9770, Key: agentKey}
	}
	return d
}

func TestShareEndToEnd(t *testing.T) {
	gists, srv := newGistServer(t)
	ctx := context.Background()
	key := "kY5jjMI6cQpU1gqHSiok1wAPlr6OUVgey66jYkJw5DI"
	owner := newShareService(t, srv,
		testDevice(t, "dev-1", "PC streaming", "192.168.1.20", key),
		testDevice(t, "dev-2", "Bureau", "192.168.1.21", key))
	guest := newShareService(t, srv)

	// Connexion GitHub de la personne qui partage.
	login := call(t, owner, "shareLogin", map[string]any{"name": "Hugo"})
	if login["code"] != "WXYZ-1234" {
		t.Fatalf("code : %v", login)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && (owner.State().Share.Owner == nil || !owner.State().Share.Owner.Connected) {
		time.Sleep(10 * time.Millisecond)
	}
	if o := owner.State().Share.Owner; o == nil || !o.Connected || o.User != "hugo" || o.Name != "Hugo" {
		t.Fatalf("connexion : %+v", owner.State().Share)
	}
	gists.mu.Lock()
	if len(gists.files) != 1 {
		t.Errorf("Gist non créé : %v", gists.files)
	}
	gists.mu.Unlock()

	// Invitation → demande de Léa.
	invite := call(t, owner, "shareInvite", nil)
	link, _ := invite["link"].(string)
	if !strings.HasPrefix(link, "wolshare://invite?") || !strings.Contains(invite["qr"].(string), "<svg") {
		t.Fatalf("invitation : %v", invite)
	}
	if r := call(t, guest, "shareReadInvite", map[string]any{"text": link}); r["ok"] != true || r["name"] != "Hugo" {
		t.Fatalf("lecture de l'invitation : %v", r)
	}
	if r := call(t, owner, "shareReadInvite", map[string]any{"text": link}); r["ok"] != false {
		t.Errorf("propre invitation acceptée : %v", r)
	}
	req := call(t, guest, "shareRequest", map[string]any{"text": link, "name": "Léa"})
	reqLink, _ := req["link"].(string)
	if !strings.HasPrefix(reqLink, "wolshare://request?") || req["code"] == "" {
		t.Fatalf("demande : %v", req)
	}
	ownerKey := guest.State().Share.Received[0].Owner
	guest.syncAccess(ctx, ownerKey)
	if a := guest.State().Share.Received[0]; a.Active || a.OwnerName != "Hugo" || a.Error != "" {
		t.Errorf("demande en attente : %+v", a)
	}

	// Lecture de la demande : même code des deux côtés.
	read := call(t, owner, "shareReadRequest", map[string]any{"text": reqLink})
	if read["ok"] != true || read["code"] != req["code"] || read["name"] != "Léa" {
		t.Fatalf("lecture de la demande : %v", read)
	}
	// Autorisation : PC streaming en « démarrer » seulement.
	call(t, owner, "shareGrant", map[string]any{"device": read["device"], "name": "Léa", "rights": map[string]string{"dev-1": "wake"}})
	if err := owner.publishShares(ctx); err != nil {
		t.Fatal(err)
	}
	if p := owner.State().Share.Owner.People; len(p) != 1 || !p[0].Published || p[0].Devices[0] != "PC streaming" {
		t.Errorf("personnes : %+v", p)
	}

	// Léa reçoit le PC, sans la clé de l'agent et sans le bureau.
	guest.syncAccess(ctx, ownerKey)
	st := guest.State()
	if len(st.Devices) != 1 || st.Devices[0].Name != "PC streaming" || st.Devices[0].Shared == nil ||
		st.Devices[0].Shared.OwnerName != "Hugo" || st.Devices[0].HasAgent || st.Devices[0].CanShutdown {
		t.Fatalf("PC reçus : %+v", st.Devices)
	}
	sharedID := st.Devices[0].ID
	if !st.Share.Received[0].Active {
		t.Error("accès non actif")
	}
	// Non modifiable, absent des exports.
	if _, err := guest.Call("deleteDevice", json.RawMessage(`{"id":"`+sharedID+`"}`)); err == nil {
		t.Error("PC partagé supprimé")
	}
	if _, err := guest.Call("saveDevice", json.RawMessage(`{"id":"`+sharedID+`","form":{}}`)); err == nil {
		t.Error("PC partagé modifié")
	}
	if d := call(t, guest, "getDevice", map[string]any{"id": sharedID}); d["shared"] != true {
		t.Errorf("formulaire d'un PC partagé : %v", d)
	}
	guest.mu.Lock()
	if len(guest.cfg.Devices) != 0 {
		t.Error("PC partagé ajouté à la configuration")
	}
	guest.mu.Unlock()

	// Modification chez Hugo → republication → Léa à jour (même identifiant local).
	form := call(t, owner, "getDevice", map[string]any{"id": "dev-1"})["form"]
	f := form.(map[string]any)
	f["host"] = "192.168.1.30"
	if r := call(t, owner, "saveDevice", map[string]any{"id": "dev-1", "form": f}); r["ok"] != true {
		t.Fatalf("modification : %v", r)
	}
	if err := owner.publishShares(ctx); err != nil {
		t.Fatal(err)
	}
	guest.syncAccess(ctx, ownerKey)
	if st := guest.State(); len(st.Devices) != 1 || st.Devices[0].Host != "192.168.1.30" || st.Devices[0].ID != sharedID {
		t.Errorf("mise à jour : %+v", st.Devices)
	}

	// Accès complet : la clé de l'agent est transmise, extinction possible.
	call(t, owner, "shareGrant", map[string]any{"device": read["device"], "name": "Léa", "rights": map[string]string{"dev-1": "full"}})
	_ = owner.publishShares(ctx)
	guest.syncAccess(ctx, ownerKey)
	if st := guest.State(); !st.Devices[0].CanShutdown {
		t.Errorf("accès complet : %+v", st.Devices[0])
	}

	// Retrait : le PC disparaît chez Léa.
	call(t, owner, "shareRevoke", map[string]any{"device": read["device"]})
	if err := owner.publishShares(ctx); err != nil {
		t.Fatal(err)
	}
	guest.syncAccess(ctx, ownerKey)
	st = guest.State()
	if len(st.Devices) != 0 || !st.Share.Received[0].Removed {
		t.Errorf("après retrait : %+v %+v", st.Devices, st.Share.Received)
	}

	// Arrêt du partage : Gist supprimé.
	call(t, owner, "shareStop", nil)
	gists.mu.Lock()
	if len(gists.files) != 0 {
		t.Error("Gist non supprimé")
	}
	gists.mu.Unlock()
	if owner.State().Share.Owner != nil {
		t.Error("partage toujours actif")
	}
	// Léa peut retirer le partage de sa liste.
	call(t, guest, "shareLeave", map[string]any{"owner": ownerKey})
	if len(guest.State().Share.Received) != 0 {
		t.Error("partage non supprimé")
	}
}

func TestShareRejectsForeignRequest(t *testing.T) {
	_, srv := newGistServer(t)
	a := newShareService(t, srv, testDevice(t, "dev-1", "PC", "192.168.1.20", ""))
	b := newShareService(t, srv)
	call(t, a, "shareLogin", map[string]any{"name": "Hugo"})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && (a.State().Share.Owner == nil || !a.State().Share.Owner.Connected) {
		time.Sleep(10 * time.Millisecond)
	}
	// Demande adressée à une autre personne (clé différente) : refusée.
	other, _ := share.NewOwnerKey()
	device, _ := share.NewDeviceKey()
	foreign := share.Request{Name: "Intrus", Device: share.DevicePublic(device), Owner: share.KeyID(share.OwnerPublic(other))}
	if r := call(t, a, "shareReadRequest", map[string]any{"text": foreign.Link()}); r["ok"] != false {
		t.Errorf("demande étrangère acceptée : %v", r)
	}
	// Autorisation sans PC : refusée.
	if _, err := a.Call("shareGrant", json.RawMessage(`{"device":"`+share.DevicePublic(device)+`","name":"Léa","rights":{}}`)); err == nil {
		t.Error("autorisation sans PC acceptée")
	}
	// Droit « complet » sur un PC sans agent : ramené à « démarrer ».
	call(t, a, "shareGrant", map[string]any{"device": share.DevicePublic(device), "name": "Léa", "rights": map[string]string{"dev-1": "full"}})
	if r := a.State().Share.Owner.People[0].Rights["dev-1"]; r != share.RightWake {
		t.Errorf("droit : %s", r)
	}
	if b.State().Share.Available != true || b.State().Share.CanLogin != true {
		t.Error("partage indisponible")
	}
}

// La clé de partage suit la sauvegarde complète : sur un nouvel appareil, les accès continuent
// après reconnexion à GitHub (même Gist, mêmes personnes).
func TestShareKeyInFullBackup(t *testing.T) {
	_, srv := newGistServer(t)
	a := newShareService(t, srv, testDevice(t, "dev-1", "PC", "192.168.1.20", ""))
	call(t, a, "shareLogin", map[string]any{"name": "Hugo"})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && (a.State().Share.Owner == nil || !a.State().Share.Owner.Connected) {
		time.Sleep(10 * time.Millisecond)
	}
	device, _ := share.NewDeviceKey()
	call(t, a, "shareGrant", map[string]any{"device": share.DevicePublic(device), "name": "Léa", "rights": map[string]string{"dev-1": "wake"}})

	platform := a.platform.(*fakePlatform)
	call(t, a, "exportConfig", map[string]any{"withSecrets": true, "password": "motdepasse"})
	backup := platform.saved
	if strings.Contains(string(backup), "Léa") {
		t.Fatal("sauvegarde lisible")
	}
	// Export lisible : jamais la clé de partage.
	call(t, a, "exportConfig", map[string]any{"withSecrets": false})
	if strings.Contains(string(platform.saved), "sharing") {
		t.Error("clé de partage dans un export lisible")
	}

	b := newShareService(t, srv)
	if r := call(t, b, "importFile", map[string]any{"text": string(backup)}); r["step"] != "password" {
		t.Fatalf("import : %v", r)
	}
	step := call(t, b, "importPassword", map[string]any{"password": "motdepasse"})
	info, _ := step["sharing"].(map[string]any)
	if info["name"] != "Hugo" || info["people"] != float64(1) {
		t.Fatalf("partage annoncé : %v", step)
	}
	if r := call(t, b, "importConfirm", map[string]any{"replace": true}); r["sharing"] != true {
		t.Fatalf("partage repris : %v", r)
	}
	o := b.State().Share.Owner
	if o == nil || o.Connected || o.Name != "Hugo" || o.User != "hugo" || len(o.People) != 1 || o.People[0].Name != "Léa" {
		t.Fatalf("partage importé : %+v", o)
	}
	// Même clé : l'invitation reste valable après reconnexion.
	call(t, b, "shareLogin", map[string]any{"name": ""})
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && !b.State().Share.Owner.Connected {
		time.Sleep(10 * time.Millisecond)
	}
	ia := call(t, a, "shareInvite", nil)["link"]
	ib := call(t, b, "shareInvite", nil)["link"]
	if ia != ib {
		t.Errorf("invitations différentes :\n%v\n%v", ia, ib)
	}
}

// Jeton révoqué sur GitHub : la publication échoue et l'interface propose de se reconnecter.
func TestShareRevokedToken(t *testing.T) {
	_, srv := newGistServer(t)
	a := newShareService(t, srv, testDevice(t, "dev-1", "PC", "192.168.1.20", ""))
	call(t, a, "shareLogin", map[string]any{"name": "Hugo"})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && (a.State().Share.Owner == nil || !a.State().Share.Owner.Connected) {
		time.Sleep(10 * time.Millisecond)
	}
	a.sharing.mu.Lock()
	a.sharing.state.Owner.Token = "revoque"
	a.sharing.mu.Unlock()
	device, _ := share.NewDeviceKey()
	call(t, a, "shareGrant", map[string]any{"device": share.DevicePublic(device), "name": "Léa", "rights": map[string]string{"dev-1": "wake"}})
	if err := a.publishShares(context.Background()); err == nil {
		t.Fatal("publication acceptée avec un jeton révoqué")
	}
	if o := a.State().Share.Owner; o.Connected || o.Error == "" || len(o.People) != 1 {
		t.Errorf("après révocation : %+v", o)
	}
}

// Sauvegarde complète : l'historique suit et s'ajoute à celui de l'appareil qui l'importe.
func TestHistoryInFullBackup(t *testing.T) {
	_, srv := newGistServer(t)
	a := newShareService(t, srv, testDevice(t, "dev-1", "PC", "192.168.1.20", ""))
	now := time.Now().UnixMilli()
	a.recordAll([]history.Event{
		{Device: "dev-1", Time: now - 60_000, Kind: history.WakeSent, Source: history.App},
		{Device: "dev-1", Time: now - 30_000, Kind: history.On, Source: history.App, Approx: true},
	})
	platform := a.platform.(*fakePlatform)
	call(t, a, "exportConfig", map[string]any{"withSecrets": false})
	if strings.Contains(string(platform.saved), `"history"`) {
		t.Error("historique dans un export lisible")
	}
	call(t, a, "exportConfig", map[string]any{"withSecrets": true, "password": "motdepasse"})
	backup := platform.saved

	// Appareil qui a déjà son propre historique pour ce PC (et un PC absent de la sauvegarde).
	b := newShareService(t, srv, testDevice(t, "dev-1", "PC", "192.168.1.20", ""))
	b.recordAll([]history.Event{{Device: "dev-1", Time: now - 10_000, Kind: history.ShutdownSent, Source: history.App}})
	call(t, b, "importFile", map[string]any{"text": string(backup)})
	step := call(t, b, "importPassword", map[string]any{"password": "motdepasse"})
	if step["history"] != float64(2) {
		t.Fatalf("historique annoncé : %v", step)
	}
	call(t, b, "importConfirm", map[string]any{"replace": false})
	kinds := func(s *Service) []string {
		var out []string
		for _, e := range call(t, s, "getHistory", map[string]any{"id": "dev-1"})["events"].([]any) {
			ev := e.(map[string]any)
			out = append(out, ev["kind"].(string))
		}
		return out
	}
	if got := kinds(b); !slices.Equal(got, []string{"shutdown_req", "on", "wake"}) {
		t.Fatalf("historique fusionné : %v", got)
	}
	// Importer deux fois la même sauvegarde ne duplique rien.
	call(t, b, "importFile", map[string]any{"text": string(backup)})
	call(t, b, "importPassword", map[string]any{"password": "motdepasse"})
	call(t, b, "importConfirm", map[string]any{"replace": false})
	if got := kinds(b); len(got) != 3 {
		t.Fatalf("second import : %v", got)
	}
}

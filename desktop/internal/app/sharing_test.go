package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/desktop/internal/config"
	"github.com/slaynaw/wakeonlan/desktop/internal/model"
	"github.com/slaynaw/wakeonlan/desktop/internal/netstate"
	"github.com/slaynaw/wakeonlan/desktop/internal/share"
	"github.com/slaynaw/wakeonlan/desktop/internal/status"
)

// gistServer imite GitHub : connexion par code (validée immédiatement), compte, Gists.
type gistServer struct {
	mu    sync.Mutex
	files map[string]map[string]string
	seq   int
}

func newGistServer(t *testing.T) (*gistServer, *httptest.Server) {
	t.Helper()
	g := &gistServer{files: map[string]map[string]string{}}
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
			Files map[string]struct {
				Content string `json:"content"`
			} `json:"files"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		g.mu.Lock()
		id := "abcdefabcdefabcdefabcdefabcdef12"
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
			out[name] = map[string]any{"content": c}
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

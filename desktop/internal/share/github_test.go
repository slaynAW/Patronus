package share

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeGitHub imite les parties de GitHub utilisées : connexion par code, compte, Gists.
type fakeGitHub struct {
	mu        sync.Mutex
	polls     int
	gists     map[string]map[string]string
	version   int
	lastToken string
}

func newFakeGitHub(t *testing.T) (*fakeGitHub, *GitHub) {
	t.Helper()
	f := &fakeGitHub{gists: map[string]map[string]string{}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /login/device/code", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("client_id") != "client-test" || r.Form.Get("scope") != "gist" || r.Header.Get("Accept") != "application/json" {
			http.Error(w, "bad", http.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(w, `{"device_code":"dev-1","user_code":"ABCD-1234","verification_uri":"https://github.com/login/device","expires_in":900,"interval":1}`)
	})
	mux.HandleFunc("POST /login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		f.mu.Lock()
		f.polls++
		n := f.polls
		f.mu.Unlock()
		switch {
		case r.Form.Get("device_code") != "dev-1":
			_, _ = io.WriteString(w, `{"error":"bad_verification_code"}`)
		case n == 1:
			_, _ = io.WriteString(w, `{"error":"authorization_pending"}`)
		case n == 2:
			_, _ = io.WriteString(w, `{"error":"slow_down","interval":1}`)
		default:
			_, _ = io.WriteString(w, `{"access_token":"gho_test","token_type":"bearer","scope":"gist"}`)
		}
	})
	auth := func(r *http.Request) bool { return r.Header.Get("Authorization") == "Bearer gho_test" }
	mux.HandleFunc("GET /user", func(w http.ResponseWriter, r *http.Request) {
		if !auth(r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = io.WriteString(w, `{"login":"slaynAW"}`)
	})
	mux.HandleFunc("POST /gists", func(w http.ResponseWriter, r *http.Request) {
		if !auth(r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var body struct {
			Public bool                         `json:"public"`
			Files  map[string]map[string]string `json:"files"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Public {
			http.Error(w, "gist public", http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		id := "0123456789abcdef0123456789abcdef"
		f.gists[id] = map[string]string{}
		for name, file := range body.Files {
			f.gists[id][name] = file["content"]
		}
		f.version++
		f.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":"`+id+`"}`)
	})
	mux.HandleFunc("PATCH /gists/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !auth(r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var body struct {
			Files map[string]*struct {
				Content string `json:"content"`
			} `json:"files"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		defer f.mu.Unlock()
		files, ok := f.gists[r.PathValue("id")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		for name, file := range body.Files {
			if file == nil {
				delete(files, name)
			} else {
				files[name] = file.Content
			}
		}
		f.version++
		_, _ = io.WriteString(w, `{}`)
	})
	mux.HandleFunc("GET /gists/{id}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.lastToken = r.Header.Get("Authorization")
		files, ok := f.gists[r.PathValue("id")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		etag := `"v` + string(rune('0'+f.version)) + `"`
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		out := map[string]any{}
		for name, content := range files {
			out[name] = map[string]any{"content": content, "truncated": false, "size": len(content)}
		}
		w.Header().Set("ETag", etag)
		_ = json.NewEncoder(w).Encode(map[string]any{"files": out})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	gh := &GitHub{API: srv.URL, Web: srv.URL, ClientID: "client-test", Client: srv.Client(), pollUnit: time.Millisecond}
	return f, gh
}

func TestGitHubLoginAndGists(t *testing.T) {
	fake, gh := newFakeGitHub(t)
	ctx := context.Background()
	dc, err := gh.StartLogin(ctx)
	if err != nil || dc.UserCode != "ABCD-1234" {
		t.Fatalf("code : %+v %v", dc, err)
	}
	token, err := gh.WaitLogin(ctx, dc)
	if err != nil || token != "gho_test" {
		t.Fatalf("jeton : %q %v", token, err)
	}
	if login, err := gh.User(ctx, token); err != nil || login != "slaynAW" {
		t.Fatalf("compte : %q %v", login, err)
	}
	if _, err := gh.User(ctx, "mauvais"); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("jeton refusé : %v", err)
	}

	id, err := gh.CreateGist(ctx, token, "Wake On LAN", map[string]string{"LISEZMOI.md": "chiffré"})
	if err != nil {
		t.Fatal(err)
	}
	content := `{"format":"wakeonlan-share"}`
	if err := gh.UpdateGist(ctx, token, id, map[string]*string{"acces-1.json": &content}); err != nil {
		t.Fatal(err)
	}
	snap, err := gh.FetchGist(ctx, id, "")
	if err != nil || snap.Files["acces-1.json"] != content || len(snap.Files) != 1 || snap.ETag == "" {
		t.Fatalf("lecture : %+v %v", snap, err)
	}
	if fake.lastToken != "" {
		t.Error("jeton envoyé pour une lecture publique")
	}
	// Rien de changé : réponse 304.
	if again, err := gh.FetchGist(ctx, id, snap.ETag); err != nil || !again.NotModified {
		t.Errorf("ETag : %+v %v", again, err)
	}
	// Suppression d'un fichier.
	if err := gh.UpdateGist(ctx, token, id, map[string]*string{"acces-1.json": nil}); err != nil {
		t.Fatal(err)
	}
	if snap, err := gh.FetchGist(ctx, id, snap.ETag); err != nil || len(snap.Files) != 0 {
		t.Errorf("après suppression : %+v %v", snap, err)
	}
	if _, err := gh.FetchGist(ctx, "ffffffffffffffffffffffffffffffff", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("Gist absent : %v", err)
	}
	if _, err := gh.FetchGist(ctx, "../user", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("identifiant invalide : %v", err)
	}
}

func TestGitHubLoginRefused(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /login/oauth/access_token", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"error":"access_denied"}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	gh := &GitHub{API: srv.URL, Web: srv.URL, ClientID: "c", Client: srv.Client(), pollUnit: time.Millisecond}
	if _, err := gh.WaitLogin(context.Background(), DeviceCode{DeviceCode: "d", Interval: 1, ExpiresIn: 60}); !errors.Is(err, ErrDenied) {
		t.Errorf("refus : %v", err)
	}
	if _, err := (&GitHub{}).StartLogin(context.Background()); !errors.Is(err, ErrNoClientID) {
		t.Errorf("sans identifiant : %v", err)
	}
}

func TestGitHubRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	gh := &GitHub{API: srv.URL, Client: srv.Client()}
	_, err := gh.FetchGist(context.Background(), "0123456789abcdef0123456789abcdef", "")
	if !errors.Is(err, ErrRateLimited) || !strings.Contains(err.Error(), "limite") {
		t.Errorf("limite : %v", err)
	}
}

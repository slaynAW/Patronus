//go:build !windows

// Hors Windows : mode développement. L'interface est servie sur 127.0.0.1 et s'ouvre dans un
// navigateur ; utile pour tester l'interface et le moteur (tests automatisés, Linux, macOS).
package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/slaynaw/wakeonlan/desktop/internal/app"
	"github.com/slaynaw/wakeonlan/desktop/internal/config"
	"github.com/slaynaw/wakeonlan/desktop/internal/history"
)

type devPlatform struct{}

// SaveFile laisse le navigateur proposer le téléchargement.
func (devPlatform) SaveFile(string, []byte) (string, error) { return "", nil }

func (devPlatform) OpenURL(url string) error {
	log.Printf("ouverture demandée : %s", url)
	return nil
}

func (devPlatform) ReadClipboard() (string, error) { return "", fmt.Errorf("indisponible") }

// WriteClipboard : l'interface utilise alors le presse-papiers du navigateur.
func (devPlatform) WriteClipboard(string) error { return fmt.Errorf("indisponible") }

// Relaunch : en mode développement, la nouvelle version est vérifiée mais pas installée.
func (devPlatform) Relaunch() error {
	return fmt.Errorf("mode développement : nouvelle version vérifiée, non installée")
}

func main() {
	listen := flag.String("listen", "127.0.0.1:0", "adresse d'écoute (locale uniquement)")
	dataDir := flag.String("data", defaultDataDir(), "dossier de configuration")
	flag.Parse()

	store, err := config.NewStore(*dataDir)
	if err != nil {
		log.Fatal(err)
	}
	histStore, err := history.NewStore(*dataDir)
	if err != nil {
		log.Fatal(err)
	}
	svc := app.New(app.Options{Version: version, Store: store, Platform: devPlatform{}, History: histStore, Share: shareOptions(*dataDir)})
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go svc.Run(ctx)

	token := rand.Text()
	hub := newHub()
	svc.OnChange(hub.notify)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", serveFile("index.html", "text/html; charset=utf-8"))
	mux.HandleFunc("GET /app.css", serveFile("app.css", "text/css; charset=utf-8"))
	mux.HandleFunc("GET /app.js", serveFile("app.js", "text/javascript; charset=utf-8"))
	mux.HandleFunc("GET /fonts/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if !strings.HasSuffix(name, ".woff2") {
			http.NotFound(w, r)
			return
		}
		serveFile("fonts/"+name, "font/woff2")(w, r)
	})
	mux.HandleFunc("POST /rpc", func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Token")), []byte(token)) != 1 {
			http.Error(w, "jeton invalide", http.StatusForbidden)
			return
		}
		var req struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		result, err := svc.Call(req.Method, req.Params)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": result})
	})
	mux.HandleFunc("GET /events", func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("token")), []byte(token)) != 1 {
			http.Error(w, "jeton invalide", http.StatusForbidden)
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "flux non pris en charge", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")
		ch := hub.subscribe()
		defer hub.unsubscribe(ch)
		for {
			data, _ := json.Marshal(svc.State())
			if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
				return
			}
			flusher.Flush()
			select {
			case <-r.Context().Done():
				return
			case <-ch:
				// Regroupe les changements rapprochés.
				time.Sleep(50 * time.Millisecond)
			}
		}
	})

	l, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Patronus %s (mode développement) : http://%s/#token=%s\n", version, l.Addr(), token)
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		_ = server.Close()
	}()
	if err := server.Serve(l); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func serveFile(name, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		b, err := uiFiles.ReadFile("ui/" + name)
		if err != nil {
			http.NotFound(w, nil)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(b)
	}
}

// hub diffuse les notifications de changement aux flux d'évènements ouverts.
type hub struct {
	mu   sync.Mutex
	subs map[chan struct{}]bool
}

func newHub() *hub { return &hub{subs: map[chan struct{}]bool{}} }

func (h *hub) subscribe() chan struct{} {
	ch := make(chan struct{}, 1)
	h.mu.Lock()
	h.subs[ch] = true
	h.mu.Unlock()
	return ch
}

func (h *hub) unsubscribe(ch chan struct{}) {
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
}

func (h *hub) notify() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

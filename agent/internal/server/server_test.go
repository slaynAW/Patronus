package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/internal/config"
	"github.com/slaynaw/wakeonlan/agent/internal/history"
	"github.com/slaynaw/wakeonlan/agent/internal/power"
	"github.com/slaynaw/wakeonlan/agent/protocol"
)

type recorder struct {
	mu      sync.Mutex
	actions []power.Action
	done    chan struct{}
}

func (r *recorder) Do(a power.Action, _ bool) error {
	r.mu.Lock()
	r.actions = append(r.actions, a)
	r.mu.Unlock()
	r.done <- struct{}{}
	return nil
}

func start(t *testing.T, mutate func(*config.Config)) (*config.Config, *recorder, string) {
	t.Helper()
	return startServer(t, mutate, nil)
}

func startServer(t *testing.T, mutate func(*config.Config), setup func(*Server)) (*config.Config, *recorder, string) {
	t.Helper()
	cfg, err := config.New("test", 1)
	if err != nil {
		t.Fatal(err)
	}
	if mutate != nil {
		mutate(cfg)
	}
	rec := &recorder{done: make(chan struct{}, 4)}
	srv, err := New(cfg, rec, "test", log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	if setup != nil {
		setup(srv)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = srv.Serve(ctx, ln) }()
	return cfg, rec, ln.Addr().String()
}

// exchange joue le rôle de l'application : même algorithme que le client Kotlin.
func exchange(t *testing.T, addr, key, body string) (protocol.Response, string) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		return protocol.Response{}, ""
	}
	var early protocol.Response
	if json.Unmarshal([]byte(line), &early) == nil && early.Error != "" {
		return early, ""
	}
	var hello protocol.Hello
	if err := json.Unmarshal([]byte(line), &hello); err != nil {
		t.Fatal(err)
	}
	keyBytes, _ := protocol.DecodeKey(key)
	cnonce, _ := protocol.NewNonce(protocol.ClientNonceBytes)
	req, _ := json.Marshal(protocol.Request{CNonce: cnonce, Body: body, Mac: protocol.RequestMAC(keyBytes, hello.Nonce, cnonce, body)})
	_, _ = conn.Write(append(req, '\n'))
	line, err = reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	var resp protocol.Response
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error == "" && !protocol.MACEqual(protocol.ResponseMAC(keyBytes, hello.Nonce, cnonce, resp.Body), resp.Mac) {
		t.Fatal("signature de réponse invalide")
	}
	return resp, hello.Nonce
}

func decodeBody(t *testing.T, resp protocol.Response) protocol.ResponseBody {
	t.Helper()
	var body protocol.ResponseBody
	if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
		t.Fatalf("corps illisible %q : %v", resp.Body, err)
	}
	return body
}

func TestStatusAndShutdown(t *testing.T) {
	minActionDelay = 10 * time.Millisecond
	cfg, rec, addr := start(t, nil)

	resp, _ := exchange(t, addr, cfg.Key, `{"cmd":"status"}`)
	body := decodeBody(t, resp)
	if !body.OK || body.Version != "test" || body.Hostname == "" {
		t.Fatalf("statut inattendu : %+v", body)
	}

	resp, _ = exchange(t, addr, cfg.Key, `{"cmd":"shutdown","delay":0,"force":false}`)
	if body := decodeBody(t, resp); !body.OK {
		t.Fatalf("extinction refusée : %+v", body)
	}
	select {
	case <-rec.done:
	case <-time.After(2 * time.Second):
		t.Fatal("action non exécutée")
	}
	if rec.actions[0] != power.Shutdown {
		t.Fatalf("action %v", rec.actions)
	}
}

func TestWrongKeyAndRateLimit(t *testing.T) {
	_, rec, addr := start(t, nil)
	other, _ := protocol.NewKey()
	for i := 0; i < 5; i++ {
		resp, _ := exchange(t, addr, other, `{"cmd":"shutdown"}`)
		if resp.Error != protocol.ErrUnauthorized {
			t.Fatalf("essai %d : %+v", i, resp)
		}
	}
	resp, _ := exchange(t, addr, other, `{"cmd":"status"}`)
	if resp.Error != protocol.ErrRateLimited {
		t.Fatalf("pas de blocage après 5 échecs : %+v", resp)
	}
	if len(rec.actions) != 0 {
		t.Fatal("action exécutée sans authentification")
	}
}

func TestReplayIsRejected(t *testing.T) {
	cfg, _, addr := start(t, nil)
	keyBytes, _ := protocol.DecodeKey(cfg.Key)
	// Requête valide pour un AUTRE nonce (capturée lors d'un échange précédent).
	cnonce, _ := protocol.NewNonce(protocol.ClientNonceBytes)
	oldNonce, _ := protocol.NewNonce(protocol.ServerNonceBytes)
	body := `{"cmd":"shutdown"}`
	replayed, _ := json.Marshal(protocol.Request{CNonce: cnonce, Body: body, Mac: protocol.RequestMAC(keyBytes, oldNonce, cnonce, body)})

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	reader := bufio.NewReader(conn)
	_, _ = reader.ReadString('\n')
	_, _ = conn.Write(append(replayed, '\n'))
	line, _ := reader.ReadString('\n')
	if !strings.Contains(line, protocol.ErrUnauthorized) {
		t.Fatalf("rejeu accepté : %s", line)
	}
}

func TestForbiddenCommandAndNetwork(t *testing.T) {
	cfg, _, addr := start(t, func(c *config.Config) { c.Commands = []string{"status", "sleep"} })
	resp, _ := exchange(t, addr, cfg.Key, `{"cmd":"shutdown"}`)
	if body := decodeBody(t, resp); body.OK || body.Code != "forbidden" {
		t.Fatalf("commande désactivée acceptée : %+v", body)
	}

	cfg, _, addr = start(t, func(c *config.Config) { c.Allow = []string{"10.0.0.0/8"} })
	resp, nonce := exchange(t, addr, cfg.Key, `{"cmd":"status"}`)
	if nonce != "" || resp.Body != "" {
		t.Fatal("connexion acceptée depuis un réseau non autorisé")
	}
}

func TestOversizedMessage(t *testing.T) {
	_, _, addr := start(t, nil)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	reader := bufio.NewReader(conn)
	_, _ = reader.ReadString('\n')
	_, _ = conn.Write([]byte(strings.Repeat("A", protocol.MaxLineBytes+10)))
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if line, _ := reader.ReadString('\n'); line != "" {
		t.Fatalf("réponse inattendue : %q", line)
	}
}

func TestHistoryCommand(t *testing.T) {
	minActionDelay = 10 * time.Millisecond
	journal, err := history.Open(filepath.Join(t.TempDir(), "history.json"), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	_ = journal.Started(history.Boot{ID: "x", At: time.Now().Add(-time.Hour)}, time.Time{})
	cfg, rec, addr := startServer(t, nil, func(s *Server) { s.SetHistory(journal) })

	resp, _ := exchange(t, addr, cfg.Key, `{"cmd":"sleep","delay":0}`)
	if body := decodeBody(t, resp); !body.OK {
		t.Fatalf("veille refusée : %+v", body)
	}
	<-rec.done
	resp, _ = exchange(t, addr, cfg.Key, `{"cmd":"history"}`)
	body := decodeBody(t, resp)
	if !body.OK || body.History == nil || len(body.History.Events) != 2 {
		t.Fatalf("journal inattendu : %+v", body)
	}
	cmd := body.History.Events[1]
	if body.History.Events[0].K != protocol.HistoryBoot || cmd.K != protocol.HistoryCommand || cmd.A != "sleep" || cmd.C != "127.0.0.1" {
		t.Fatalf("évènements : %+v", body.History.Events)
	}
	if body.History.From > body.History.Events[0].T {
		t.Errorf("début de couverture %d après le premier évènement", body.History.From)
	}

	// Lecture seule : autorisée avec « status », même si « history » n'est pas listé (anciennes configurations).
	cfg, _, addr = startServer(t, func(c *config.Config) { c.Commands = []string{"status"} }, func(s *Server) { s.SetHistory(journal) })
	if body := decodeBody(t, first(exchange(t, addr, cfg.Key, `{"cmd":"history"}`))); !body.OK {
		t.Fatalf("journal refusé avec status : %+v", body)
	}
	cfg, _, addr = startServer(t, func(c *config.Config) { c.Commands = []string{"sleep"} }, func(s *Server) { s.SetHistory(journal) })
	if body := decodeBody(t, first(exchange(t, addr, cfg.Key, `{"cmd":"history"}`))); body.OK || body.Code != "forbidden" {
		t.Fatalf("journal accepté sans status : %+v", body)
	}
	cfg, _, addr = start(t, nil)
	if body := decodeBody(t, first(exchange(t, addr, cfg.Key, `{"cmd":"history"}`))); body.OK || body.Code != "unsupported" {
		t.Fatalf("journal absent : %+v", body)
	}
}

func first(resp protocol.Response, _ string) protocol.Response { return resp }

func TestWakesCommand(t *testing.T) {
	minActionDelay = 10 * time.Millisecond
	journal, err := history.Open(filepath.Join(t.TempDir(), "history.json"), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	bootAt := time.Now().Add(-time.Hour)
	_ = journal.Started(history.Boot{ID: "x", At: bootAt}, time.Time{})
	cfg, rec, addr := startServer(t, func(c *config.Config) { c.Commands = []string{"status", "sleep"} }, func(s *Server) { s.SetHistory(journal) })

	wake := bootAt.Add(-time.Minute).Unix()
	resp, _ := exchange(t, addr, cfg.Key, fmt.Sprintf(`{"cmd":"wakes","wakes":[%d],"by":"  Pixel\u0007 8 de   Léa "}`, wake))
	body := decodeBody(t, resp)
	if !body.OK || body.History == nil || len(body.History.Events) != 2 {
		t.Fatalf("démarrage refusé : %+v", body)
	}
	if e := body.History.Events[0]; e.K != protocol.HistoryWake || e.T != wake || e.B != "Pixel 8 de Léa" || e.C != "127.0.0.1" {
		t.Fatalf("démarrage noté : %+v", e)
	}

	// Nom de l'appareil noté avec les commandes d'alimentation.
	if body := decodeBody(t, first(exchange(t, addr, cfg.Key, `{"cmd":"sleep","delay":0,"by":"PC-SALON"}`))); !body.OK {
		t.Fatalf("veille refusée : %+v", body)
	}
	<-rec.done
	h := journal.Snapshot()
	if e := h.Events[len(h.Events)-1]; e.K != protocol.HistoryCommand || e.B != "PC-SALON" {
		t.Fatalf("commande notée : %+v", e)
	}

	for _, bad := range []string{`{"cmd":"wakes"}`, `{"cmd":"wakes","wakes":[` + strings.Repeat("1,", protocol.MaxWakes) + `1]}`} {
		if body := decodeBody(t, first(exchange(t, addr, cfg.Key, bad))); body.OK || body.Code != "bad_request" {
			t.Errorf("requête invalide acceptée : %+v", body)
		}
	}
	// Comme la lecture du journal : refusé sans « status ».
	cfg, _, addr = startServer(t, func(c *config.Config) { c.Commands = []string{"sleep"} }, func(s *Server) { s.SetHistory(journal) })
	if body := decodeBody(t, first(exchange(t, addr, cfg.Key, fmt.Sprintf(`{"cmd":"wakes","wakes":[%d]}`, wake)))); body.OK || body.Code != "forbidden" {
		t.Fatalf("accepté sans status : %+v", body)
	}
}

func TestCleanName(t *testing.T) {
	for in, want := range map[string]string{
		"Pixel 8":               "Pixel 8",
		"  PC\tsalon  ":         "PC salon",
		"a\u200bb":              "ab",
		strings.Repeat("é", 50): strings.Repeat("é", protocol.MaxByLength),
		"":                      "",
	} {
		if got := CleanName(in); got != want {
			t.Errorf("CleanName(%q) = %q", in, got)
		}
	}
}

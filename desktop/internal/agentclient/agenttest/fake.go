// Package agenttest fournit un faux agent pour les tests (comportements normaux et défaillants).
package agenttest

import (
	"bufio"
	"encoding/json"
	"io"
	"net"
	"sync"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol"
)

// Behavior choisit le comportement du faux agent.
type Behavior int

const (
	Normal Behavior = iota
	BadResponseMAC
	Reject
	WrongProto
	Hang
	RateLimited
)

// Server est un faux agent qui écoute sur 127.0.0.1.
type Server struct {
	key      []byte
	behavior Behavior
	listener net.Listener
	mu       sync.Mutex
	history  *protocol.History
	commands []string
	requests []protocol.RequestBody
	wg       sync.WaitGroup
}

// Start démarre un faux agent.
func Start(key []byte, behavior Behavior) (*Server, error) {
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &Server{key: key, behavior: behavior, listener: l}
	s.wg.Add(1)
	go s.serve()
	return s, nil
}

// Port renvoie le port d'écoute.
func (s *Server) Port() int { return s.listener.Addr().(*net.TCPAddr).Port }

// SetHistory définit le journal renvoyé par « history » (nil : agent ancien, commande inconnue).
func (s *Server) SetHistory(h *protocol.History) {
	s.mu.Lock()
	s.history = h
	s.mu.Unlock()
}

// Commands renvoie les commandes authentifiées reçues.
func (s *Server) Commands() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.commands...)
}

// Requests renvoie les requêtes authentifiées reçues.
func (s *Server) Requests() []protocol.RequestBody {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]protocol.RequestBody(nil), s.requests...)
}

// Close arrête le faux agent.
func (s *Server) Close() {
	s.listener.Close()
	s.wg.Wait()
}

func (s *Server) serve() {
	defer s.wg.Done()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer conn.Close()
			s.handle(conn)
		}()
	}
}

func (s *Server) handle(conn net.Conn) {
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	write := func(v any) {
		b, _ := json.Marshal(v)
		_, _ = conn.Write(append(b, '\n'))
	}
	switch s.behavior {
	case Hang:
		// Muet jusqu'à ce que le client abandonne et ferme la connexion.
		_, _ = io.Copy(io.Discard, conn)
		return
	case RateLimited:
		write(protocol.Response{Error: protocol.ErrRateLimited})
		return
	}
	nonce, _ := protocol.NewNonce(protocol.ServerNonceBytes)
	proto := protocol.Proto
	if s.behavior == WrongProto {
		proto = "wolagent/99"
	}
	write(protocol.Hello{Proto: proto, Nonce: nonce})

	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return
	}
	var req protocol.Request
	if json.Unmarshal(line, &req) != nil || !protocol.MACEqual(protocol.RequestMAC(s.key, nonce, req.CNonce, req.Body), req.Mac) {
		write(protocol.Response{Error: protocol.ErrUnauthorized})
		return
	}
	var body protocol.RequestBody
	_ = json.Unmarshal([]byte(req.Body), &body)
	s.mu.Lock()
	s.commands = append(s.commands, body.Cmd)
	s.requests = append(s.requests, body)
	s.mu.Unlock()

	resp := protocol.ResponseBody{OK: true, Code: "ok", Message: "OK", Hostname: "PC-TEST", OS: "windows", Arch: "amd64",
		Version: "1.0.0", Uptime: 3600}
	if body.Cmd == protocol.CmdHistory || body.Cmd == protocol.CmdWakes {
		s.mu.Lock()
		if s.history != nil && body.Cmd == protocol.CmdWakes {
			// Agent 1.4.0 : démarrages signalés ajoutés au journal (sans le tri ni les contrôles du vrai).
			h := *s.history
			h.Events = append([]protocol.HistoryEvent(nil), h.Events...)
			for _, t := range body.Wakes {
				h.Events = append(h.Events, protocol.HistoryEvent{T: t, K: protocol.HistoryWake, C: "127.0.0.1", B: body.By})
			}
			s.history = &h
		}
		h := s.history
		s.mu.Unlock()
		if h == nil {
			resp = protocol.ResponseBody{OK: false, Code: "unsupported", Message: "commande inconnue : " + body.Cmd}
		} else {
			resp.History = h
		}
	}
	if s.behavior == Reject {
		resp = protocol.ResponseBody{OK: false, Code: "forbidden", Message: "commande désactivée"}
	}
	b, _ := json.Marshal(resp)
	mac := protocol.ResponseMAC(s.key, nonce, req.CNonce, string(b))
	if s.behavior == BadResponseMAC {
		mac = protocol.ResponseMAC(make([]byte, protocol.KeyBytes), nonce, req.CNonce, string(b))
	}
	write(protocol.Response{Body: string(b), Mac: mac})
}

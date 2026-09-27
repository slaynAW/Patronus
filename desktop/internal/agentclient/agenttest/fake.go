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
	commands []string
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

// Commands renvoie les commandes authentifiées reçues.
func (s *Server) Commands() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.commands...)
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
	s.mu.Unlock()

	resp := protocol.ResponseBody{OK: true, Code: "ok", Message: "OK", Hostname: "PC-TEST", OS: "windows", Arch: "amd64",
		Version: "1.0.0", Uptime: 3600}
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

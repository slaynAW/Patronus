// Package client ouvre une session wolagent/2 avec un agent (docs/PROTOCOLE-V2.md) : lecture du salut,
// TLS 1.3 avec l'empreinte de l'agent épinglée, puis requêtes et réponses en trames. Il sert à
// wol-agent sur le PC (administration locale), aux tests, et servira à l'application Windows.
package client

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol"
	"github.com/slaynaw/wakeonlan/agent/protocol2"
)

var (
	// ErrRateLimited : l'agent bloque cette adresse après trop d'échecs (5 minutes).
	ErrRateLimited = errors.New("trop d'échecs depuis cette adresse : réessayez dans quelques minutes")
	// ErrNotAgent : ce qui répond n'est pas un agent Patronus.
	ErrNotAgent = errors.New("ce n'est pas un agent Patronus")
	// ErrPinMismatch : l'agent ne présente pas l'empreinte épinglée. Rien n'a été envoyé.
	ErrPinMismatch = errors.New("l'empreinte de l'agent ne correspond pas à celle enregistrée : connexion refusée")
	// ErrMigration : la preuve de l'agent est fausse (autre clé, ou intermédiaire). Rien d'autre n'a
	// été envoyé.
	ErrMigration = errors.New("preuve de l'agent invalide : migration refusée")
	// ErrUnpinned : seule la migration est permise tant que l'empreinte de l'agent n'est pas prouvée.
	ErrUnpinned = errors.New("empreinte de l'agent non vérifiée : seule la migration est permise")
	// ErrBroken : la session a connu une erreur et doit être rouverte.
	ErrBroken = errors.New("session wolagent/2 interrompue")
)

// Options décrit la session à ouvrir.
type Options struct {
	// Identity est l'identité de l'appareil : clé ECDSA P-256 et son certificat.
	Identity tls.Certificate
	// Agent est l'empreinte épinglée de l'agent. Nil seulement pour une migration (§9) : l'empreinte
	// vue est alors prouvée par l'agent avant tout autre envoi.
	Agent *protocol2.Pin
	// Dial ouvre la connexion TCP (net.Dialer par défaut).
	Dial func(ctx context.Context, network, addr string) (net.Conn, error)
}

// Session est une connexion wolagent/2 établie. Une requête à la fois (§4.3).
type Session struct {
	conn   *tls.Conn
	agent  protocol2.Pin
	device protocol2.Pin
	pinned bool
	lastID int64
	broken bool
}

// Dial ouvre une session avec l'agent à l'adresse addr (« hôte:port »).
func Dial(ctx context.Context, addr string, opts Options) (*Session, error) {
	if len(opts.Identity.Certificate) == 0 {
		return nil, errors.New("identité de l'appareil absente")
	}
	device, err := protocol2.PinFromCertificate(opts.Identity.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("identité de l'appareil : %w", err)
	}
	dial := opts.Dial
	if dial == nil {
		dial = (&net.Dialer{}).DialContext
	}
	raw, err := dial(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(protocol2.HandshakeTimeout)
	}
	_ = raw.SetDeadline(deadline)
	if err := readHello(raw); err != nil {
		_ = raw.Close()
		return nil, err
	}
	var seen protocol2.Pin
	conn := tls.Client(raw, &tls.Config{
		MinVersion:       tls.VersionTLS13,
		MaxVersion:       tls.VersionTLS13,
		NextProtos:       []string{protocol2.ALPN},
		Certificates:     []tls.Certificate{opts.Identity},
		CurvePreferences: protocol2.Curves,
		// Pas d'autorité de certification : la vérification est celle de l'empreinte, ci-dessous,
		// faite pendant la poignée de main, avant tout envoi de données.
		InsecureSkipVerify: true,
		VerifyConnection: func(cs tls.ConnectionState) error {
			if cs.NegotiatedProtocol != protocol2.ALPN {
				return errors.New("protocole applicatif refusé")
			}
			if len(cs.PeerCertificates) != 1 {
				return errors.New("l'agent doit présenter un seul certificat")
			}
			pin, err := protocol2.PinFromCertificate(cs.PeerCertificates[0].Raw)
			if err != nil {
				return err
			}
			if opts.Agent != nil && pin != *opts.Agent {
				return ErrPinMismatch
			}
			seen = pin
			return nil
		},
	})
	if err := conn.HandshakeContext(ctx); err != nil {
		_ = raw.Close()
		return nil, err
	}
	_ = raw.SetDeadline(time.Time{})
	return &Session{conn: conn, agent: seen, device: device, pinned: opts.Agent != nil}, nil
}

// readHello lit la ligne de salut que l'agent envoie avant TLS (§3.1), sans rien lire au-delà.
func readHello(conn net.Conn) error {
	line := make([]byte, 0, protocol2.MaxHelloBytes)
	var b [1]byte
	for {
		if _, err := io.ReadFull(conn, b[:]); err != nil {
			return fmt.Errorf("salut de l'agent : %w", err)
		}
		if b[0] == '\n' {
			break
		}
		if len(line) == protocol2.MaxHelloBytes {
			return ErrNotAgent
		}
		line = append(line, b[0])
	}
	var hello struct {
		Proto string `json:"proto"`
		Error string `json:"error"`
	}
	switch {
	case json.Unmarshal(line, &hello) != nil:
		return ErrNotAgent
	case hello.Error == protocol.ErrRateLimited:
		return ErrRateLimited
	case hello.Error != "" || hello.Proto == "":
		return ErrNotAgent
	}
	return nil
}

// AgentPin est l'empreinte de l'agent vue dans la poignée de main.
func (s *Session) AgentPin() protocol2.Pin { return s.agent }

// DevicePin est l'empreinte de cet appareil.
func (s *Session) DevicePin() protocol2.Pin { return s.device }

// Close ferme la session.
func (s *Session) Close() error { return s.conn.Close() }

// Do envoie une requête (son identifiant est attribué ici) et lit sa réponse.
func (s *Session) Do(ctx context.Context, req protocol2.Request) (protocol2.Response, error) {
	if !s.pinned && req.Cmd != protocol2.CmdMigrate {
		return protocol2.Response{}, ErrUnpinned
	}
	return s.do(ctx, req)
}

func (s *Session) do(ctx context.Context, req protocol2.Request) (protocol2.Response, error) {
	if s.broken {
		return protocol2.Response{}, ErrBroken
	}
	s.lastID++
	req.ID = s.lastID
	payload, err := json.Marshal(req)
	if err != nil {
		return protocol2.Response{}, err
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(protocol2.IdleTimeout)
	}
	_ = s.conn.SetDeadline(deadline)
	fail := func(err error) (protocol2.Response, error) {
		s.broken = true
		_ = s.conn.Close()
		return protocol2.Response{}, err
	}
	if err := protocol2.WriteFrame(s.conn, payload, protocol2.MaxRequestBytes); err != nil {
		return fail(err)
	}
	raw, err := protocol2.ReadFrame(s.conn, protocol2.MaxResponseBytes)
	if err != nil {
		return fail(err)
	}
	var resp protocol2.Response
	if err := json.Unmarshal(raw, &resp); err != nil {
		return fail(fmt.Errorf("réponse illisible : %w", err))
	}
	if resp.ID != req.ID {
		return fail(fmt.Errorf("réponse à la requête %d au lieu de %d", resp.ID, req.ID))
	}
	return resp, nil
}

// Migrate fait passer en wolagent/2 un appareil appairé en wolagent/1 (§9), sur une session ouverte
// sans empreinte épinglée ; key est la clé wolagent/1 du PC. La preuve de l'agent est vérifiée avant
// d'envoyer celle de l'appareil : si elle est fausse, la session est fermée et rien d'autre n'est
// envoyé. En cas de succès, AgentPin est prouvée et peut être épinglée.
func (s *Session) Migrate(ctx context.Context, key []byte, name string) (protocol2.Response, error) {
	nc := protocol2.NewNonce()
	first, err := s.do(ctx, protocol2.Request{Cmd: protocol2.CmdMigrate, Nonce: nc.String(), Name: name})
	if err != nil || !first.OK {
		return first, err
	}
	na, err1 := protocol2.ParseNonce(first.Nonce)
	pa, err2 := protocol2.ParseProof(first.Proof)
	if err1 != nil || err2 != nil || !protocol2.AgentProof(key, s.agent, s.device, nc, na).Equal(pa) {
		s.broken = true
		_ = s.conn.Close()
		return protocol2.Response{}, ErrMigration
	}
	s.pinned = true
	pd := protocol2.DeviceProof(key, s.agent, s.device, nc, na)
	return s.do(ctx, protocol2.Request{Cmd: protocol2.CmdMigrate, Proof: pd.String()})
}

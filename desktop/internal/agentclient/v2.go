package agentclient

import (
	"context"
	"errors"
	"net"
	"strconv"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol"
	"github.com/slaynaw/wakeonlan/agent/protocol2"
	"github.com/slaynaw/wakeonlan/agent/protocol2/client"
	"github.com/slaynaw/wakeonlan/desktop/internal/model"
	"github.com/slaynaw/wakeonlan/desktop/internal/netdial"
)

// Migration est le résultat du passage d'un PC en wolagent/2.
type Migration struct {
	// Role : rôle donné à cet ordinateur par l'agent (« control » : les droits de la clé).
	Role string
}

// Migrate enregistre cet ordinateur auprès de l'agent (docs/PROTOCOLE-V2.md, §9), sur une session
// wolagent/2 où l'empreinte de l'agent, agent.Pin, est exigée ; la clé wolagent/1 agent.Key prouve
// les droits de cet ordinateur. L'agent prouve d'abord qu'il connaît la clé : sinon, rien d'autre
// n'est envoyé.
func (c *Client) Migrate(ctx context.Context, host string, agent model.AgentSettings) (Migration, error) {
	key := model.DecodeAgentKey(agent.Key)
	if key == nil || !agent.HasPin() {
		return Migration{}, &Error{Code: NoKey}
	}
	var out Migration
	err := c.session(ctx, host, agent, func(ctx context.Context, s *client.Session) error {
		resp, err := s.Migrate(ctx, key, c.by())
		if errors.Is(err, client.ErrMigration) {
			return &Error{Code: Protocol, Detail: "preuve de l'agent invalide (clé changée, ou interception)"}
		}
		if err != nil {
			return err
		}
		if _, err := fromV2(resp); err != nil {
			return err
		}
		out.Role = resp.Role
		return nil
	})
	return out, err
}

// Pairing est le résultat de l'appairage de cet ordinateur avec un code.
type Pairing struct {
	// Role : rôle donné à cet ordinateur par l'agent (celui de l'invitation).
	Role string
	// Already : cet ordinateur était déjà enregistré par l'agent ; le code n'a pas servi.
	Already bool
}

// Pair enregistre cet ordinateur auprès de l'agent avec un code d'appairage (docs/PROTOCOLE-V2.md,
// §6.3 et §7.3), sur une session où l'empreinte de l'agent lue dans le lien, agent.Pin, est exigée :
// le code n'est envoyé qu'à cet agent. Le nom de cet ordinateur est celui que l'agent enregistre.
func (c *Client) Pair(ctx context.Context, host string, agent model.AgentSettings, code string) (Pairing, error) {
	normalized, err := protocol2.NormalizeCode(code)
	if err != nil || !agent.HasPin() {
		return Pairing{}, &Error{Code: PairingFailed, Detail: "code d'appairage ou empreinte de l'agent invalide"}
	}
	var out Pairing
	err = c.session(ctx, host, agent, func(ctx context.Context, s *client.Session) error {
		resp, err := s.Do(ctx, protocol2.Request{Cmd: protocol2.CmdPair, Code: normalized, Name: c.by()})
		switch {
		case err != nil:
			return err
		case !resp.OK && resp.Code == protocol2.CodePairingFailed:
			return &Error{Code: PairingFailed, Detail: resp.Message}
		case !resp.OK && resp.Code == protocol2.CodeAlreadyPaired:
			out.Role, out.Already = resp.Role, true
			return nil
		}
		if _, err := fromV2(resp); err != nil {
			return err
		}
		out.Role = resp.Role
		return nil
	})
	return out, err
}

// exchange2 mène un échange wolagent/2 : une requête sur une session où l'empreinte de l'agent est
// exigée.
func (c *Client) exchange2(ctx context.Context, host string, agent model.AgentSettings, request protocol.RequestBody) (*responseBody, error) {
	var body *responseBody
	err := c.session(ctx, host, agent, func(ctx context.Context, s *client.Session) error {
		resp, err := s.Do(ctx, protocol2.Request{Cmd: request.Cmd, Delay: request.Delay, Force: request.Force,
			Wakes: request.Wakes, Day: request.Day})
		if err != nil {
			return err
		}
		body, err = fromV2(resp)
		return err
	})
	return body, err
}

// session ouvre une session wolagent/2 avec l'agent (empreinte agent.Pin exigée pendant la poignée de
// main TLS), y mène run, puis la ferme. Les délais sont ceux de wolagent/1 : connexion, puis deux
// lectures au plus.
func (c *Client) session(ctx context.Context, host string, agent model.AgentSettings, run func(context.Context, *client.Session) error) error {
	return c.sessionWith(ctx, host, agent.Port, agent.Pin, "", c.ConnectTimeout+2*c.ReadTimeout, run)
}

// sessionWith ouvre une session sur le transport de wolagent/2 avec le service host:port, dont
// l'empreinte pinText est exigée, en proposant protocol (wolagent/2 si vide ; patronus-hub/1 pour le
// hub), et y mène run ; budget borne la session entière.
func (c *Client) sessionWith(ctx context.Context, host string, port int, pinText, protocol string, budget time.Duration,
	run func(context.Context, *client.Session) error) error {
	pin, err := protocol2.ParsePin(pinText)
	if err != nil {
		return &Error{Code: Protocol, Detail: "empreinte enregistrée invalide"}
	}
	if c.Identity == nil {
		return &Error{Code: NoIdentity}
	}
	cert, err := c.Identity()
	if err != nil {
		return &Error{Code: NoIdentity, Detail: err.Error()}
	}
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	var dialErr error
	s, err := client.Dial(ctx, net.JoinHostPort(host, strconv.Itoa(port)), client.Options{
		Identity: cert,
		Agent:    &pin,
		Protocol: protocol,
		Dial: func(ctx context.Context, _, addr string) (net.Conn, error) {
			conn, err := netdial.Dial(ctx, addr, c.ConnectTimeout)
			dialErr = err
			return conn, err
		},
	})
	switch {
	case err == nil:
	case dialErr != nil:
		return dialError(dialErr)
	case errors.Is(err, client.ErrPinMismatch):
		return &Error{Code: PinMismatch}
	case errors.Is(err, client.ErrRateLimited):
		return &Error{Code: RateLimited}
	case errors.Is(err, client.ErrUnexpectedProtocol):
		return &Error{Code: OtherService, Detail: err.Error()}
	default:
		// La machine a répondu (connexion établie), mais pas comme un agent wolagent/2 attendu.
		return &Error{Code: Protocol, Detail: err.Error()}
	}
	defer s.Close()
	// Annulation : fermer la session débloque immédiatement les lectures en cours.
	stop := context.AfterFunc(ctx, func() { s.Close() })
	defer stop()
	if err := run(ctx, s); err != nil {
		var e *Error
		if errors.As(err, &e) {
			return e
		}
		return &Error{Code: Protocol, Detail: err.Error()}
	}
	return nil
}

// fromV2 traduit une réponse wolagent/2 dans la forme commune aux deux protocoles.
func fromV2(resp protocol2.Response) (*responseBody, error) {
	switch {
	case resp.Code == "":
		return nil, protocolError("réponse sans code")
	case !resp.OK && resp.Code == protocol2.CodeUnknownDevice:
		return nil, &Error{Code: Unregistered, Detail: resp.Message}
	case !resp.OK:
		return nil, &Error{Code: Rejected, Detail: resp.Message, Reason: resp.Code}
	}
	ok, code := resp.OK, resp.Code
	return &responseBody{
		OK: &ok, Code: &code, Message: resp.Message,
		Hostname: resp.Hostname, OS: resp.OS, Arch: resp.Arch, Version: resp.Version, Uptime: resp.Uptime,
		History: resp.History, Metrics: resp.Metrics, Temperatures: resp.Temperatures, Disks: resp.Disks, Measures: resp.Measures,
		Specs: resp.Specs, Protocols: resp.Protocols, Role: resp.Role, proto: protocol2.ALPN,
	}, nil
}

package agentclient

import (
	"context"
	"time"

	"github.com/slaynaw/wakeonlan/agent/hubprotocol"
	"github.com/slaynaw/wakeonlan/agent/protocol2"
	"github.com/slaynaw/wakeonlan/agent/protocol2/client"
)

// Hub sur le NAS (docs/PROTOCOLE-HUB.md) : même transport que wolagent/2, avec le protocole
// patronus-hub/1, son port et ses commandes. L'empreinte du hub est toujours exigée : rien n'est
// envoyé à un service qui en présente une autre. Les refus du hub sont des erreurs de ce paquet
// (Unregistered, Rejected avec le code du hub), comme pour un agent.

// DefaultHubTimeout borne une session avec le hub, après la connexion : plus qu'avec un agent, car le
// hub s'appaire lui-même à l'agent d'un PC qu'on lui confie (hub.pcs.add).
const DefaultHubTimeout = 20 * time.Second

// Hub est le hub tel que cet ordinateur le connaît.
type Hub struct {
	Host string
	Port int
	Pin  string // empreinte complète
}

// HubPair enregistre cet ordinateur auprès du hub avec le code d'un lien d'appairage (§5.2 de
// docs/HUB.md), sous le nom de cet ordinateur.
func (c *Client) HubPair(ctx context.Context, hub Hub, code string) (Pairing, error) {
	normalized, err := protocol2.NormalizeCode(code)
	if err != nil {
		return Pairing{}, &Error{Code: PairingFailed, Detail: "code d'appairage invalide"}
	}
	var out Pairing
	err = c.HubSession(ctx, hub, func(ctx context.Context, ops HubOps) error {
		var resp hubprotocol.Response
		if err := ops.s.Call(ctx, hubprotocol.Request{Cmd: protocol2.CmdPair, Code: normalized, Name: c.by()}, &resp); err != nil {
			return err
		}
		switch {
		case !resp.OK && resp.Code == protocol2.CodePairingFailed:
			return &Error{Code: PairingFailed, Detail: resp.Message}
		case !resp.OK && resp.Code == protocol2.CodeAlreadyPaired:
			out.Role, out.Already = resp.Role, true
			return nil
		}
		if err := hubRefusal(resp); err != nil {
			return err
		}
		out.Role = resp.Role
		return nil
	})
	return out, err
}

// HubOps mène des commandes sur une session ouverte par HubSession.
type HubOps struct{ s *client.Session }

// HubSession ouvre une session patronus-hub/1 avec le hub et y mène run : plusieurs commandes, une
// seule poignée de main.
func (c *Client) HubSession(ctx context.Context, hub Hub, run func(context.Context, HubOps) error) error {
	budget := c.HubTimeout
	if budget <= 0 {
		budget = DefaultHubTimeout
	}
	return c.sessionWith(ctx, hub.Host, hub.Port, hub.Pin, hubprotocol.ALPN, c.ConnectTimeout+budget, func(ctx context.Context, s *client.Session) error {
		return run(ctx, HubOps{s: s})
	})
}

// Do envoie une requête au hub ; un refus devient une erreur (Unregistered pour un appareil retiré,
// Rejected sinon, avec le code du hub).
func (h HubOps) Do(ctx context.Context, req hubprotocol.Request) (hubprotocol.Response, error) {
	var resp hubprotocol.Response
	if err := h.s.Call(ctx, req, &resp); err != nil {
		return resp, err
	}
	return resp, hubRefusal(resp)
}

// hubRefusal traduit une réponse du hub en erreur, ou nil si elle est un succès.
func hubRefusal(resp hubprotocol.Response) error {
	switch {
	case resp.Code == "":
		return protocolError("réponse sans code")
	case !resp.OK && resp.Code == protocol2.CodeUnknownDevice:
		return &Error{Code: Unregistered, Detail: resp.Message}
	case !resp.OK:
		return &Error{Code: Rejected, Detail: resp.Message, Reason: resp.Code}
	}
	return nil
}

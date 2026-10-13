package agentclient

import (
	"context"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol2"
	"github.com/slaynaw/wakeonlan/agent/protocol2/client"
	"github.com/slaynaw/wakeonlan/desktop/internal/model"
)

// Administration des appareils autorisés par un agent (docs/PROTOCOLE-V2.md, §6.2), réservée aux
// appareils de rôle « admin » et toujours en wolagent/2 (empreinte de l'agent exigée). Un refus de
// l'agent est une erreur Rejected dont Reason donne la cause : « insufficient_role », « last_admin »
// (dernier administrateur : seulement sur le PC), « bad_request »…

// Admin est l'état d'un agent vu par un administrateur : appareils autorisés et invitations en
// attente (sans leur code).
type Admin struct {
	Devices []protocol2.DeviceInfo `json:"devices"`
	Invites []protocol2.InviteInfo `json:"invites"`
}

// Admin lit les appareils et les invitations en attente, sur une seule session.
func (c *Client) Admin(ctx context.Context, host string, agent model.AgentSettings) (Admin, error) {
	out := Admin{Devices: []protocol2.DeviceInfo{}, Invites: []protocol2.InviteInfo{}}
	err := c.admin(ctx, host, agent, func(ctx context.Context, s *client.Session) error {
		devices, err := adminDo(ctx, s, protocol2.Request{Cmd: protocol2.CmdDevices})
		if err != nil {
			return err
		}
		invites, err := adminDo(ctx, s, protocol2.Request{Cmd: protocol2.CmdInvites})
		if err != nil {
			return err
		}
		if devices.Devices != nil {
			out.Devices = devices.Devices
		}
		if invites.Invites != nil {
			out.Invites = invites.Invites
		}
		return nil
	})
	return out, err
}

// Invite crée une invitation à usage unique pour le rôle donné, valable ttl ; name est le nom que
// portera l'appareil invité ("" : celui qu'il donnera). La réponse est la seule à contenir le code.
func (c *Client) Invite(ctx context.Context, host string, agent model.AgentSettings, role string, ttl time.Duration, name string) (protocol2.InviteInfo, error) {
	var out protocol2.InviteInfo
	err := c.admin(ctx, host, agent, func(ctx context.Context, s *client.Session) error {
		resp, err := adminDo(ctx, s, protocol2.Request{Cmd: protocol2.CmdInvite, Role: role, TTL: int64(ttl / time.Second), Name: name})
		if err != nil {
			return err
		}
		if resp.Invite == nil || resp.Invite.Code == "" {
			return protocolError("invitation absente de la réponse")
		}
		out = *resp.Invite
		return nil
	})
	return out, err
}

// Revoke retire un appareil ; SetRole change son rôle ; Rename son nom ; CancelInvite annule une
// invitation.
func (c *Client) Revoke(ctx context.Context, host string, agent model.AgentSettings, device string) error {
	return c.adminChange(ctx, host, agent, protocol2.Request{Cmd: protocol2.CmdDevicesRevoke, Device: device})
}

func (c *Client) SetRole(ctx context.Context, host string, agent model.AgentSettings, device, role string) error {
	return c.adminChange(ctx, host, agent, protocol2.Request{Cmd: protocol2.CmdDevicesRole, Device: device, Role: role})
}

func (c *Client) Rename(ctx context.Context, host string, agent model.AgentSettings, device, name string) error {
	return c.adminChange(ctx, host, agent, protocol2.Request{Cmd: protocol2.CmdDevicesRename, Device: device, Name: name})
}

func (c *Client) CancelInvite(ctx context.Context, host string, agent model.AgentSettings, id string) error {
	return c.adminChange(ctx, host, agent, protocol2.Request{Cmd: protocol2.CmdInvitesCancel, Invite: id})
}

func (c *Client) adminChange(ctx context.Context, host string, agent model.AgentSettings, req protocol2.Request) error {
	return c.admin(ctx, host, agent, func(ctx context.Context, s *client.Session) error {
		_, err := adminDo(ctx, s, req)
		return err
	})
}

// admin mène run sur une session wolagent/2 : l'administration n'existe pas en wolagent/1.
func (c *Client) admin(ctx context.Context, host string, agent model.AgentSettings, run func(context.Context, *client.Session) error) error {
	if !agent.HasPin() {
		return &Error{Code: NoKey, Detail: "PC pas encore passé en wolagent/2"}
	}
	return c.session(ctx, host, agent, run)
}

// adminDo envoie une requête d'administration ; un refus devient une erreur (Unregistered pour un
// appareil retiré, Rejected sinon, avec le code de l'agent).
func adminDo(ctx context.Context, s *client.Session, req protocol2.Request) (protocol2.Response, error) {
	resp, err := s.Do(ctx, req)
	if err != nil {
		return resp, err
	}
	if _, err := fromV2(resp); err != nil {
		return resp, err
	}
	return resp, nil
}

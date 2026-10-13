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
	var out Admin
	err := c.AdminSession(ctx, host, agent, func(ctx context.Context, ops AdminOps) (err error) {
		out, err = ops.State(ctx)
		return err
	})
	return out, err
}

// Invite crée une invitation à usage unique pour le rôle donné, valable ttl ; name est le nom que
// portera l'appareil invité ("" : celui qu'il donnera). La réponse est la seule à contenir le code.
func (c *Client) Invite(ctx context.Context, host string, agent model.AgentSettings, role string, ttl time.Duration, name string) (protocol2.InviteInfo, error) {
	var out protocol2.InviteInfo
	err := c.AdminSession(ctx, host, agent, func(ctx context.Context, ops AdminOps) (err error) {
		out, err = ops.Invite(ctx, role, ttl, name, "")
		return err
	})
	return out, err
}

// AdminOps mène des commandes d'administration sur une session ouverte par AdminSession.
type AdminOps struct{ s *client.Session }

// AdminSession ouvre une session d'administration wolagent/2 et y mène run : plusieurs commandes,
// une seule poignée de main.
func (c *Client) AdminSession(ctx context.Context, host string, agent model.AgentSettings, run func(context.Context, AdminOps) error) error {
	return c.admin(ctx, host, agent, func(ctx context.Context, s *client.Session) error { return run(ctx, AdminOps{s: s}) })
}

// State lit les appareils autorisés et les invitations en attente.
func (a AdminOps) State(ctx context.Context) (Admin, error) {
	out := Admin{Devices: []protocol2.DeviceInfo{}, Invites: []protocol2.InviteInfo{}}
	devices, err := adminDo(ctx, a.s, protocol2.Request{Cmd: protocol2.CmdDevices})
	if err != nil {
		return out, err
	}
	invites, err := adminDo(ctx, a.s, protocol2.Request{Cmd: protocol2.CmdInvites})
	if err != nil {
		return out, err
	}
	if devices.Devices != nil {
		out.Devices = devices.Devices
	}
	if invites.Invites != nil {
		out.Invites = invites.Invites
	}
	return out, nil
}

// Invite crée une invitation ; device, s'il n'est pas vide, la réserve à cette empreinte.
func (a AdminOps) Invite(ctx context.Context, role string, ttl time.Duration, name, device string) (protocol2.InviteInfo, error) {
	resp, err := adminDo(ctx, a.s, protocol2.Request{Cmd: protocol2.CmdInvite, Role: role, TTL: int64(ttl / time.Second), Name: name, Device: device})
	if err != nil {
		return protocol2.InviteInfo{}, err
	}
	if resp.Invite == nil || resp.Invite.Code == "" {
		return protocol2.InviteInfo{}, protocolError("invitation absente de la réponse")
	}
	return *resp.Invite, nil
}

// Revoke, SetRole et CancelInvite : comme les méthodes du même nom de Client, sur la session ouverte.
func (a AdminOps) Revoke(ctx context.Context, device string) error {
	_, err := adminDo(ctx, a.s, protocol2.Request{Cmd: protocol2.CmdDevicesRevoke, Device: device})
	return err
}

func (a AdminOps) SetRole(ctx context.Context, device, role string) error {
	_, err := adminDo(ctx, a.s, protocol2.Request{Cmd: protocol2.CmdDevicesRole, Device: device, Role: role})
	return err
}

func (a AdminOps) CancelInvite(ctx context.Context, id string) error {
	_, err := adminDo(ctx, a.s, protocol2.Request{Cmd: protocol2.CmdInvitesCancel, Invite: id})
	return err
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

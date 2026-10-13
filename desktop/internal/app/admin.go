package app

import (
	"context"
	"strings"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol2"
	"github.com/slaynaw/wakeonlan/desktop/internal/agentclient"
	"github.com/slaynaw/wakeonlan/desktop/internal/diag"
	"github.com/slaynaw/wakeonlan/desktop/internal/model"
	"github.com/slaynaw/wakeonlan/desktop/internal/share"
)

// Administration des appareils autorisés par un PC (docs/PROTOCOLE-V2.md, §6.2 et §7.5), depuis cet
// ordinateur quand l'agent lui a donné le rôle « admin ». Les journaux ne contiennent ni code
// d'appairage ni empreinte complète.

// adminTimeout borne un échange d'administration (une session, deux requêtes au plus).
const adminTimeout = 15 * time.Second

// adminTarget renvoie le PC et ses réglages d'agent wolagent/2, ou la cause de l'impossibilité.
func (s *Service) adminTarget(id string) (model.Device, map[string]any) {
	d, ok := s.ownDevice(id)
	if !ok || d.Agent == nil || !d.Agent.HasPin() || !d.HasHost() {
		return d, map[string]any{"ok": false, "code": agentclient.NoKey}
	}
	return d, nil
}

func adminFailure(err error) map[string]any {
	return map[string]any{"ok": false, "code": agentclient.CodeOf(err), "reason": agentclient.ReasonOf(err)}
}

// shortPin renvoie l'empreinte courte d'une empreinte complète (texte vide si elle est invalide).
func shortPin(pin string) string {
	p, err := protocol2.ParsePin(pin)
	if err != nil {
		return ""
	}
	return p.Short()
}

// agentAdmin lit les appareils autorisés par le PC et ses invitations en attente.
func (s *Service) agentAdmin(id string) map[string]any {
	d, fail := s.adminTarget(id)
	if fail != nil {
		return fail
	}
	ctx, cancel := context.WithTimeout(context.Background(), adminTimeout)
	defer cancel()
	a, err := s.agent.Admin(ctx, d.Host, *d.Agent)
	if err != nil {
		diag.Warn(areaAgent, "appareils autorisés par « %s » : %s (%v)", d.Name, agentclient.CodeOf(err), err)
		return adminFailure(err)
	}
	s.agentAnswered(id)
	devices := make([]map[string]any, 0, len(a.Devices))
	for _, dev := range a.Devices {
		devices = append(devices, map[string]any{"device": dev.Device, "short": shortPin(dev.Device), "name": dev.Name,
			"role": dev.Role, "added": dev.Added, "via": dev.Via, "seen": dev.Seen, "self": dev.Self})
	}
	invites := make([]map[string]any, 0, len(a.Invites))
	for _, inv := range a.Invites {
		invites = append(invites, map[string]any{"id": inv.ID, "role": inv.Role, "name": inv.Name, "expires": inv.Expires})
	}
	return map[string]any{"ok": true, "devices": devices, "invites": invites}
}

// agentAdminChange applique une modification : « revoke », « role » ou « rename » d'un appareil,
// « cancel » d'une invitation.
func (s *Service) agentAdminChange(id, action, target, value string) map[string]any {
	d, fail := s.adminTarget(id)
	if fail != nil {
		return fail
	}
	ctx, cancel := context.WithTimeout(context.Background(), adminTimeout)
	defer cancel()
	var err error
	var done string
	switch action {
	case "revoke":
		err, done = s.agent.Revoke(ctx, d.Host, *d.Agent, target), "appareil "+shortPin(target)+" retiré"
	case "role":
		if _, ok := protocol2.ParseRole(value); !ok {
			return map[string]any{"ok": false, "code": agentclient.Rejected, "reason": protocol2.CodeBadRequest}
		}
		err, done = s.agent.SetRole(ctx, d.Host, *d.Agent, target, value), "appareil "+shortPin(target)+" : rôle "+value
	case "rename":
		value = strings.TrimSpace(value)
		if value == "" {
			return map[string]any{"ok": false, "code": agentclient.Rejected, "reason": protocol2.CodeBadRequest}
		}
		err, done = s.agent.Rename(ctx, d.Host, *d.Agent, target, value), "appareil "+shortPin(target)+" renommé"
	case "cancel":
		err, done = s.agent.CancelInvite(ctx, d.Host, *d.Agent, target), "invitation annulée"
	default:
		return map[string]any{"ok": false, "code": agentclient.Rejected, "reason": protocol2.CodeBadRequest}
	}
	if err != nil {
		diag.Warn(areaAgent, "administration de « %s » (%s) : %s %s (%v)", d.Name, action, agentclient.CodeOf(err),
			agentclient.ReasonOf(err), err)
		return adminFailure(err)
	}
	diag.Info(areaAgent, "« %s » : %s", d.Name, done)
	return map[string]any{"ok": true}
}

// agentInvite crée une invitation pour un autre appareil, avec le rôle et la validité choisis, et
// renvoie le lien d'appairage v=2 et son QR code (adresse du PC telle que cet ordinateur la connaît).
func (s *Service) agentInvite(id, role string, ttlSeconds int64, name string) map[string]any {
	d, fail := s.adminTarget(id)
	if fail != nil {
		return fail
	}
	parsedRole, ok := protocol2.ParseRole(role)
	if !ok || ttlSeconds < protocol2.MinInviteTTL || ttlSeconds > protocol2.MaxInviteTTL {
		return map[string]any{"ok": false, "code": agentclient.Rejected, "reason": protocol2.CodeBadRequest}
	}
	pin, err := protocol2.ParsePin(d.Agent.Pin)
	if err != nil {
		return map[string]any{"ok": false, "code": agentclient.Protocol}
	}
	ctx, cancel := context.WithTimeout(context.Background(), adminTimeout)
	defer cancel()
	inv, err := s.agent.Invite(ctx, d.Host, *d.Agent, role, time.Duration(ttlSeconds)*time.Second, strings.TrimSpace(name))
	if err != nil {
		diag.Warn(areaAgent, "invitation pour « %s » : %s %s (%v)", d.Name, agentclient.CodeOf(err), agentclient.ReasonOf(err), err)
		return adminFailure(err)
	}
	code, err := protocol2.NormalizeCode(inv.Code)
	if err != nil || inv.Agent != pin.String() {
		return map[string]any{"ok": false, "code": agentclient.Protocol}
	}
	// Adresse MAC pour le réveil, sauf valeur qu'un lien refuse (§7.1).
	mac := strings.ToUpper(d.MAC.String())
	if mac == "00:00:00:00:00:00" || mac == "FF:FF:FF:FF:FF:FF" {
		mac = ""
	}
	link := protocol2.Link{Name: d.Name, Host: d.Host, Port: d.Agent.Port, MAC: mac, Agent: pin, Code: code, Role: parsedRole,
		Expires: inv.Expires}.String()
	qr, err := share.QRCodeSVG(link)
	if err != nil {
		return map[string]any{"ok": false, "code": agentclient.Protocol, "reason": err.Error()}
	}
	diag.Info(areaAgent, "« %s » : invitation créée (rôle %s, %d min)", d.Name, role, ttlSeconds/60)
	return map[string]any{"ok": true, "code": protocol2.FormatCode(code), "role": role, "expires": inv.Expires,
		"link": link, "qr": qr}
}

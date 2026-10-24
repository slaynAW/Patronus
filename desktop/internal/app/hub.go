package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/slaynaw/wakeonlan/agent/hubprotocol"
	"github.com/slaynaw/wakeonlan/agent/protocol2"
	"github.com/slaynaw/wakeonlan/desktop/internal/agentclient"
	"github.com/slaynaw/wakeonlan/desktop/internal/diag"
	"github.com/slaynaw/wakeonlan/desktop/internal/hubstore"
	"github.com/slaynaw/wakeonlan/desktop/internal/share"
)

// Hub sur le NAS (docs/HUB.md) : appairage de cette installation, état du hub et, pour un
// administrateur du hub, ses appareils autorisés, ses invitations et sa durée de conservation. Un seul
// hub à la fois. Les journaux ne contiennent ni code d'appairage ni empreinte complète.

const areaHub = "hub"

// hubTimeout borne un échange avec le hub (une session, quelques requêtes).
const hubTimeout = 15 * time.Second

// Durée de conservation de l'archive du hub, en jours (docs/PROTOCOLE-HUB.md, §6.4).
const (
	hubMinRetention = 90
	hubMaxRetention = 1095
)

// errNoHub : aucun hub appairé.
var errNoHub = errors.New("aucun hub appairé")

// HubView est le hub appairé, pour l'interface (nil : aucun).
type HubView struct {
	Name   string `json:"name"`
	Host   string `json:"host"`
	Port   int    `json:"port"`
	Short  string `json:"short"` // empreinte courte du hub
	Role   string `json:"role"`
	Paired int64  `json:"paired"`
	// Unseen : évènements du hub non vus (« Pendant votre absence ») ; More : il y en a davantage.
	Unseen int  `json:"unseen"`
	More   bool `json:"more,omitempty"`
}

// loadHub lit le hub appairé au démarrage (dans le dossier des données, comme l'identité).
func (s *Service) loadHub(dir string) {
	store, err := hubstore.NewStore(dir)
	if err != nil {
		diag.Error(areaHub, "hub : %v", err)
		store = hubstore.Memory()
	}
	s.hubStore = store
	h, err := store.Load()
	if err != nil {
		diag.Error(areaHub, "%v", err)
	}
	s.hub = h
}

// hubView renvoie le hub appairé pour l'interface.
func (s *Service) hubView() *HubView {
	s.hubMu.Lock()
	defer s.hubMu.Unlock()
	if s.hub == nil {
		return nil
	}
	v := &HubView{Name: s.hub.Name, Host: s.hub.Host, Port: s.hub.Port, Short: shortPin(s.hub.Pin), Role: s.hub.Role,
		Paired: s.hub.Paired}
	if s.hubEvents.pin == s.hub.Pin {
		v.Unseen, v.More = len(s.hubEvents.events), s.hubEvents.more
	}
	return v
}

// currentHub renvoie une copie du hub appairé.
func (s *Service) currentHub() (hubstore.Hub, bool) {
	s.hubMu.Lock()
	defer s.hubMu.Unlock()
	if s.hub == nil {
		return hubstore.Hub{}, false
	}
	return *s.hub, true
}

// saveHub enregistre le hub (nil : l'oublie) puis prévient l'interface.
func (s *Service) saveHub(h *hubstore.Hub) error {
	s.hubMu.Lock()
	err := s.hubStore.Save(h)
	if err == nil {
		s.hub = h
	}
	s.hubMu.Unlock()
	if err != nil {
		diag.Error(areaHub, "enregistrement du hub : %v", err)
		return err
	}
	s.notify()
	return nil
}

// updateHub applique change au hub appairé, s'il est toujours celui d'empreinte pin.
func (s *Service) updateHub(pin string, change func(*hubstore.Hub)) {
	s.hubMu.Lock()
	if s.hub == nil || s.hub.Pin != pin {
		s.hubMu.Unlock()
		return
	}
	next := *s.hub
	change(&next)
	if next == *s.hub {
		s.hubMu.Unlock()
		return
	}
	err := s.hubStore.Save(&next)
	if err == nil {
		s.hub = &next
	}
	s.hubMu.Unlock()
	if err != nil {
		diag.Error(areaHub, "enregistrement du hub : %v", err)
		return
	}
	s.notify()
}

func hubTarget(h hubstore.Hub) agentclient.Hub {
	return agentclient.Hub{Host: h.Host, Port: h.Port, Pin: h.Pin}
}

func hubFailure(err error) map[string]any {
	if errors.Is(err, errNoHub) {
		return map[string]any{"ok": false, "code": "NO_HUB"}
	}
	return adminFailure(err)
}

// hubPair appaire cette installation au hub d'un lien patronus://hub (docs/HUB.md, §5.2).
func (s *Service) hubPair(text string) map[string]any {
	text = strings.TrimSpace(text)
	link, err := hubprotocol.ParseLink(text)
	if err != nil {
		reason := "invalid"
		if _, agentErr := protocol2.ParseLink(text); agentErr == nil {
			reason = "agent_link"
		}
		return map[string]any{"ok": false, "code": "LINK", "reason": reason}
	}
	if link.Expires != 0 && s.now().Unix() >= link.Expires {
		return map[string]any{"ok": false, "code": "LINK", "reason": "expired"}
	}
	if current, ok := s.currentHub(); ok && current.Pin != link.Agent.String() {
		return map[string]any{"ok": false, "code": "HUB_PAIRED"}
	}
	hub := agentclient.Hub{Host: link.Host, Port: link.Port, Pin: link.Agent.String()}
	ctx, cancel := context.WithTimeout(context.Background(), hubTimeout)
	defer cancel()
	paired, err := s.agent.HubPair(ctx, hub, link.Code)
	if err != nil {
		diag.Warn(areaHub, "appairage au hub %s : %s (%v)", link.Agent.Short(), agentclient.CodeOf(err), err)
		return adminFailure(err)
	}
	name := link.Name
	if info, err := s.hubStatus(ctx, hub); err == nil && info.Hub != nil && info.Hub.Name != "" {
		name = info.Hub.Name
	}
	if name == "" {
		name = "Hub"
	}
	h := &hubstore.Hub{Name: name, Host: link.Host, Port: link.Port, Pin: link.Agent.String(), Role: paired.Role,
		Paired: s.now().Unix()}
	if err := s.saveHub(h); err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	diag.Info(areaHub, "hub « %s » appairé (%s, rôle %s%s)", name, link.Agent.Short(), paired.Role,
		map[bool]string{true: ", déjà connu", false: ""}[paired.Already])
	s.refreshHubSoon()
	return map[string]any{"ok": true, "name": name, "role": paired.Role, "already": paired.Already}
}

// hubStatus lit hub.status sur une session.
func (s *Service) hubStatus(ctx context.Context, hub agentclient.Hub) (hubprotocol.Response, error) {
	var out hubprotocol.Response
	err := s.agent.HubSession(ctx, hub, func(ctx context.Context, ops agentclient.HubOps) (err error) {
		out, err = ops.Do(ctx, hubprotocol.Request{Cmd: hubprotocol.CmdStatus})
		return err
	})
	return out, err
}

// hubInfo lit l'état du hub et, pour un administrateur, ses appareils autorisés, ses invitations et
// ses réglages, sur une seule session.
func (s *Service) hubInfo() map[string]any {
	h, ok := s.currentHub()
	if !ok {
		return hubFailure(errNoHub)
	}
	ctx, cancel := context.WithTimeout(context.Background(), hubTimeout)
	defer cancel()
	var status, devices, invites, settings hubprotocol.Response
	err := s.agent.HubSession(ctx, hubTarget(h), func(ctx context.Context, ops agentclient.HubOps) (err error) {
		if status, err = ops.Do(ctx, hubprotocol.Request{Cmd: hubprotocol.CmdStatus}); err != nil {
			return err
		}
		if status.Role != protocol2.RoleAdmin.String() {
			return nil
		}
		if devices, err = ops.Do(ctx, hubprotocol.Request{Cmd: protocol2.CmdDevices}); err != nil {
			return err
		}
		if invites, err = ops.Do(ctx, hubprotocol.Request{Cmd: protocol2.CmdInvites}); err != nil {
			return err
		}
		settings, err = ops.Do(ctx, hubprotocol.Request{Cmd: hubprotocol.CmdSettings})
		return err
	})
	if err != nil {
		diag.Warn(areaHub, "état du hub « %s » : %s %s (%v)", h.Name, agentclient.CodeOf(err), agentclient.ReasonOf(err), err)
		return hubFailure(err)
	}
	if status.Hub == nil {
		return map[string]any{"ok": false, "code": agentclient.Protocol}
	}
	s.keepSnapshot(h, status)
	out := map[string]any{
		"ok": true, "role": status.Role, "name": status.Hub.Name, "version": status.Hub.Version,
		"started": status.Hub.Started, "short": shortPin(h.Pin), "pcs": s.hubPCViews(status.PCs, status.States),
	}
	if status.Role == protocol2.RoleAdmin.String() {
		list := make([]map[string]any, 0, len(devices.Devices))
		for _, dev := range devices.Devices {
			list = append(list, map[string]any{"device": dev.Device, "short": shortPin(dev.Device), "name": dev.Name,
				"role": dev.Role, "added": dev.Added, "via": dev.Via, "seen": dev.Seen, "self": dev.Self})
		}
		pending := make([]map[string]any, 0, len(invites.Invites))
		for _, inv := range invites.Invites {
			pending = append(pending, map[string]any{"id": inv.ID, "role": inv.Role, "name": inv.Name, "expires": inv.Expires})
		}
		out["devices"], out["invites"] = list, pending
		if settings.Settings != nil {
			out["retention"] = settings.Settings.Retention
		}
	}
	return out
}

// hubAdminChange applique une modification sur le hub : « revoke », « role » ou « rename » d'un
// appareil, « cancel » d'une invitation.
func (s *Service) hubAdminChange(action, device, value string) map[string]any {
	h, ok := s.currentHub()
	if !ok {
		return hubFailure(errNoHub)
	}
	var req hubprotocol.Request
	var done string
	switch action {
	case "revoke":
		req, done = hubprotocol.Request{Cmd: protocol2.CmdDevicesRevoke, Device: device}, "appareil "+shortPin(device)+" retiré"
	case "role":
		if _, ok := hubprotocol.ParseRole(value); !ok {
			return map[string]any{"ok": false, "code": agentclient.Rejected, "reason": protocol2.CodeBadRequest}
		}
		req, done = hubprotocol.Request{Cmd: protocol2.CmdDevicesRole, Device: device, Role: value}, "appareil "+shortPin(device)+" : rôle "+value
	case "rename":
		value = strings.TrimSpace(value)
		if value == "" {
			return map[string]any{"ok": false, "code": agentclient.Rejected, "reason": protocol2.CodeBadRequest}
		}
		req, done = hubprotocol.Request{Cmd: protocol2.CmdDevicesRename, Device: device, Name: value}, "appareil "+shortPin(device)+" renommé"
	case "cancel":
		req, done = hubprotocol.Request{Cmd: protocol2.CmdInvitesCancel, Invite: device}, "invitation annulée"
	default:
		return map[string]any{"ok": false, "code": agentclient.Rejected, "reason": protocol2.CodeBadRequest}
	}
	ctx, cancel := context.WithTimeout(context.Background(), hubTimeout)
	defer cancel()
	err := s.agent.HubSession(ctx, hubTarget(h), func(ctx context.Context, ops agentclient.HubOps) error {
		_, err := ops.Do(ctx, req)
		return err
	})
	if err != nil {
		diag.Warn(areaHub, "administration du hub (%s) : %s %s (%v)", action, agentclient.CodeOf(err), agentclient.ReasonOf(err), err)
		return hubFailure(err)
	}
	diag.Info(areaHub, "hub « %s » : %s", h.Name, done)
	if id := s.identity.Existing(); id != nil && device == id.Pin.String() {
		switch action {
		case "role":
			s.updateHub(h.Pin, func(n *hubstore.Hub) { n.Role = value })
		case "revoke":
			// Cet ordinateur s'est retiré lui-même : le hub ne le connaît plus.
			_ = s.saveHub(nil)
		}
	}
	return map[string]any{"ok": true}
}

// hubInvite crée sur le hub une invitation pour un autre appareil, et renvoie le lien patronus://hub
// et son QR code (adresse du hub telle que cet ordinateur la connaît).
func (s *Service) hubInvite(role string, ttlSeconds int64, name string) map[string]any {
	h, ok := s.currentHub()
	if !ok {
		return hubFailure(errNoHub)
	}
	parsedRole, ok := hubprotocol.ParseRole(role)
	if !ok || ttlSeconds < protocol2.MinInviteTTL || ttlSeconds > protocol2.MaxInviteTTL {
		return map[string]any{"ok": false, "code": agentclient.Rejected, "reason": protocol2.CodeBadRequest}
	}
	pin, err := protocol2.ParsePin(h.Pin)
	if err != nil {
		return map[string]any{"ok": false, "code": agentclient.Protocol}
	}
	ctx, cancel := context.WithTimeout(context.Background(), hubTimeout)
	defer cancel()
	var resp hubprotocol.Response
	err = s.agent.HubSession(ctx, hubTarget(h), func(ctx context.Context, ops agentclient.HubOps) (err error) {
		resp, err = ops.Do(ctx, hubprotocol.Request{Cmd: protocol2.CmdInvite, Role: role, TTL: ttlSeconds, Name: strings.TrimSpace(name)})
		return err
	})
	if err != nil {
		diag.Warn(areaHub, "invitation sur le hub : %s %s (%v)", agentclient.CodeOf(err), agentclient.ReasonOf(err), err)
		return hubFailure(err)
	}
	if resp.Invite == nil {
		return map[string]any{"ok": false, "code": agentclient.Protocol}
	}
	code, err := protocol2.NormalizeCode(resp.Invite.Code)
	if err != nil || resp.Invite.Hub != pin.String() {
		return map[string]any{"ok": false, "code": agentclient.Protocol}
	}
	link := protocol2.Link{Name: h.Name, Host: h.Host, Port: h.Port, Agent: pin, Code: code, Role: parsedRole,
		Expires: resp.Invite.Expires}.Format(hubprotocol.LinkFormat)
	qr, err := share.QRCodeSVG(link)
	if err != nil {
		return map[string]any{"ok": false, "code": agentclient.Protocol, "reason": err.Error()}
	}
	diag.Info(areaHub, "hub « %s » : invitation créée (rôle %s, %d min)", h.Name, role, ttlSeconds/60)
	return map[string]any{"ok": true, "code": protocol2.FormatCode(code), "role": role, "expires": resp.Invite.Expires,
		"link": link, "qr": qr}
}

// hubRetention change la durée de conservation de l'archive du hub, en jours.
func (s *Service) hubRetention(days int) map[string]any {
	h, ok := s.currentHub()
	if !ok {
		return hubFailure(errNoHub)
	}
	if days < hubMinRetention || days > hubMaxRetention {
		return map[string]any{"ok": false, "code": agentclient.Rejected, "reason": protocol2.CodeBadRequest}
	}
	ctx, cancel := context.WithTimeout(context.Background(), hubTimeout)
	defer cancel()
	var resp hubprotocol.Response
	err := s.agent.HubSession(ctx, hubTarget(h), func(ctx context.Context, ops agentclient.HubOps) (err error) {
		resp, err = ops.Do(ctx, hubprotocol.Request{Cmd: hubprotocol.CmdSettings, Retention: days})
		return err
	})
	if err != nil {
		diag.Warn(areaHub, "réglages du hub : %s %s (%v)", agentclient.CodeOf(err), agentclient.ReasonOf(err), err)
		return hubFailure(err)
	}
	if resp.Settings == nil {
		return map[string]any{"ok": false, "code": agentclient.Protocol}
	}
	diag.Info(areaHub, "hub « %s » : conservation de %d jours", h.Name, resp.Settings.Retention)
	return map[string]any{"ok": true, "retention": resp.Settings.Retention}
}

// hubForget oublie le hub sur cet ordinateur, après s'être retiré de ses appareils autorisés quand c'est
// possible : administrateur du hub (seul un administrateur retire un appareil), mais pas le dernier,
// et hub joignable. revoked indique si le hub l'a retiré ; sinon, un administrateur du hub le retire.
func (s *Service) hubForget() map[string]any {
	h, ok := s.currentHub()
	if !ok {
		return hubFailure(errNoHub)
	}
	revoked := false
	if id := s.identity.Existing(); id != nil && h.Role == protocol2.RoleAdmin.String() {
		ctx, cancel := context.WithTimeout(context.Background(), hubTimeout)
		err := s.agent.HubSession(ctx, hubTarget(h), func(ctx context.Context, ops agentclient.HubOps) error {
			_, err := ops.Do(ctx, hubprotocol.Request{Cmd: protocol2.CmdDevicesRevoke, Device: id.Pin.String()})
			return err
		})
		cancel()
		switch {
		case err == nil, agentclient.CodeOf(err) == agentclient.Unregistered:
			revoked = true
		default:
			diag.Warn(areaHub, "retrait de cet ordinateur du hub « %s » : %s %s (%v)", h.Name, agentclient.CodeOf(err),
				agentclient.ReasonOf(err), err)
		}
	}
	if err := s.saveHub(nil); err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	diag.Info(areaHub, "hub « %s » oublié%s", h.Name, map[bool]string{true: " (retiré du hub)", false: ""}[revoked])
	return map[string]any{"ok": true, "revoked": revoked}
}

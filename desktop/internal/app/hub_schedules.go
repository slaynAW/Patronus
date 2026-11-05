package app

import (
	"context"
	"errors"

	"github.com/slaynaw/wakeonlan/agent/hubprotocol"
	"github.com/slaynaw/wakeonlan/agent/protocol2"
	"github.com/slaynaw/wakeonlan/desktop/internal/agentclient"
	"github.com/slaynaw/wakeonlan/desktop/internal/diag"
	"github.com/slaynaw/wakeonlan/desktop/internal/model"
)

// Programmations du hub (docs/PROTOCOLE-HUB.md, §6.7) : réveil, et extinction d'un PC confié s'il est
// inactif. L'extinction demande au hub le rôle « control » sur l'agent : cet ordinateur, administrateur
// du PC, le lui donne avant de l'activer et le lui retire quand il la désactive (docs/HUB.md, §4). Le hub
// ne le demande jamais lui-même.

// hubSchedulesOf lit les programmations du hub, par identifiant de PC ; un hub qui ne les sert pas encore
// n'en a aucune.
func hubSchedulesOf(ctx context.Context, ops agentclient.HubOps) (map[string]hubprotocol.PCSchedule, string, error) {
	resp, err := ops.Do(ctx, hubprotocol.Request{Cmd: hubprotocol.CmdSchedules})
	if err != nil {
		if agentclient.ReasonOf(err) == protocol2.CodeUnsupported {
			return nil, "", nil
		}
		return nil, "", err
	}
	out := map[string]hubprotocol.PCSchedule{}
	for _, ps := range resp.Schedules {
		out[ps.PC] = ps
	}
	tz := ""
	if resp.Settings != nil {
		tz = resp.Settings.Timezone
	}
	return out, tz, nil
}

// scheduleView décrit une programmation pour l'interface (nil : aucune).
func scheduleView(ps hubprotocol.PCSchedule, found bool) map[string]any {
	if !found || (ps.Wake == nil && ps.Shutdown == nil) {
		return nil
	}
	slot := func(s *hubprotocol.Slot) any {
		if s == nil {
			return nil
		}
		return map[string]any{"on": s.On, "days": s.Days, "at": s.At, "next": s.Next}
	}
	return map[string]any{"wake": slot(ps.Wake), "shutdown": slot(ps.Shutdown)}
}

// localDeviceOf renvoie le PC de cet ordinateur dont l'agent a l'empreinte complète agent.
func (s *Service) localDeviceOf(agent string) (model.Device, bool) {
	for _, d := range s.ownDevices() {
		if agent != "" && d.Agent != nil && d.Agent.Pin == agent && d.HasHost() {
			return d, true
		}
	}
	return model.Device{}, false
}

// hubSchedule remplace la programmation du PC pcID (identifiant du hub) : wake, shutdown (nil : retirée).
// Activer l'extinction donne d'abord au hub le rôle « control » sur l'agent (step « agent » en cas de
// refus) ; la désactiver le lui retire ensuite (agent : « withdrawn », ou « failed » à faire depuis
// « Appareils autorisés » du PC).
func (s *Service) hubSchedule(pcID string, wake, shutdown *hubprotocol.Slot) map[string]any {
	h, ok := s.currentHub()
	if !ok {
		return hubFailure(errNoHub)
	}
	for _, slot := range []*hubprotocol.Slot{wake, shutdown} {
		if slot != nil {
			if err := hubprotocol.ValidSlot(*slot); err != nil {
				return map[string]any{"ok": false, "code": agentclient.Rejected, "reason": protocol2.CodeBadRequest}
			}
		}
	}
	pcs, _ := s.hubPCs()
	var pc hubprotocol.PCInfo
	for _, p := range pcs {
		if p.ID == pcID {
			pc = p
		}
	}
	if pc.ID == "" {
		return map[string]any{"ok": false, "code": agentclient.Rejected, "reason": protocol2.CodeBadRequest}
	}
	local, known := s.localDeviceOf(pc.Agent)
	wantControl := shutdown != nil && shutdown.On
	setRole := func(role protocol2.Role) error {
		if !known {
			return errNotAdminOfPC
		}
		ctx, cancel := context.WithTimeout(context.Background(), adminTimeout)
		defer cancel()
		return s.agent.SetRole(ctx, local.Host, *local.Agent, h.Pin, role.String())
	}
	granted := false
	if wantControl && pc.Role != protocol2.RoleControl.String() {
		if pc.Agent == "" {
			return map[string]any{"ok": false, "code": agentclient.Rejected, "reason": protocol2.CodeBadRequest}
		}
		if err := setRole(protocol2.RoleControl); err != nil {
			diag.Warn(areaHub, "rôle control du hub sur « %s » : %s %s (%v)", pc.Name, agentclient.CodeOf(err), agentclient.ReasonOf(err), err)
			out := map[string]any{"ok": false, "code": "NOT_ADMIN_OF_PC", "step": "agent"}
			if !errors.Is(err, errNotAdminOfPC) {
				out = adminFailure(err)
				out["step"] = "agent"
			}
			return out
		}
		granted = true
		diag.Info(areaHub, "rôle control donné au hub sur « %s » (extinction programmée)", pc.Name)
	}
	var req hubprotocol.Request
	req.Cmd, req.PC = hubprotocol.CmdSchedulesSet, pcID
	if wake != nil || shutdown != nil {
		req.Schedule = &hubprotocol.Schedule{Wake: wake, Shutdown: shutdown}
	}
	ctx, cancel := context.WithTimeout(context.Background(), hubTimeout)
	defer cancel()
	var resp hubprotocol.Response
	err := s.agent.HubSession(ctx, hubTarget(h), func(ctx context.Context, ops agentclient.HubOps) (err error) {
		resp, err = ops.Do(ctx, req)
		return err
	})
	if err != nil {
		diag.Warn(areaHub, "programmation de « %s » sur le hub : %s %s (%v)", pc.Name, agentclient.CodeOf(err), agentclient.ReasonOf(err), err)
		if granted {
			// Programmation refusée : le hub n'a pas besoin du rôle donné juste avant.
			if rerr := setRole(protocol2.RoleWake); rerr != nil {
				diag.Warn(areaHub, "rôle control du hub sur « %s » non retiré : %v", pc.Name, rerr)
			}
		}
		out := hubFailure(err)
		out["step"] = "hub"
		return out
	}
	out := map[string]any{"ok": true, "agent": "none"}
	if resp.Schedule != nil {
		out["schedule"] = scheduleView(*resp.Schedule, true)
	}
	switch {
	case granted:
		out["agent"] = "granted"
	case !wantControl && pc.Role == protocol2.RoleControl.String():
		// Extinction désactivée : le hub ne doit plus pouvoir éteindre le PC.
		if err := setRole(protocol2.RoleWake); err != nil {
			diag.Warn(areaHub, "rôle control du hub sur « %s » non retiré : %s %s (%v)", pc.Name, agentclient.CodeOf(err), agentclient.ReasonOf(err), err)
			out["agent"] = "failed"
		} else {
			out["agent"] = "withdrawn"
			diag.Info(areaHub, "rôle control retiré au hub sur « %s »", pc.Name)
		}
	}
	diag.Info(areaHub, "programmation de « %s » enregistrée sur le hub « %s »", pc.Name, h.Name)
	s.refreshHubSoon()
	return out
}

// hubTimezone change le fuseau horaire des programmations du hub (nom IANA).
func (s *Service) hubTimezone(tz string) map[string]any {
	h, ok := s.currentHub()
	if !ok {
		return hubFailure(errNoHub)
	}
	if tz == "" || len(tz) > 64 {
		return map[string]any{"ok": false, "code": agentclient.Rejected, "reason": protocol2.CodeBadRequest}
	}
	ctx, cancel := context.WithTimeout(context.Background(), hubTimeout)
	defer cancel()
	var resp hubprotocol.Response
	err := s.agent.HubSession(ctx, hubTarget(h), func(ctx context.Context, ops agentclient.HubOps) (err error) {
		resp, err = ops.Do(ctx, hubprotocol.Request{Cmd: hubprotocol.CmdSettings, Timezone: tz})
		return err
	})
	if err != nil {
		diag.Warn(areaHub, "fuseau horaire du hub : %s %s (%v)", agentclient.CodeOf(err), agentclient.ReasonOf(err), err)
		return hubFailure(err)
	}
	if resp.Settings == nil {
		return map[string]any{"ok": false, "code": agentclient.Protocol}
	}
	diag.Info(areaHub, "hub « %s » : fuseau horaire %s", h.Name, resp.Settings.Timezone)
	return map[string]any{"ok": true, "timezone": resp.Settings.Timezone}
}

// errNotAdminOfPC : cet ordinateur ne connaît pas le PC confié (il ne peut pas changer le rôle du hub).
var errNotAdminOfPC = errors.New("PC inconnu de cet ordinateur")

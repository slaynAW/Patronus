package app

import (
	"context"
	"net/netip"
	"strings"
	"time"

	"github.com/slaynaw/wakeonlan/agent/hubprotocol"
	"github.com/slaynaw/wakeonlan/agent/protocol2"
	"github.com/slaynaw/wakeonlan/desktop/internal/agentclient"
	"github.com/slaynaw/wakeonlan/desktop/internal/diag"
	"github.com/slaynaw/wakeonlan/desktop/internal/hubstore"
	"github.com/slaynaw/wakeonlan/desktop/internal/model"
)

// PC confiés au hub (docs/HUB.md, §5.3 et §5.4). Un PC avec agent est confié par cet ordinateur quand
// il administre le PC et le hub : l'agent crée une invitation réservée à l'empreinte du hub, au rôle
// « wake », que le hub utilise pour s'appairer lui-même. Un appareil sans agent (box, imprimante) est
// ajouté par un administrateur du hub. Le hub est relu en arrière-plan, fenêtre affichée, pour
// l'interface et pour relayer le réveil.

const (
	// hubEvery est l'intervalle entre deux relevés du hub, fenêtre affichée.
	hubEvery = 30 * time.Second
	// hubInviteTTL : validité de l'invitation de l'agent transmise au hub (elle sert aussitôt).
	hubInviteTTL = 10 * time.Minute
	// hubWakeTimeout borne la demande de réveil au hub, faite en même temps que l'envoi local.
	hubWakeTimeout = 5 * time.Second
)

// hubSnapshot est le dernier relevé du hub (verrou hubMu).
type hubSnapshot struct {
	pin     string // hub relevé
	pcs     []hubprotocol.PCInfo
	states  []hubprotocol.PCState
	checked time.Time
}

// DeviceHubView indique qu'un PC est confié au hub (DeviceView.Hub), et son état vu par le hub.
type DeviceHubView struct {
	PC string `json:"pc"` // identifiant du PC sur le hub
	// Role est le rôle du hub sur l'agent du PC (view, wake, ou control pour une extinction programmée).
	Role string `json:"role"`
	// State, Checked (secondes Unix, horloge du hub) et Problem : dernier relevé du hub (§6.2).
	State   string `json:"state,omitempty"`
	Checked int64  `json:"checked,omitempty"`
	Problem string `json:"problem,omitempty"`
}

// runHub relit le hub toutes les hubEvery tant que la fenêtre est affichée, et dès qu'on le demande.
func (s *Service) runHub(ctx context.Context) {
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		case <-s.hubWake:
		}
		s.mu.Lock()
		active := s.windowActive && s.pageVisible
		s.mu.Unlock()
		if _, paired := s.currentHub(); paired && active {
			s.refreshHub(ctx)
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(hubEvery)
	}
}

// refreshHubSoon demande un relevé du hub sans attendre (après un changement de ses PC).
func (s *Service) refreshHubSoon() {
	select {
	case s.hubWake <- struct{}{}:
	default:
	}
}

// refreshHub relit l'état du hub et de ses PC.
func (s *Service) refreshHub(ctx context.Context) {
	h, ok := s.currentHub()
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, hubTimeout)
	defer cancel()
	var status, events hubprotocol.Response
	err := s.agent.HubSession(ctx, hubTarget(h), func(ctx context.Context, ops agentclient.HubOps) (err error) {
		if status, err = ops.Do(ctx, hubprotocol.Request{Cmd: hubprotocol.CmdStatus}); err != nil {
			return err
		}
		events, err = ops.Do(ctx, hubprotocol.Request{Cmd: hubprotocol.CmdEvents, Since: eventsSince(h)})
		return err
	})
	// Un échec n'est noté au journal que quand sa cause change (hub injoignable des heures durant).
	cause := ""
	if err != nil {
		cause = string(agentclient.CodeOf(err)) + " " + agentclient.ReasonOf(err)
	}
	s.hubMu.Lock()
	changed := cause != s.hubFailed
	s.hubFailed = cause
	s.hubMu.Unlock()
	if err != nil {
		if changed {
			diag.Warn(areaHub, "relevé du hub « %s » : %s (%v)", h.Name, cause, err)
		}
		return
	}
	if changed {
		diag.Info(areaHub, "hub « %s » de nouveau joint", h.Name)
	}
	s.keepEvents(h, events)
	s.keepSnapshot(h, status)
}

// keepSnapshot garde le relevé hub.status du hub h, et le rôle et le nom qu'il annonce.
func (s *Service) keepSnapshot(h hubstore.Hub, status hubprotocol.Response) {
	s.hubMu.Lock()
	s.hubSnap = hubSnapshot{pin: h.Pin, pcs: status.PCs, states: status.States, checked: s.now()}
	s.hubMu.Unlock()
	s.updateHub(h.Pin, func(n *hubstore.Hub) {
		if status.Role != "" {
			n.Role = status.Role
		}
		if status.Hub != nil {
			if name := strings.TrimSpace(status.Hub.Name); name != "" {
				n.Name = name
			}
		}
	})
	s.notify()
}

// hubPCs renvoie les PC du dernier relevé du hub appairé, avec leur état.
func (s *Service) hubPCs() ([]hubprotocol.PCInfo, []hubprotocol.PCState) {
	s.hubMu.Lock()
	defer s.hubMu.Unlock()
	if s.hub == nil || s.hubSnap.pin != s.hub.Pin {
		return nil, nil
	}
	return s.hubSnap.pcs, s.hubSnap.states
}

// hubDevices renvoie, par empreinte complète de l'agent, les PC confiés au hub appairé.
func (s *Service) hubDevices() map[string]DeviceHubView {
	pcs, states := s.hubPCs()
	byID := map[string]hubprotocol.PCState{}
	for _, st := range states {
		byID[st.ID] = st
	}
	out := map[string]DeviceHubView{}
	for _, p := range pcs {
		if p.Agent != "" {
			st := byID[p.ID]
			out[p.Agent] = DeviceHubView{PC: p.ID, Role: p.Role, State: st.State, Checked: st.Checked, Problem: st.Problem}
		}
	}
	return out
}

// hubPCOf renvoie le PC du hub qui surveille l'agent du PC d.
func (s *Service) hubPCOf(d model.Device) (hubprotocol.PCInfo, bool) {
	if d.Agent == nil || !d.Agent.HasPin() {
		return hubprotocol.PCInfo{}, false
	}
	pcs, _ := s.hubPCs()
	for _, p := range pcs {
		if p.Agent == d.Agent.Pin {
			return p, true
		}
	}
	return hubprotocol.PCInfo{}, false
}

// hubPCViews décrit les PC du hub pour sa fiche, avec leur état et le PC de cet ordinateur qui leur
// correspond.
func (s *Service) hubPCViews(pcs []hubprotocol.PCInfo, states []hubprotocol.PCState) []map[string]any {
	local := map[string]string{}
	for _, d := range s.ownDevices() {
		if d.Agent != nil && d.Agent.HasPin() {
			local[d.Agent.Pin] = d.ID
		}
	}
	byID := map[string]hubprotocol.PCState{}
	for _, st := range states {
		byID[st.ID] = st
	}
	out := make([]map[string]any, 0, len(pcs))
	for _, p := range pcs {
		st, ok := byID[p.ID]
		if !ok {
			st.State = hubprotocol.StateUnknown
		}
		v := map[string]any{"id": p.ID, "name": p.Name, "host": p.Host, "mac": p.MAC, "added": p.Added,
			"state": st.State, "since": st.Since, "checked": st.Checked, "latency": st.Latency, "problem": st.Problem}
		if p.Agent != "" {
			v["agent"], v["role"], v["local"] = shortPin(p.Agent), p.Role, local[p.Agent]
		}
		if p.Probe != nil {
			v["probe"], v["port"] = p.Probe.Kind, p.Probe.Port
		}
		out = append(out, v)
	}
	return out
}

// hubEntrust confie le PC id au hub (§5.3) : invitation de l'agent réservée à l'empreinte du hub, au
// rôle « wake », puis hub.pcs.add. step indique, en cas d'échec, l'étape refusée (« agent » ou « hub »).
func (s *Service) hubEntrust(id string) map[string]any {
	h, ok := s.currentHub()
	if !ok {
		return hubFailure(errNoHub)
	}
	d, fail := s.adminTarget(id)
	if fail != nil {
		return fail
	}
	hubPin, err := protocol2.ParsePin(h.Pin)
	if err != nil {
		return map[string]any{"ok": false, "code": agentclient.Protocol}
	}
	if _, already := s.hubPCOf(d); already {
		return map[string]any{"ok": false, "code": "ALREADY_ENTRUSTED"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*hubTimeout)
	defer cancel()
	var inv protocol2.InviteInfo
	err = s.agent.AdminSession(ctx, d.Host, *d.Agent, func(ctx context.Context, ops agentclient.AdminOps) (err error) {
		inv, err = ops.Invite(ctx, protocol2.RoleWake.String(), hubInviteTTL, "", hubPin.String())
		return err
	})
	if err != nil {
		diag.Warn(areaHub, "invitation du hub par « %s » : %s %s (%v)", d.Name, agentclient.CodeOf(err), agentclient.ReasonOf(err), err)
		out := adminFailure(err)
		out["step"] = "agent"
		return out
	}
	var resp hubprotocol.Response
	err = s.agent.HubSession(ctx, hubTarget(h), func(ctx context.Context, ops agentclient.HubOps) (err error) {
		resp, err = ops.Do(ctx, hubprotocol.Request{Cmd: hubprotocol.CmdPCsAdd, Name: d.Name, Host: d.Host, Port: d.Agent.Port,
			MAC: wakeMAC(d), Agent: d.Agent.Pin, Code: inv.Code})
		return err
	})
	if err != nil {
		diag.Warn(areaHub, "« %s » confié au hub « %s » : %s %s (%v)", d.Name, h.Name, agentclient.CodeOf(err), agentclient.ReasonOf(err), err)
		// L'invitation, réservée au hub, ne sert plus : annulée (sinon elle expire d'elle-même).
		cctx, ccancel := context.WithTimeout(context.Background(), adminTimeout)
		if cerr := s.agent.CancelInvite(cctx, d.Host, *d.Agent, inv.ID); cerr != nil {
			diag.Warn(areaHub, "annulation de l'invitation du hub sur « %s » : %v", d.Name, cerr)
		}
		ccancel()
		out := hubFailure(err)
		out["step"] = "hub"
		return out
	}
	if resp.PC == nil {
		return map[string]any{"ok": false, "code": agentclient.Protocol, "step": "hub"}
	}
	diag.Info(areaHub, "« %s » confié au hub « %s » (rôle du hub %s)", d.Name, h.Name, resp.PC.Role)
	s.refreshHubSoon()
	return map[string]any{"ok": true, "role": resp.PC.Role}
}

// wakeMAC renvoie l'adresse MAC du PC pour le réveil par le hub, sauf valeur qu'il refuse.
func wakeMAC(d model.Device) string {
	mac := strings.ToUpper(d.MAC.String())
	if mac == "00:00:00:00:00:00" || mac == "FF:FF:FF:FF:FF:FF" {
		return ""
	}
	return mac
}

// hubRemovePC retire un PC ou un appareil du hub. Pour un PC avec agent que cet ordinateur administre,
// le hub est aussi retiré des appareils autorisés du PC (il ne peut pas s'en retirer lui-même) :
// agent vaut « revoked », « failed » (PC éteint ou rôle insuffisant : à faire sur le PC), ou « none ».
func (s *Service) hubRemovePC(pcID string) map[string]any {
	h, ok := s.currentHub()
	if !ok {
		return hubFailure(errNoHub)
	}
	ctx, cancel := context.WithTimeout(context.Background(), hubTimeout)
	defer cancel()
	var resp hubprotocol.Response
	err := s.agent.HubSession(ctx, hubTarget(h), func(ctx context.Context, ops agentclient.HubOps) (err error) {
		resp, err = ops.Do(ctx, hubprotocol.Request{Cmd: hubprotocol.CmdPCsRemove, PC: pcID})
		return err
	})
	if err != nil {
		diag.Warn(areaHub, "retrait d'un PC du hub « %s » : %s %s (%v)", h.Name, agentclient.CodeOf(err), agentclient.ReasonOf(err), err)
		return hubFailure(err)
	}
	if resp.PC == nil {
		return map[string]any{"ok": false, "code": agentclient.Protocol}
	}
	removed := *resp.PC
	diag.Info(areaHub, "« %s » retiré du hub « %s »", removed.Name, h.Name)
	s.refreshHubSoon()
	out := map[string]any{"ok": true, "agent": "none", "hub": shortPin(h.Pin)}
	if removed.Agent == "" {
		return out
	}
	out["agent"] = "failed"
	for _, d := range s.ownDevices() {
		if d.Agent == nil || d.Agent.Pin != removed.Agent || !d.HasHost() {
			continue
		}
		actx, acancel := context.WithTimeout(context.Background(), adminTimeout)
		err := s.agent.Revoke(actx, d.Host, *d.Agent, h.Pin)
		acancel()
		if err == nil || agentclient.ReasonOf(err) == protocol2.CodeBadRequest {
			// Appareil inconnu de l'agent : le hub en était déjà retiré.
			out["agent"] = "revoked"
			diag.Info(areaHub, "hub retiré des appareils autorisés de « %s »", d.Name)
		} else {
			diag.Warn(areaHub, "retrait du hub sur « %s » : %s %s (%v)", d.Name, agentclient.CodeOf(err), agentclient.ReasonOf(err), err)
		}
		break
	}
	return out
}

// hubAddDevice ajoute au hub un appareil sans agent (§5.4) : nom, adresse, sonde (« tcp » avec un
// port, « http » ou « https »), MAC facultative pour le réveil.
func (s *Service) hubAddDevice(name, host, mac, kind string, port int) map[string]any {
	h, ok := s.currentHub()
	if !ok {
		return hubFailure(errNoHub)
	}
	invalid := func(field string) map[string]any {
		return map[string]any{"ok": false, "code": "INVALID", "field": field}
	}
	name, host, mac = strings.TrimSpace(name), strings.TrimSpace(host), strings.TrimSpace(mac)
	if protocol2.CleanName(name, protocol2.MaxPCNameLength) == "" {
		return invalid("name")
	}
	if !protocol2.ValidHost(host) {
		return invalid("host")
	}
	if mac != "" {
		normalized, ok := protocol2.NormalizeMAC(mac)
		if !ok {
			return invalid("mac")
		}
		mac = normalized
	}
	probe := &hubprotocol.Probe{Kind: kind}
	switch kind {
	case hubprotocol.ProbeTCP:
		if port < 1 || port > 65535 {
			return invalid("port")
		}
		probe.Port = port
	case hubprotocol.ProbeHTTP, hubprotocol.ProbeHTTPS:
		if port != 0 && (port < 1 || port > 65535) {
			return invalid("port")
		}
		probe.Port = port
	default:
		return invalid("probe")
	}
	ctx, cancel := context.WithTimeout(context.Background(), hubTimeout)
	defer cancel()
	err := s.agent.HubSession(ctx, hubTarget(h), func(ctx context.Context, ops agentclient.HubOps) error {
		_, err := ops.Do(ctx, hubprotocol.Request{Cmd: hubprotocol.CmdPCsAdd, Name: name, Host: host, MAC: mac, Probe: probe})
		return err
	})
	if err != nil {
		diag.Warn(areaHub, "appareil ajouté au hub « %s » : %s %s (%v)", h.Name, agentclient.CodeOf(err), agentclient.ReasonOf(err), err)
		return hubFailure(err)
	}
	diag.Info(areaHub, "appareil « %s » ajouté au hub « %s » (sonde %s)", name, h.Name, kind)
	s.refreshHubSoon()
	return map[string]any{"ok": true}
}

// relayWake demande au hub de démarrer le PC d, quand cet ordinateur n'est pas sur le réseau du PC
// (VPN, autre réseau) : le hub envoie le paquet magique depuis le réseau du NAS (§6.6). Il faut que le
// PC soit confié au hub, avec sa MAC, et le rôle « wake » au moins sur le hub. Renvoie true si le hub
// l'a envoyé.
func (s *Service) relayWake(d model.Device, local []netip.Prefix) bool {
	h, ok := s.currentHub()
	if !ok || (h.Role != protocol2.RoleWake.String() && h.Role != protocol2.RoleAdmin.String()) {
		return false
	}
	pc, ok := s.hubPCOf(d)
	if !ok || pc.MAC == "" || onLocalNetwork(d.Host, local) {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), hubWakeTimeout)
	defer cancel()
	err := s.agent.HubSession(ctx, hubTarget(h), func(ctx context.Context, ops agentclient.HubOps) error {
		_, err := ops.Do(ctx, hubprotocol.Request{Cmd: hubprotocol.CmdWake, PC: pc.ID})
		return err
	})
	if err != nil {
		diag.Warn(areaHub, "réveil de « %s » par le hub « %s » : %s %s (%v)", d.Name, h.Name, agentclient.CodeOf(err), agentclient.ReasonOf(err), err)
		return false
	}
	diag.Info(areaHub, "réveil de « %s » envoyé par le hub « %s »", d.Name, h.Name)
	return true
}

// onLocalNetwork indique que host est une adresse d'un des réseaux de cet ordinateur : le paquet
// magique envoyé d'ici l'atteint. Un nom d'hôte ne permet pas de le savoir.
func onLocalNetwork(host string, local []netip.Prefix) bool {
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	addr = addr.Unmap()
	for _, p := range local {
		if p.Masked().Contains(addr) {
			return true
		}
	}
	return false
}

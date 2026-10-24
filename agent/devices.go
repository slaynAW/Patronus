package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/slaynaw/wakeonlan/agent/internal/config"
	"github.com/slaynaw/wakeonlan/agent/internal/devices"
	"github.com/slaynaw/wakeonlan/agent/internal/history"
	"github.com/slaynaw/wakeonlan/agent/internal/identity"
	"github.com/slaynaw/wakeonlan/agent/internal/netinfo"
	"github.com/slaynaw/wakeonlan/agent/internal/server"
	"github.com/slaynaw/wakeonlan/agent/protocol"
	"github.com/slaynaw/wakeonlan/agent/protocol2"
	"github.com/slaynaw/wakeonlan/agent/protocol2/client"
)

const devicesUsage = `Utilisation :
  wol-agent devices                              liste les appareils autorisés en wolagent/2
  wol-agent devices revoke <empreinte>           retire un appareil
  wol-agent devices role <empreinte> <rôle>      change son rôle : view, wake, control ou admin
  wol-agent devices rename <empreinte> <nom>     le renomme
<empreinte> : le début de son empreinte courte (4 caractères au moins, tirets facultatifs).
Option --yes : retirer ou rétrograder le dernier administrateur sans confirmation.`

// enableV2 active wolagent/2 au démarrage du service : identité de l'agent (créée au premier
// lancement) et registre des appareils. Sans eux, l'agent continue en wolagent/1 seul.
func enableV2(srv *server.Server, cfg *config.Config, cfgPath string, logger interface{ Printf(string, ...any) }) {
	if !cfg.Accepts(config.ProtocolV2) {
		return
	}
	dir := filepath.Dir(cfgPath)
	id, created, err := identity.LoadOrCreate(dir, time.Now())
	if err != nil {
		logger.Printf("wolagent/2 désactivé : %v", err)
		return
	}
	if created {
		logger.Printf("identité wolagent/2 de l'agent créée (empreinte %s)", id.Pin.Short())
	}
	registry, err := devices.Open(devices.Path(dir), time.Now)
	if err != nil {
		logger.Printf("wolagent/2 désactivé : %v", err)
		return
	}
	srv.EnableV2(id, registry)
}

// describeV2 résume wolagent/2 pour wol-agent status et le rapport de diagnostic (rien de secret).
func describeV2(cfg *config.Config, cfgPath string) string {
	var accepted []string
	for _, p := range []string{config.ProtocolV1, config.ProtocolV2} {
		if cfg.Accepts(p) {
			accepted = append(accepted, p)
		}
	}
	text := strings.Join(accepted, " et ")
	if !cfg.Accepts(config.ProtocolV2) {
		return text
	}
	dir := filepath.Dir(cfgPath)
	id, err := identity.Load(dir, time.Now())
	switch {
	case errors.Is(err, os.ErrNotExist):
		return text + " — identité de l'agent créée au prochain démarrage du service"
	case err != nil:
		return text + " — identité de l'agent illisible (" + err.Error() + ")"
	}
	text += " — empreinte de l'agent " + id.Pin.Short()
	registry, err := devices.Open(devices.Path(dir), time.Now)
	if err != nil {
		return text + " — registre des appareils illisible"
	}
	return fmt.Sprintf("%s — %d appareil(s) autorisé(s), %d invitation(s) en attente", text, len(registry.Devices()), len(registry.Invites()))
}

// errNoIdentity : le service n'a pas encore créé l'identité wolagent/2 de l'agent.
var errNoIdentity = errors.New("identité wolagent/2 pas encore créée : elle l'est au démarrage du service (wol-agent status)")

// loadAgentIdentity lit l'identité wolagent/2 de l'agent, réservée aux administrateurs du PC.
func loadAgentIdentity(cfgPath string) (*identity.Identity, error) {
	id, err := identity.Load(filepath.Dir(cfgPath), time.Now())
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil, errNoIdentity
	case errors.Is(err, os.ErrPermission):
		return nil, errors.New("droits administrateur nécessaires")
	}
	return id, err
}

// deviceAdmin agit sur le registre : par le service s'il tourne (effet immédiat, connexions de
// l'appareil retiré fermées), sinon directement dans le fichier, relu au démarrage du service.
type deviceAdmin struct {
	session  *client.Session
	registry *devices.Registry
	journal  string
}

// openDeviceAdmin se connecte au service sur le PC avec l'identité de l'agent : seul un
// administrateur du PC peut la lire, et l'agent ne l'accepte que depuis l'adresse de bouclage.
// wait laisse au service qui vient de démarrer le temps d'écouter (install) : il doit alors répondre,
// le registre qu'il a déjà lu ne pouvant plus être modifié directement.
func openDeviceAdmin(cfg *config.Config, cfgPath string, id *identity.Identity, wait time.Duration) (*deviceAdmin, error) {
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(cfg.Port))
	deadline := time.Now().Add(wait)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		session, err := client.Dial(ctx, addr, client.Options{Identity: id.Cert, Agent: &id.Pin})
		cancel()
		switch {
		case err == nil:
			return &deviceAdmin{session: session}, nil
		case !netinfo.ConnRefused(err):
			return nil, fmt.Errorf("connexion au service : %w", err)
		case time.Now().Before(deadline):
			time.Sleep(250 * time.Millisecond)
			continue
		case wait > 0:
			return nil, fmt.Errorf("le service ne répond pas sur le port %d (wol-agent status)", cfg.Port)
		}
		break
	}
	registry, err := devices.Open(devices.Path(filepath.Dir(cfgPath)), time.Now)
	if err != nil {
		return nil, err
	}
	return &deviceAdmin{registry: registry, journal: historyPath(cfgPath)}, nil
}

func (a *deviceAdmin) Close() {
	if a.session != nil {
		_ = a.session.Close()
	}
}

func (a *deviceAdmin) list() ([]protocol2.DeviceInfo, int, error) {
	if a.session != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		resp, err := a.session.Do(ctx, protocol2.Request{Cmd: protocol2.CmdDevices})
		if err != nil {
			return nil, 0, err
		}
		if !resp.OK {
			return nil, 0, errors.New(resp.Message)
		}
		invites, err := a.session.Do(ctx, protocol2.Request{Cmd: protocol2.CmdInvites})
		if err != nil {
			return nil, 0, err
		}
		return resp.Devices, len(invites.Invites), nil
	}
	var list []protocol2.DeviceInfo
	for _, d := range a.registry.Devices() {
		list = append(list, protocol2.DeviceInfo{Device: d.Pin.String(), Name: d.Name, Role: d.Role.String(), Added: d.Added, Via: d.Via, Seen: d.Seen})
	}
	return list, len(a.registry.Invites()), nil
}

// invite crée une invitation (wol-agent pair) : par le service s'il tourne, sinon dans le registre, où
// le service la trouvera à son démarrage. agent est l'empreinte de l'agent, jointe à l'invitation.
func (a *deviceAdmin) invite(agent protocol2.Pin, role protocol2.Role, ttl time.Duration) (protocol2.InviteInfo, error) {
	if a.session != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		resp, err := a.session.Do(ctx, protocol2.Request{Cmd: protocol2.CmdInvite, Role: role.String(), TTL: int64(ttl / time.Second)})
		switch {
		case err != nil:
			return protocol2.InviteInfo{}, err
		case !resp.OK || resp.Invite == nil:
			return protocol2.InviteInfo{}, errors.New(resp.Message)
		}
		return *resp.Invite, nil
	}
	inv, code, err := a.registry.CreateInvite(role, "", ttl, nil, devices.ByLocal)
	if err != nil {
		return protocol2.InviteInfo{}, err
	}
	return protocol2.InviteInfo{ID: inv.ID, Code: code, Role: role.String(), Expires: inv.Expires, Agent: agent.String()}, nil
}

// change applique devices.revoke, devices.role ou devices.rename ; renvoie le code de réponse.
func (a *deviceAdmin) change(cmd string, target protocol2.Pin, value string, force bool) (string, string, error) {
	if a.session != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		req := protocol2.Request{Cmd: cmd, Device: target.String(), Role: value, Name: value}
		if force {
			req.Force = &force
		}
		resp, err := a.session.Do(ctx, req)
		return resp.Code, resp.Message, err
	}
	var changed devices.Device
	var err error
	action := protocol.DeviceRevoked
	switch cmd {
	case protocol2.CmdDevicesRevoke:
		changed, err = a.registry.Remove(target, force)
	case protocol2.CmdDevicesRole:
		role, ok := protocol2.ParseRole(value)
		if !ok {
			return protocol2.CodeBadRequest, "rôle invalide : view, wake, control ou admin", nil
		}
		action = protocol.DeviceRole
		changed, err = a.registry.SetRole(target, role, force)
	default:
		name := server.CleanName(value)
		if name == "" {
			return protocol2.CodeBadRequest, "nom vide", nil
		}
		action = protocol.DeviceRenamed
		changed, err = a.registry.Rename(target, name)
	}
	switch {
	case errors.Is(err, devices.ErrLastAdmin):
		return protocol2.CodeLastAdmin, err.Error(), nil
	case errors.Is(err, devices.ErrUnknownDevice):
		return protocol2.CodeBadRequest, "appareil introuvable", nil
	case err != nil:
		return "", "", err
	}
	if journal, err := history.Open(a.journal, time.Now); err == nil {
		_ = journal.Add(protocol.HistoryEvent{T: time.Now().Unix(), K: protocol.HistoryDevice, A: action, B: changed.Name,
			I: changed.Pin.Short(), X: changed.Role.String()})
	}
	return protocol2.CodeOK, "", nil
}

func cmdDevices(args []string) error {
	cfgPath, yes := config.DefaultPath(), false
	var rest []string
	for _, arg := range args {
		switch {
		case arg == "--yes" || arg == "-yes" || arg == "-y":
			yes = true
		case strings.HasPrefix(arg, "--config="):
			cfgPath = strings.TrimPrefix(arg, "--config=")
		case arg == "help" || arg == "--help" || arg == "-h":
			fmt.Println(devicesUsage)
			return nil
		default:
			rest = append(rest, arg)
		}
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	if !cfg.Accepts(config.ProtocolV2) {
		return errors.New("wolagent/2 est désactivé dans la configuration (« protocols »)")
	}
	id, err := loadAgentIdentity(cfgPath)
	if err != nil {
		return err
	}
	admin, err := openDeviceAdmin(cfg, cfgPath, id, 0)
	if err != nil {
		return err
	}
	defer admin.Close()
	list, invites, err := admin.list()
	if err != nil {
		return err
	}

	if len(rest) == 0 || rest[0] == "list" {
		printDevices(id, list, invites, admin.session == nil)
		return nil
	}
	cmd := map[string]string{"revoke": protocol2.CmdDevicesRevoke, "role": protocol2.CmdDevicesRole, "rename": protocol2.CmdDevicesRename}[rest[0]]
	if cmd == "" || len(rest) < 2 || (cmd == protocol2.CmdDevicesRevoke) != (len(rest) == 2) {
		return errors.New(devicesUsage)
	}
	target, err := resolveDevice(list, rest[1])
	if err != nil {
		return err
	}
	pin, _ := protocol2.ParsePin(target.Device)
	value := strings.Join(rest[2:], " ")
	code, message, err := admin.change(cmd, pin, value, false)
	if err == nil && code == protocol2.CodeLastAdmin {
		if !yes && !confirmDanger(fmt.Sprintf("« %s » est le dernier administrateur : plus aucun appareil ne pourra gérer les autres. Continuer ? [o/N] ", target.Name)) {
			return errors.New("annulé")
		}
		code, message, err = admin.change(cmd, pin, value, true)
	}
	switch {
	case err != nil:
		return err
	case code != protocol2.CodeOK:
		return errors.New(message)
	}
	done := map[string]string{protocol2.CmdDevicesRevoke: "retiré", protocol2.CmdDevicesRole: "a désormais le rôle " + value,
		protocol2.CmdDevicesRename: "renommé « " + value + " »"}[cmd]
	fmt.Printf("✔ Appareil « %s » (%s) %s.\n", target.Name, pin.Short(), done)
	if admin.session == nil {
		fmt.Println("  Service arrêté : modification appliquée à son prochain démarrage.")
	}
	return nil
}

func printDevices(id *identity.Identity, list []protocol2.DeviceInfo, invites int, offline bool) {
	fmt.Printf("Empreinte de l'agent : %s\n", id.Pin.Short())
	if offline {
		fmt.Println("(service arrêté : registre lu directement)")
	}
	if len(list) == 0 {
		fmt.Println("Aucun appareil autorisé en wolagent/2 pour l'instant : les applications appairées en wolagent/1")
		fmt.Println("y passeront d'elles-mêmes après leur mise à jour.")
	}
	via := map[string]string{devices.ViaPair: "code d'appairage", devices.ViaMigrate: "migration", devices.ViaLocal: "sur le PC"}
	for _, d := range list {
		pin, err := protocol2.ParsePin(d.Device)
		if err != nil {
			continue
		}
		line := fmt.Sprintf("  %s  %-8s %-24s ajouté le %s (%s)", pin.Short(), d.Role, d.Name,
			time.Unix(d.Added, 0).Format("02/01/2006"), via[d.Via])
		if d.Seen > 0 {
			line += ", vu le " + time.Unix(d.Seen, 0).Format("02/01/2006 15:04")
		}
		fmt.Println(line)
	}
	if invites > 0 {
		fmt.Printf("%d invitation(s) en attente.\n", invites)
	}
}

// resolveDevice trouve l'appareil désigné par le début de son empreinte courte (O lu 0, I et L lus 1,
// comme les codes) ou de son empreinte complète.
func resolveDevice(list []protocol2.DeviceInfo, prefix string) (protocol2.DeviceInfo, error) {
	typed := strings.TrimSpace(prefix)
	// Empreinte courte : tirets et espaces facultatifs ; empreinte complète (B64) : telle quelle, « - »
	// faisant partie de son alphabet.
	short := strings.NewReplacer("-", "", " ", "", "O", "0", "I", "1", "L", "1").Replace(strings.ToUpper(typed))
	if len(short) < 4 {
		return protocol2.DeviceInfo{}, errors.New("indiquez au moins 4 caractères de l'empreinte de l'appareil (wol-agent devices)")
	}
	var found []protocol2.DeviceInfo
	for _, d := range list {
		pin, err := protocol2.ParsePin(d.Device)
		if err != nil {
			continue
		}
		if strings.HasPrefix(strings.ReplaceAll(pin.Short(), "-", ""), short) || len(typed) >= 4 && strings.HasPrefix(d.Device, typed) {
			found = append(found, d)
		}
	}
	switch len(found) {
	case 0:
		return protocol2.DeviceInfo{}, fmt.Errorf("aucun appareil dont l'empreinte commence par « %s » (wol-agent devices)", prefix)
	case 1:
		return found[0], nil
	}
	return protocol2.DeviceInfo{}, fmt.Errorf("plusieurs appareils correspondent à « %s » : précisez", prefix)
}

// confirmDanger pose une question dont la réponse par défaut est non.
func confirmDanger(question string) bool {
	fmt.Print(question)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "o", "oui", "y", "yes":
		return true
	}
	return false
}

// Package agents ouvre les sessions du hub avec les agents des PC qui lui sont confiés (docs/HUB.md,
// §5.3 et §7.1) : wolagent/2 avec l'identité du hub et l'empreinte de l'agent épinglée, vers des
// adresses des réseaux privés seulement (T40). Il appaire le hub à un agent avec une invitation réservée
// au hub, en gardant le rôle le plus faible (R13).
package agents

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"

	"github.com/slaynaw/wakeonlan/agent/internal/config"
	"github.com/slaynaw/wakeonlan/agent/internal/identity"
	"github.com/slaynaw/wakeonlan/agent/protocol2"
	"github.com/slaynaw/wakeonlan/agent/protocol2/client"
)

// ErrNotPrivate : l'adresse visée n'est pas dans un réseau privé du réseau local.
var ErrNotPrivate = errors.New("adresse hors du réseau local : le hub ne joint que des adresses privées")

// TargetNetworks sont les réseaux que le hub peut joindre : ceux de la liste « allow » par défaut d'un
// agent, sans l'adresse de bouclage (le NAS se joint par son adresse sur le réseau local).
func TargetNetworks() []netip.Prefix {
	var out []netip.Prefix
	for _, cidr := range config.DefaultAllow {
		p := netip.MustParsePrefix(cidr)
		if !p.Addr().IsLoopback() {
			out = append(out, p)
		}
	}
	return out
}

// Dialer ouvre les connexions du hub.
type Dialer struct {
	Identity *identity.Identity
	// Networks sont les réseaux permis (TargetNetworks en service).
	Networks []netip.Prefix
	// Resolver résout les noms d'hôte (net.DefaultResolver par défaut).
	Resolver *net.Resolver
}

// DialContext ouvre une connexion TCP vers host:port, après avoir vérifié que chaque adresse essayée est
// dans un réseau permis.
func (d *Dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	resolver := d.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	addrs, err := resolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	var last error = ErrNotPrivate
	for _, a := range addrs {
		a = a.Unmap()
		if !d.permitted(a) {
			continue
		}
		var nd net.Dialer
		conn, err := nd.DialContext(ctx, network, net.JoinHostPort(a.String(), port))
		if err == nil {
			return conn, nil
		}
		last = err
	}
	return nil, last
}

func (d *Dialer) permitted(a netip.Addr) bool {
	for _, p := range d.Networks {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// Dial ouvre une session wolagent/2 avec l'agent, son empreinte épinglée.
func (d *Dialer) Dial(ctx context.Context, host string, port int, agent protocol2.Pin) (*client.Session, error) {
	return client.Dial(ctx, net.JoinHostPort(host, strconv.Itoa(port)), client.Options{
		Identity: d.Identity.Cert, Agent: &agent, Dial: d.DialContext,
	})
}

// Pair appaire le hub à l'agent avec l'invitation code, réservée au hub, sous le nom name. Il renvoie le
// rôle obtenu : view, wake ou control. Une invitation au rôle admin est ramenée à wake (le hub n'est
// administrateur d'aucun PC) ; si l'agent le refuse, le hub se retire et l'appairage échoue.
func (d *Dialer) Pair(ctx context.Context, host string, port int, agent protocol2.Pin, code, name string) (protocol2.Role, error) {
	sess, err := d.Dial(ctx, host, port, agent)
	if err != nil {
		return 0, err
	}
	defer sess.Close()
	resp, err := sess.Do(ctx, protocol2.Request{Cmd: protocol2.CmdPair, Code: code, Name: name})
	switch {
	case err != nil:
		return 0, err
	case resp.Code == protocol2.CodeAlreadyPaired:
		// Déjà appairé (PC retiré du hub puis confié de nouveau) : le rôle est celui de l'agent.
	case !resp.OK:
		return 0, fmt.Errorf("l'agent refuse l'appairage : %s", resp.Message)
	}
	role, ok := protocol2.ParseRole(resp.Role)
	if !ok {
		return 0, fmt.Errorf("rôle du hub sur l'agent illisible : %q", resp.Role)
	}
	if role != protocol2.RoleAdmin {
		return role, nil
	}
	self := d.Identity.Pin.String()
	down, err := sess.Do(ctx, protocol2.Request{Cmd: protocol2.CmdDevicesRole, Device: self, Role: protocol2.RoleWake.String()})
	if err == nil && down.OK {
		return protocol2.RoleWake, nil
	}
	if gone, err := sess.Do(ctx, protocol2.Request{Cmd: protocol2.CmdDevicesRevoke, Device: self}); err != nil || !gone.OK {
		// Seul administrateur du PC, le hub ne peut ni se rétrograder ni se retirer (§8 de PROTOCOLE-V2.md).
		return 0, fmt.Errorf("l'invitation donnait au hub le rôle admin, qu'il refuse, et il n'a pas pu s'en défaire : "+
			"retirez-le sur le PC (wol-agent devices revoke %s)", d.Identity.Pin.Short())
	}
	return 0, errors.New("l'invitation donnait au hub le rôle admin, qu'il refuse : invitez-le avec le rôle wake")
}

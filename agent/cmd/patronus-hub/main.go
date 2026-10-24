// Commande patronus-hub : le hub Patronus sur le NAS (docs/HUB.md). Dans le conteneur, elle sert
// patronus-hub/1 aux applications ; « patronus-hub pair » affiche un nouveau lien d'appairage.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/slaynaw/wakeonlan/agent/hubprotocol"
	"github.com/slaynaw/wakeonlan/agent/internal/devices"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/agents"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/fleet"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/monitor"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/server"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/store"
	"github.com/slaynaw/wakeonlan/agent/internal/identity"
	"github.com/slaynaw/wakeonlan/agent/internal/netinfo"
	"github.com/slaynaw/wakeonlan/agent/protocol2"
	"github.com/slaynaw/wakeonlan/agent/protocol2/client"
)

// version est injectée à la compilation (-ldflags "-X main.version=2.0.0").
var version = "dev"

// registryFile est le registre des appareils autorisés, dans le stockage chiffré.
const registryFile = "devices.db"

const usageText = `patronus-hub %s — hub Patronus sur le NAS (docs/HUB.md)

Utilisation :
  patronus-hub [serve]   Sert les applications (par défaut ; c'est la commande du conteneur).
  patronus-hub pair      Affiche un nouveau lien d'appairage, à usage unique, valable 10 minutes.
  patronus-hub version   Affiche la version.

Options de serve et de pair :
  --data DOSSIER   données chiffrées (/data)
  --keys DOSSIER   identité du hub et clé des données (/keys), à part des données
  --port PORT      port TCP (9771)
  --ip ADRESSE     adresse du NAS annoncée dans le lien (détectée sinon)
Options de serve : --name NOM (« Hub NAS »). Option de pair : --role view|wake|admin (admin).
`

func main() {
	args := os.Args[1:]
	cmd := "serve"
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		cmd, args = args[0], args[1:]
	}
	var err error
	switch cmd {
	case "serve":
		err = serve(args)
	case "pair":
		err = pair(args)
	case "version":
		fmt.Println(version)
	case "help", "-h", "--help":
		fmt.Printf(usageText, version)
	default:
		fmt.Fprintf(os.Stderr, usageText, version)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "erreur :", err)
		os.Exit(1)
	}
}

type options struct {
	data, keys, ip, name, role string
	port                       int
}

func parse(name string, args []string) options {
	var o options
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	fs.StringVar(&o.data, "data", "/data", "données chiffrées")
	fs.StringVar(&o.keys, "keys", "/keys", "identité du hub et clé des données")
	fs.IntVar(&o.port, "port", hubprotocol.DefaultPort, "port TCP")
	fs.StringVar(&o.ip, "ip", "", "adresse du NAS annoncée dans le lien")
	if name == "serve" {
		fs.StringVar(&o.name, "name", "Hub NAS", "nom du hub")
	} else {
		fs.StringVar(&o.role, "role", "admin", "rôle donné : view, wake ou admin")
	}
	_ = fs.Parse(args)
	return o
}

func serve(args []string) error {
	o := parse("serve", args)
	logger := log.New(os.Stdout, "", log.LstdFlags)
	name := protocol2.CleanName(o.name, hubprotocol.MaxHubNameLength)
	if name == "" {
		return errors.New("nom du hub vide")
	}
	key, created, err := store.LoadOrCreateKey(o.keys)
	if err != nil {
		return err
	}
	if created {
		logger.Printf("clé des données créée dans %s : copiez ce dossier une fois à part (docs/HUB.md, §9.3)", o.keys)
	}
	id, created, err := identity.LoadOrCreate(o.keys, time.Now())
	if err != nil {
		return err
	}
	if created {
		logger.Printf("identité du hub créée")
	}
	st, err := store.Open(o.data, key)
	if err != nil {
		return err
	}
	registry, err := devices.OpenStorage(st.File(registryFile), registryFile+" (chiffré)", time.Now)
	if err != nil {
		return err
	}
	pcs, err := fleet.Open(st.File(fleet.FileName), time.Now)
	if err != nil {
		return err
	}
	dialer := &agents.Dialer{Identity: id, Networks: agents.TargetNetworks()}
	watch := monitor.New(pcs, dialer, logger)
	srv, err := server.New(server.Config{
		Name: name, Version: version, Identity: id, Registry: registry, Fleet: pcs, Agents: dialer, States: watch.States,
		Logger: logger,
	})
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", ":"+strconv.Itoa(o.port))
	if err != nil {
		return err
	}
	logger.Printf("hub « %s » %s : port %d, empreinte %s", name, version, o.port, id.Pin.Short())

	// Premier démarrage : aucun administrateur, un lien d'appairage dans le journal du conteneur (§5.2).
	// Les invitations d'un démarrage précédent sont d'abord annulées : sans administrateur, elles ne
	// peuvent venir que du NAS, et des redémarrages répétés ne doivent pas en épuiser le nombre.
	if !hasAdmin(registry) {
		if _, err := registry.CancelInvites(); err != nil {
			logger.Printf("registre des appareils : %v", err)
		}
		inv, code, err := registry.CreateInvite(protocol2.RoleAdmin, "", protocol2.DefaultInviteTTL*time.Second, nil, devices.ByLocal)
		if err != nil {
			logger.Printf("lien d'appairage impossible (%v) : patronus-hub pair en crée un", err)
		} else {
			printLink(os.Stdout, name, o, id.Pin, code, inv.Expires, protocol2.RoleAdmin)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go watch.Run(ctx)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stop
		logger.Printf("arrêt du hub")
		cancel()
		_ = ln.Close()
	}()
	return srv.Serve(ln)
}

func hasAdmin(r *devices.Registry) bool {
	for _, d := range r.Devices() {
		if d.Role == protocol2.RoleAdmin {
			return true
		}
	}
	return false
}

// pair demande au hub en marche, depuis le NAS et avec l'identité du hub, une invitation à usage unique.
func pair(args []string) error {
	o := parse("pair", args)
	role, ok := hubprotocol.ParseRole(o.role)
	if !ok {
		return fmt.Errorf("rôle invalide %q : view, wake ou admin", o.role)
	}
	id, err := identity.Load(o.keys, time.Now())
	if err != nil {
		return fmt.Errorf("identité du hub illisible (lancez cette commande dans le conteneur du hub) : %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pin := id.Pin
	sess, err := client.Dial(ctx, net.JoinHostPort("127.0.0.1", strconv.Itoa(o.port)), client.Options{
		Identity: id.Cert, Agent: &pin, Protocol: hubprotocol.ALPN,
	})
	if err != nil {
		return fmt.Errorf("hub injoignable sur ce NAS (port %d) : %w", o.port, err)
	}
	defer sess.Close()
	var status hubprotocol.Response
	if err := sess.Call(ctx, hubprotocol.Request{Cmd: hubprotocol.CmdStatus}, &status); err != nil || !status.OK || status.Hub == nil {
		return fmt.Errorf("état du hub illisible : %v %s", err, status.Message)
	}
	var resp hubprotocol.Response
	if err := sess.Call(ctx, hubprotocol.Request{Cmd: protocol2.CmdInvite, Role: role.String()}, &resp); err != nil {
		return err
	}
	if !resp.OK || resp.Invite == nil {
		return fmt.Errorf("invitation refusée : %s", resp.Message)
	}
	printLink(os.Stdout, status.Hub.Name, o, pin, resp.Invite.Code, resp.Invite.Expires, role)
	return nil
}

// printLink affiche le lien d'appairage et le code : le seul secret jamais écrit au journal du conteneur,
// à usage unique et valable 10 minutes (docs/HUB.md, §5.1 et §15).
func printLink(w *os.File, name string, o options, hub protocol2.Pin, code string, expires int64, role protocol2.Role) {
	host := o.ip
	if host == "" {
		if iface, err := netinfo.Detect(""); err == nil {
			host = iface.IP.String()
		}
	}
	display := protocol2.FormatCode(code)
	fmt.Fprintf(w, "\nAppairage d'une application au hub (rôle %s), une seule fois, jusqu'à %s :\n", role,
		time.Unix(expires, 0).Format("15:04"))
	if host == "" {
		fmt.Fprintf(w, "  adresse du NAS introuvable : relancez avec --ip ADRESSE\n")
	} else {
		link := protocol2.Link{Name: name, Host: host, Port: o.port, Agent: hub, Code: code, Role: role, Expires: expires}
		fmt.Fprintf(w, "  lien : %s\n", link.Format(hubprotocol.LinkFormat))
		fmt.Fprintf(w, "  ou : adresse %s, port %d, code %s\n", host, o.port, display)
	}
	fmt.Fprintf(w, "  empreinte du hub : %s\n", hub.Short())
	fmt.Fprintf(w, "Nouveau code : sudo docker exec patronus-hub patronus-hub pair\n\n")
}

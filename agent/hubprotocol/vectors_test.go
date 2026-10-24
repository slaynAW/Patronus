package hubprotocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/slaynaw/wakeonlan/agent/protocol2"
)

var update = flag.Bool("update", false, "régénère protocol/hub-vectors.json à partir de ce paquet")

var vectorsPath = filepath.Join("..", "..", "protocol", "hub-vectors.json")

type vectorFile struct {
	Description string        `json:"description"`
	Protocol    string        `json:"protocol"`
	Port        int           `json:"port"`
	Hello       string        `json:"hello"`
	Links       linkVectors   `json:"links"`
	Roles       []string      `json:"roles"`
	Authorize   []authVector  `json:"authorize"`
	Commands    []commandRole `json:"commands"`
}

type linkVectors struct {
	Valid   []linkVector `json:"valid"`
	Invalid []linkVector `json:"invalid"`
}

type linkVector struct {
	Name  string `json:"name"`
	Link  string `json:"link"`
	Error string `json:"error,omitempty"` // paramètre en cause (comme protocol2.LinkError)
	// Contenu attendu d'un lien valide.
	HubName string `json:"hubName,omitempty"`
	Host    string `json:"host,omitempty"`
	Port    int    `json:"port,omitempty"`
	Hub     string `json:"hub,omitempty"`
	Code    string `json:"code,omitempty"`
	Role    string `json:"role,omitempty"`
	Expires int64  `json:"expires,omitempty"`
}

type commandRole struct {
	Cmd     string `json:"cmd"`
	MinRole string `json:"minRole"` // "" : réservée aux identités inconnues (pair)
}

type authVector struct {
	Registered bool   `json:"registered"`
	Role       string `json:"role"` // "" pour une identité inconnue
	Cmd        string `json:"cmd"`
	Code       string `json:"code"`
}

var hubPin = protocol2.PinOf([]byte("hub de test"))

func buildVectors() vectorFile {
	v := vectorFile{
		Description: "Vecteurs de patronus-hub/1 (docs/PROTOCOLE-HUB.md) : liens d'appairage du hub, salut, rôles et " +
			"autorisation des commandes. Produits par agent/hubprotocol (go test ./hubprotocol -run TestVectors -update) ; " +
			"relus par l'application Android (core).",
		Protocol: ALPN, Port: DefaultPort, Hello: Hello,
	}
	pin := hubPin.String()
	valid := []struct{ name, link string }{
		{"lien complet", "patronus://hub?v=1&n=Hub+NAS&h=192.168.1.20&p=9771&a=" + pin + "&c=7KQ29MXP&r=admin&e=1790000600"},
		{"port par défaut, sans nom (le début de l'hôte le remplace)", "patronus://hub?v=1&h=nas.local&a=" + pin + "&c=7kq2-9mxp"},
		{"majuscules au début, code à normaliser", "PATRONUS://HUB?v=1&n=NAS%20maison&h=192.168.1.20&a=" + pin + "&c=7kq2+9mxp&r=view"},
		{"paramètre inconnu ignoré", "patronus://hub?v=1&h=10.0.0.2&p=19771&a=" + pin + "&c=7KQ29MXP&x=1&r=wake"},
	}
	for _, c := range valid {
		l, err := ParseLink(c.link)
		if err != nil {
			panic(c.name + " : " + err.Error())
		}
		vec := linkVector{Name: c.name, Link: c.link, HubName: l.Name, Host: l.Host, Port: l.Port, Hub: l.Agent.String(), Code: l.Code, Expires: l.Expires}
		if l.Role != 0 {
			vec.Role = l.Role.String()
		}
		v.Links.Valid = append(v.Links.Valid, vec)
	}
	invalid := []struct{ name, link string }{
		{"lien d'agent", "wolagent://pair?v=2&h=192.168.1.20&a=" + pin + "&c=7KQ29MXP"},
		{"autre version", "patronus://hub?v=2&h=192.168.1.20&a=" + pin + "&c=7KQ29MXP"},
		{"rôle control (inconnu du hub)", "patronus://hub?v=1&h=192.168.1.20&a=" + pin + "&c=7KQ29MXP&r=control"},
		{"rôle inconnu", "patronus://hub?v=1&h=192.168.1.20&a=" + pin + "&c=7KQ29MXP&r=root"},
		{"empreinte absente", "patronus://hub?v=1&h=192.168.1.20&c=7KQ29MXP"},
		{"code trop court", "patronus://hub?v=1&h=192.168.1.20&a=" + pin + "&c=7KQ29MX"},
		{"hôte invalide", "patronus://hub?v=1&h=999.1.1.1&a=" + pin + "&c=7KQ29MXP"},
		{"port hors limites", "patronus://hub?v=1&h=192.168.1.20&p=65536&a=" + pin + "&c=7KQ29MXP"},
		{"paramètre en double", "patronus://hub?v=1&h=192.168.1.20&h=192.168.1.21&a=" + pin + "&c=7KQ29MXP"},
	}
	for _, c := range invalid {
		_, err := ParseLink(c.link)
		var le *protocol2.LinkError
		if !errors.As(err, &le) {
			panic(c.name + " : accepté")
		}
		v.Links.Invalid = append(v.Links.Invalid, linkVector{Name: c.name, Link: c.link, Error: le.Field})
	}
	for _, r := range Roles {
		v.Roles = append(v.Roles, r.String())
	}
	for _, cmd := range Commands {
		cr := commandRole{Cmd: cmd}
		if r, ok := MinRole(cmd); ok {
			cr.MinRole = r.String()
		}
		v.Commands = append(v.Commands, cr)
	}
	cmds := append(append([]string{}, Commands...), "status", "shutdown", "migrate", "HUB.STATUS", "")
	for _, cmd := range cmds {
		v.Authorize = append(v.Authorize, authVector{Cmd: cmd, Code: Authorize(false, 0, cmd)})
		for _, r := range protocol2.Roles {
			v.Authorize = append(v.Authorize, authVector{Registered: true, Role: r.String(), Cmd: cmd, Code: Authorize(true, r, cmd)})
		}
	}
	return v
}

// TestVectors vérifie protocol/hub-vectors.json. Avec -update, le fichier est régénéré.
func TestVectors(t *testing.T) {
	raw, err := json.MarshalIndent(buildVectors(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	if *update {
		if err := os.WriteFile(vectorsPath, raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	saved, err := os.ReadFile(vectorsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(saved, raw) {
		t.Fatal("protocol/hub-vectors.json n'est plus à jour : go test ./hubprotocol -run TestVectors -update")
	}
}

// TestAuthorize vérifie les règles du §5 indépendamment des vecteurs.
func TestAuthorize(t *testing.T) {
	type c struct {
		registered bool
		role       protocol2.Role
		cmd, want  string
	}
	for _, x := range []c{
		{false, 0, protocol2.CmdPair, protocol2.CodeOK},
		{false, 0, CmdStatus, protocol2.CodeUnknownDevice},
		{false, 0, protocol2.CmdMigrate, protocol2.CodeUnknownDevice},
		{true, protocol2.RoleView, protocol2.CmdPair, protocol2.CodeAlreadyPaired},
		{true, protocol2.RoleView, CmdStatus, protocol2.CodeOK},
		{true, protocol2.RoleView, CmdWake, protocol2.CodeInsufficientRole},
		{true, protocol2.RoleWake, CmdWake, protocol2.CodeOK},
		{true, protocol2.RoleWake, CmdPCs, protocol2.CodeInsufficientRole},
		{true, protocol2.RoleAdmin, CmdSchedulesSet, protocol2.CodeOK},
		{true, protocol2.RoleAdmin, protocol2.CmdInvite, protocol2.CodeOK},
		// control n'existe pas sur le hub : il n'y donne rien, pas même la lecture.
		{true, protocol2.RoleControl, CmdStatus, protocol2.CodeInsufficientRole},
		{true, protocol2.RoleAdmin, "shutdown", protocol2.CodeUnsupported},
		{true, protocol2.RoleAdmin, "status", protocol2.CodeUnsupported},
		{true, protocol2.RoleAdmin, protocol2.CmdMigrate, protocol2.CodeUnsupported},
	} {
		if got := Authorize(x.registered, x.role, x.cmd); got != x.want {
			t.Errorf("%v %s %q : %s, attendu %s", x.registered, x.role, x.cmd, got, x.want)
		}
	}
	for _, name := range []string{"view", "wake", "admin"} {
		if _, ok := ParseRole(name); !ok {
			t.Errorf("%s refusé", name)
		}
	}
	if _, ok := ParseRole("control"); ok {
		t.Error("control accepté")
	}
}

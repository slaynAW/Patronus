// Package pairing lit les liens d'appairage « wolagent://pair?… » affichés par l'agent
// (même règles que PairingLink côté Android) : v=1 avec la clé de l'agent, v=2 avec son empreinte et
// un code à usage unique (docs/PROTOCOLE-V2.md, §7.1).
package pairing

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/slaynaw/wakeonlan/agent/protocol2"
	"github.com/slaynaw/wakeonlan/desktop/internal/model"
)

// Prefix est le début de tout lien d'appairage.
const Prefix = "wolagent://pair?"

// Info contient les informations d'appairage.
type Info struct {
	// Version du lien : 1 (clé de l'agent) ou 2 (empreinte et code).
	Version int    `json:"version"`
	Name    string `json:"name"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
	// MAC est vide si le lien n'en contient pas.
	MAC string `json:"mac"`
	// Key : clé wolagent/1 de l'agent (lien v=1).
	Key string `json:"key"`
	// Lien v=2 : empreinte de l'agent (complète et courte), code d'appairage normalisé, rôle et
	// expiration annoncés (purement indicatifs : l'agent applique les siens).
	Agent      string `json:"agent,omitempty"`
	AgentShort string `json:"agentShort,omitempty"`
	Code       string `json:"code,omitempty"`
	Role       string `json:"role,omitempty"`
	Expires    int64  `json:"expires,omitempty"`
}

func (i Info) String() string {
	if i.Version == 2 {
		return fmt.Sprintf("Info(v=2, name=%s, host=%s, port=%d, mac=%s, agent=%s, code=***)", i.Name, i.Host, i.Port, i.MAC, i.AgentShort)
	}
	return fmt.Sprintf("Info(name=%s, host=%s, port=%d, mac=%s, key=***)", i.Name, i.Host, i.Port, i.MAC)
}

// linkErrors : message d'un lien v=2 invalide, selon le paramètre en cause.
var linkErrors = map[string]string{
	"h": "Adresse du PC invalide dans le lien",
	"a": "Empreinte de l'agent invalide dans le lien",
	"c": "Code d'appairage invalide dans le lien",
	"p": "Port invalide dans le lien",
	"m": "Adresse MAC invalide dans le lien",
	"r": "Rôle invalide dans le lien",
	"e": "Date d'expiration invalide dans le lien",
}

// parseV2 lit un lien v=2 selon les règles de la spécification (paramètre en double refusé, chaque
// paramètre présent vérifié), puis comme un PC ajouté à la main.
func parseV2(text string) (Info, error) {
	l, err := protocol2.ParseLink(text)
	if err != nil {
		var linkErr *protocol2.LinkError
		if errors.As(err, &linkErr) && linkErrors[linkErr.Field] != "" {
			return Info{}, errors.New(linkErrors[linkErr.Field])
		}
		return Info{}, errors.New("Lien d'appairage mal formé")
	}
	if !model.IsValidHost(l.Host) {
		return Info{}, errors.New(linkErrors["h"])
	}
	info := Info{Version: 2, Name: model.TruncateUTF16(l.Name, model.MaxNameLength), Host: l.Host, Port: l.Port,
		Agent: l.Agent.String(), AgentShort: l.Agent.Short(), Code: l.Code, Expires: l.Expires}
	if l.MAC != "" {
		mac, err := model.ParseMAC(l.MAC)
		if err != nil {
			return Info{}, errors.New(linkErrors["m"])
		}
		info.MAC = mac.String()
	}
	if l.Role != 0 {
		info.Role = l.Role.String()
	}
	return info, nil
}

// Parse lit un lien ; l'erreur contient un message lisible.
func Parse(text string) (Info, error) {
	link := strings.TrimSpace(text)
	if len(link) < len(Prefix) || !strings.EqualFold(link[:len(Prefix)], Prefix) {
		return Info{}, errors.New("Ce n'est pas un lien d'appairage wolagent://")
	}
	params := map[string]string{}
	for _, part := range strings.Split(link[len(Prefix):], "&") {
		if part == "" {
			continue
		}
		eq := strings.IndexByte(part, '=')
		if eq <= 0 {
			return Info{}, errors.New("Lien d'appairage mal formé")
		}
		value, err := url.QueryUnescape(part[eq+1:])
		if err != nil {
			return Info{}, errors.New("Lien d'appairage mal formé")
		}
		params[part[:eq]] = value
	}
	if v, ok := params["v"]; v == "2" {
		return parseV2(link)
	} else if !ok || v != "1" {
		if !ok {
			v = "null"
		}
		return Info{}, fmt.Errorf("Version de lien non prise en charge : %s", v)
	}
	host := params["h"]
	if !model.IsValidHost(host) {
		return Info{}, errors.New("Adresse du PC invalide dans le lien")
	}
	port := model.DefaultAgentPort
	if p, ok := params["p"]; ok {
		if n, err := strconv.Atoi(p); err == nil {
			port = n
		}
	}
	if !model.IsValidPort(port) {
		return Info{}, errors.New("Port invalide dans le lien")
	}
	key := params["k"]
	if model.DecodeAgentKey(key) == nil {
		return Info{}, errors.New("Clé invalide dans le lien")
	}
	mac := ""
	if m := params["m"]; m != "" {
		parsed, err := model.ParseMAC(m)
		if err != nil {
			return Info{}, errors.New("Adresse MAC invalide dans le lien")
		}
		mac = parsed.String()
	}
	name := strings.Map(func(r rune) rune {
		if model.IsISOControl(r) {
			return -1
		}
		return r
	}, model.TruncateUTF16(strings.TrimSpace(params["n"]), model.MaxNameLength))
	if name == "" {
		name = host
	}
	return Info{Version: 1, Name: name, Host: host, Port: port, MAC: mac, Key: key}, nil
}

// Package pairing lit les liens d'appairage « wolagent://pair?… » affichés par l'agent
// (même règles que PairingLink côté Android).
package pairing

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/slaynaw/wakeonlan/desktop/internal/model"
)

// Prefix est le début de tout lien d'appairage.
const Prefix = "wolagent://pair?"

// Info contient les informations d'appairage.
type Info struct {
	Name string `json:"name"`
	Host string `json:"host"`
	Port int    `json:"port"`
	// MAC est vide si le lien n'en contient pas.
	MAC string `json:"mac"`
	Key string `json:"key"`
}

func (i Info) String() string {
	return fmt.Sprintf("Info(name=%s, host=%s, port=%d, mac=%s, key=***)", i.Name, i.Host, i.Port, i.MAC)
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
	if v, ok := params["v"]; !ok || v != "1" {
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
	return Info{Name: name, Host: host, Port: port, MAC: mac, Key: key}, nil
}

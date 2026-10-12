package protocol2

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

const (
	// LinkPrefix commence tout lien d'appairage (majuscules acceptées, comme en v=1).
	LinkPrefix = "wolagent://pair?"
	// MaxLinkLength borne la taille d'un lien, en octets.
	MaxLinkLength = 2048
	// MaxExpires est la plus grande expiration acceptée dans un lien : 31/12/9999 23:59:59 UTC.
	MaxExpires    = 253402300799
	maxHostLength = 253
)

// Link est le contenu d'un lien d'appairage v=2 (§7.1). Il ne contient aucun secret durable : l'empreinte
// de l'agent est publique, et le code ne sert qu'une fois, pendant quelques minutes.
type Link struct {
	Name    string // jamais vide après ParseLink : le début de l'hôte à défaut de nom
	Host    string
	Port    int
	MAC     string // AA:BB:CC:DD:EE:FF, ou vide
	Agent   Pin
	Code    string // normalisé (8 caractères)
	Role    Role   // rôle annoncé, purement indicatif ; 0 s'il est absent
	Expires int64  // expiration annoncée (secondes Unix), pour l'affichage ; 0 si elle est absente
}

// LinkError désigne ce qui rend un lien invalide : "format" (pas un lien d'appairage, trop long, ou
// paramètres mal écrits), "duplicate" (paramètre en double) ou le paramètre en cause ("v", "h", "a",
// "c", "p", "m", "r", "e").
type LinkError struct{ Field string }

func (e *LinkError) Error() string { return "wolagent/2 : lien d'appairage invalide (" + e.Field + ")" }

// ParseLink lit un lien d'appairage v=2. Les paramètres sont vérifiés dans l'ordre du tableau du §7.1 ;
// un paramètre présent doit être valide, seul le nom peut être vide. Les paramètres inconnus sont
// ignorés. Une autre version que 2 donne l'erreur "v" : l'application lit alors le lien comme avant.
func ParseLink(text string) (Link, error) {
	fail := func(field string) (Link, error) { return Link{}, &LinkError{Field: field} }
	s := strings.Trim(text, " \t\r\n")
	if len(s) > MaxLinkLength || len(s) < len(LinkPrefix) || !asciiEqualFold(s[:len(LinkPrefix)], LinkPrefix) {
		return fail("format")
	}
	params := map[string]string{}
	for _, part := range strings.Split(s[len(LinkPrefix):], "&") {
		if part == "" {
			continue
		}
		name, raw, ok := strings.Cut(part, "=")
		if !ok || name == "" {
			return fail("format")
		}
		value, err := url.QueryUnescape(raw)
		if err != nil || !utf8.ValidString(value) {
			return fail("format")
		}
		if _, dup := params[name]; dup {
			return fail("duplicate")
		}
		params[name] = value
	}

	var l Link
	if params["v"] != "2" {
		return fail("v")
	}
	if l.Host = params["h"]; !validHost(l.Host) {
		return fail("h")
	}
	agent, err := ParsePin(params["a"])
	if err != nil {
		return fail("a")
	}
	l.Agent = agent
	if l.Code, err = NormalizeCode(params["c"]); err != nil {
		return fail("c")
	}
	l.Port = DefaultPort
	if p, ok := params["p"]; ok {
		n, ok := parseDecimal(p, 5)
		if !ok || n < 1 || n > 65535 {
			return fail("p")
		}
		l.Port = int(n)
	}
	if m, ok := params["m"]; ok {
		if l.MAC, ok = normalizeMAC(m); !ok {
			return fail("m")
		}
	}
	if r, ok := params["r"]; ok {
		if l.Role, ok = ParseRole(r); !ok {
			return fail("r")
		}
	}
	if e, ok := params["e"]; ok {
		n, ok := parseDecimal(e, 12)
		if !ok || n < 1 || n > MaxExpires {
			return fail("e")
		}
		l.Expires = n
	}
	if l.Name = CleanName(params["n"], MaxPCNameLength); l.Name == "" {
		l.Name = CleanName(l.Host, MaxPCNameLength)
	}
	return l, nil
}

// String écrit le lien, paramètres dans l'ordre de la spécification ; le nom, le port, la MAC, le rôle
// et l'expiration n'y figurent que s'ils sont renseignés.
func (l Link) String() string {
	params := []string{"v=2"}
	if l.Name != "" {
		params = append(params, "n="+escape(l.Name))
	}
	params = append(params, "h="+escape(l.Host))
	if l.Port != 0 {
		params = append(params, "p="+strconv.Itoa(l.Port))
	}
	if l.MAC != "" {
		params = append(params, "m="+escape(l.MAC))
	}
	params = append(params, "a="+l.Agent.String(), "c="+escape(l.Code))
	if l.Role != 0 {
		params = append(params, "r="+l.Role.String())
	}
	if l.Expires != 0 {
		params = append(params, "e="+strconv.FormatInt(l.Expires, 10))
	}
	return LinkPrefix + strings.Join(params, "&")
}

// Expired indique si l'expiration annoncée par le lien est atteinte à l'heure now (secondes Unix). Un
// lien sans expiration n'expire pas : l'agent applique de toute façon la sienne.
func (l Link) Expired(now int64) bool { return l.Expires != 0 && now >= l.Expires }

// CleanName nettoie un nom à afficher : caractères de contrôle (U+0000 à U+001F, U+007F à U+009F)
// retirés, espaces retirés au début et à la fin, au plus limit unités UTF-16 sans couper de caractère.
func CleanName(name string, limit int) string {
	name = strings.Trim(strings.Map(func(r rune) rune {
		if r <= 0x1f || r >= 0x7f && r <= 0x9f {
			return -1
		}
		return r
	}, name), " ")
	units := 0
	for i, r := range name {
		if units += utf16.RuneLen(r); units > limit {
			name = name[:i]
			break
		}
	}
	return strings.TrimRight(name, " ")
}

// escape encode une valeur de paramètre ; « : » est conservé (valide dans une URL) pour un QR code plus
// petit, comme en v=1.
func escape(value string) string { return strings.ReplaceAll(url.QueryEscape(value), "%3A", ":") }

// asciiEqualFold compare sans tenir compte de la casse des lettres ASCII, et d'elles seules.
func asciiEqualFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		x, y := a[i], b[i]
		if x >= 'A' && x <= 'Z' {
			x += 'a' - 'A'
		}
		if y >= 'A' && y <= 'Z' {
			y += 'a' - 'A'
		}
		if x != y {
			return false
		}
	}
	return true
}

// parseDecimal lit un entier écrit avec 1 à maxDigits chiffres ASCII, sans signe.
func parseDecimal(s string, maxDigits int) (int64, bool) {
	if s == "" || len(s) > maxDigits {
		return 0, false
	}
	var n int64
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
		n = n*10 + int64(s[i]-'0')
	}
	return n, true
}

// normalizeMAC lit une adresse MAC « AA:BB:CC:DD:EE:FF » ou « AA-BB-CC-DD-EE-FF » (casse indifférente,
// un seul séparateur), et refuse celles qui ne désignent pas une carte réseau (que des 0 ou que des F).
func normalizeMAC(s string) (string, bool) {
	if len(s) != 17 || s[2] != ':' && s[2] != '-' {
		return "", false
	}
	out := make([]byte, len(s))
	zero, broadcast := true, true
	for i := 0; i < len(s); i++ {
		c := s[i]
		if i%3 == 2 {
			if c != s[2] {
				return "", false
			}
			out[i] = ':'
			continue
		}
		if c >= 'a' && c <= 'f' {
			c -= 'a' - 'A'
		}
		if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'F') {
			return "", false
		}
		out[i] = c
		zero, broadcast = zero && c == '0', broadcast && c == 'F'
	}
	if zero || broadcast {
		return "", false
	}
	return string(out), true
}

var (
	ipv4Pattern     = regexp.MustCompile(`^((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)\.){3}(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)$`)
	ipv6CharPattern = regexp.MustCompile(`^[0-9A-Fa-f:.]+(%[A-Za-z0-9_.\-]+)?$`)
)

// validHost applique les règles des applications pour l'adresse d'un PC (IPv4, IPv6 littérale ou nom
// d'hôte RFC 1123), celles de DeviceValidator.isValidHost (Android) et de model.IsValidHost (Windows).
func validHost(host string) bool {
	if host == "" || len(host) > maxHostLength || host != strings.TrimSpace(host) {
		return false
	}
	if ipv4Pattern.MatchString(host) {
		return true
	}
	if strings.Contains(host, ":") {
		colons := strings.Count(host, ":")
		return ipv6CharPattern.MatchString(host) && colons >= 2 && colons <= 7
	}
	// Un nom composé uniquement de chiffres et de points est une IPv4 mal formée.
	if strings.Trim(host, "0123456789.") == "" {
		return false
	}
	for _, label := range strings.Split(strings.TrimSuffix(host, "."), ".") {
		if len(label) < 1 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for i := 0; i < len(label); i++ {
			c := label[i]
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}

package model

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf16"
)

// Field désigne un champ du formulaire, pour afficher l'erreur au bon endroit.
type Field string

const (
	FieldName       Field = "NAME"
	FieldMAC        Field = "MAC"
	FieldHost       Field = "HOST"
	FieldBroadcast  Field = "BROADCAST"
	FieldWolPort    Field = "WOL_PORT"
	FieldSecureOn   Field = "SECURE_ON"
	FieldProbePorts Field = "PROBE_PORTS"
	FieldAgentPort  Field = "AGENT_PORT"
	FieldAgentKey   Field = "AGENT_KEY"
)

// ValidationError est une erreur rattachée à un champ.
type ValidationError struct {
	Field   Field
	Message string
}

// Limites (identiques à Android).
const (
	MaxNameLength   = 64
	MaxHostLength   = 253
	MaxProbePorts   = 8
	AgentKeyBytes   = 32
	SecureOnBytes   = 6
	maxLabelLength  = 63
	secureOnPattern = `^[0-9A-Fa-f]{2}([:-]?[0-9A-Fa-f]{2}){5}$`
)

var (
	ipv4Pattern     = regexp.MustCompile(`^((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)\.){3}(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)$`)
	ipv6CharPattern = regexp.MustCompile(`^[0-9A-Fa-f:.]+(%[A-Za-z0-9_.\-]+)?$`)
	secureOnRegexp  = regexp.MustCompile(secureOnPattern)
)

// Validate applique les règles de validation d'un terminal. Utilisée par le formulaire ET à l'import,
// pour ne jamais faire confiance à un fichier externe.
func Validate(d Device) []ValidationError {
	var errs []ValidationError
	add := func(f Field, msg string) { errs = append(errs, ValidationError{f, msg}) }

	name := strings.TrimSpace(d.Name)
	if name == "" {
		add(FieldName, "Le nom est obligatoire")
	}
	if UTF16Len(name) > MaxNameLength {
		add(FieldName, fmt.Sprintf("Nom trop long (max %d)", MaxNameLength))
	}
	if strings.IndexFunc(name, IsISOControl) >= 0 {
		add(FieldName, "Caractères interdits dans le nom")
	}
	if d.Host != "" && !IsValidHost(d.Host) {
		add(FieldHost, "Adresse IP ou nom d'hôte invalide")
	}
	if d.BroadcastAddress != nil && !IsIPv4(*d.BroadcastAddress) {
		add(FieldBroadcast, "Adresse de diffusion IPv4 invalide")
	}
	if !IsValidPort(d.WolPort) {
		add(FieldWolPort, "Port invalide (1-65535)")
	}
	if d.SecureOnPassword != nil && !secureOnRegexp.MatchString(*d.SecureOnPassword) {
		add(FieldSecureOn, "Mot de passe SecureOn : 6 octets hexadécimaux")
	}
	portsOK := len(d.ProbePorts) <= MaxProbePorts
	for _, p := range d.ProbePorts {
		portsOK = portsOK && IsValidPort(p)
	}
	if !portsOK {
		add(FieldProbePorts, fmt.Sprintf("Ports de détection invalides (max %d)", MaxProbePorts))
	}
	if d.Agent != nil {
		if !IsValidPort(d.Agent.Port) {
			add(FieldAgentPort, "Port invalide (1-65535)")
		}
		if d.Agent.HasKey() && DecodeAgentKey(d.Agent.Key) == nil {
			add(FieldAgentKey, "Clé d'agent invalide")
		}
		if !d.HasHost() {
			add(FieldHost, "L'adresse du PC est nécessaire pour l'agent")
		}
	}
	return errs
}

// IsValidPort indique un port TCP/UDP valide.
func IsValidPort(port int) bool { return port >= 1 && port <= 65535 }

// IsIPv4 reconnaît une adresse IPv4 en notation décimale pointée stricte.
func IsIPv4(value string) bool { return ipv4Pattern.MatchString(value) }

// IsValidHost accepte une IPv4, une IPv6 littérale ou un nom d'hôte RFC 1123 (ex. pc-bureau.local).
func IsValidHost(value string) bool {
	if value == "" || len(value) > MaxHostLength || value != strings.TrimSpace(value) {
		return false
	}
	if IsIPv4(value) {
		return true
	}
	if strings.Contains(value, ":") {
		colons := strings.Count(value, ":")
		return ipv6CharPattern.MatchString(value) && colons >= 2 && colons <= 7
	}
	// Un nom composé uniquement de chiffres et de points est une IPv4 mal formée.
	if strings.IndexFunc(value, func(r rune) bool { return !unicode.IsDigit(r) && r != '.' }) < 0 {
		return false
	}
	for _, label := range strings.Split(strings.TrimSuffix(value, "."), ".") {
		if !isHostnameLabel(label) {
			return false
		}
	}
	return true
}

func isHostnameLabel(label string) bool {
	if len(label) < 1 || len(label) > maxLabelLength || label[0] == '-' || label[len(label)-1] == '-' {
		return false
	}
	for i := 0; i < len(label); i++ {
		c := label[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	return true
}

// SecureOnPasswordBytes convertit un mot de passe SecureOn validé en 6 octets.
func SecureOnPasswordBytes(value string) ([]byte, error) {
	if !secureOnRegexp.MatchString(value) {
		return nil, fmt.Errorf("Mot de passe SecureOn invalide")
	}
	hex := strings.NewReplacer(":", "", "-", "").Replace(value)
	out := make([]byte, SecureOnBytes)
	for i := range out {
		out[i] = hexByte(hex[i*2], hex[i*2+1])
	}
	return out, nil
}

// DecodeAgentKey décode une clé d'agent (Base64 URL, 32 octets). Les espaces et le remplissage « = »
// éventuels (copier-coller) sont tolérés. Renvoie nil si la clé est invalide.
func DecodeAgentKey(value string) []byte {
	cleaned := strings.TrimRight(strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, value), "=")
	raw, err := base64.RawURLEncoding.DecodeString(cleaned)
	if err != nil || len(raw) != AgentKeyBytes {
		return nil
	}
	return raw
}

// IsISOControl reproduit Char.isISOControl de Kotlin (U+0000–U+001F et U+007F–U+009F).
func IsISOControl(r rune) bool { return r <= 0x1F || (r >= 0x7F && r <= 0x9F) }

// UTF16Len compte les unités UTF-16, comme String.length en Kotlin (limites identiques à Android).
func UTF16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return n
}

// TruncateUTF16 coupe s à max unités UTF-16 sans couper un caractère.
func TruncateUTF16(s string, limit int) string {
	n := 0
	for i, r := range s {
		n += utf16.RuneLen(r)
		if n > limit {
			return s[:i]
		}
	}
	return s
}

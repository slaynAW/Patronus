// Package model contient les données de l'application (PC, réglages) et leurs règles de validation.
// Le format JSON est identique à celui de l'application Android (core/model) : une sauvegarde
// exportée sur le téléphone s'importe telle quelle sur Windows, et inversement.
package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// MACLength est la taille d'une adresse MAC.
const MACLength = 6

// MAC est une adresse matérielle sur 6 octets.
//
// Formats acceptés : AA:BB:CC:DD:EE:FF, AA-BB-CC-DD-EE-FF, AABB.CCDD.EEFF (Cisco) et AABBCCDDEEFF,
// sans tenir compte de la casse. La forme canonique est AA:BB:CC:DD:EE:FF.
type MAC [MACLength]byte

// ParseMAC analyse une adresse MAC et refuse celles qui ne peuvent pas désigner une carte réseau.
func ParseMAC(input string) (MAC, error) {
	var mac MAC
	trimmed := strings.TrimSpace(input)
	var hex string
	switch {
	case isGrouped(trimmed):
		hex = strings.NewReplacer(":", "", "-", "").Replace(trimmed)
	case isCisco(trimmed):
		hex = strings.ReplaceAll(trimmed, ".", "")
	case len(trimmed) == 12 && allHex(trimmed):
		hex = trimmed
	default:
		return mac, fmt.Errorf("Adresse MAC invalide : « %s »", input)
	}
	allZero, allFF := true, true
	for i := range mac {
		mac[i] = hexByte(hex[i*2], hex[i*2+1])
		allZero = allZero && mac[i] == 0
		allFF = allFF && mac[i] == 0xFF
	}
	if allZero {
		return MAC{}, errors.New("L'adresse MAC ne peut pas être 00:00:00:00:00:00")
	}
	if allFF {
		return MAC{}, errors.New("L'adresse MAC ne peut pas être l'adresse de broadcast")
	}
	return mac, nil
}

func (m MAC) String() string {
	return fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X", m[0], m[1], m[2], m[3], m[4], m[5])
}

// IsZero indique une adresse non renseignée.
func (m MAC) IsZero() bool { return m == MAC{} }

func (m MAC) MarshalJSON() ([]byte, error) { return json.Marshal(m.String()) }

func (m *MAC) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return errors.New("adresse MAC : texte attendu")
	}
	parsed, err := ParseMAC(text)
	if err != nil {
		return err
	}
	*m = parsed
	return nil
}

// isGrouped reconnaît AA:BB:CC:DD:EE:FF ou AA-BB-CC-DD-EE-FF (même séparateur partout).
func isGrouped(s string) bool {
	if len(s) != 17 || (s[2] != ':' && s[2] != '-') {
		return false
	}
	for i := 0; i < len(s); i++ {
		if i%3 == 2 {
			if s[i] != s[2] {
				return false
			}
		} else if !isHex(s[i]) {
			return false
		}
	}
	return true
}

func isCisco(s string) bool {
	if len(s) != 14 || s[4] != '.' || s[9] != '.' {
		return false
	}
	return allHex(s[0:4]) && allHex(s[5:9]) && allHex(s[10:14])
}

func allHex(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isHex(s[i]) {
			return false
		}
	}
	return true
}

func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func hexByte(hi, lo byte) byte { return hexValue(hi)<<4 | hexValue(lo) }

func hexValue(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	default:
		return c - 'A' + 10
	}
}

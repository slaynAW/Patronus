package protocol2

import (
	"crypto/rand"
	"errors"
	"strings"
)

// ErrCode : code d'appairage invalide.
var ErrCode = errors.New("wolagent/2 : code d'appairage invalide")

// crockford est l'alphabet base32 de Crockford : chiffres et majuscules sans I, L, O ni U.
const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// CodeLength est la longueur d'un code d'appairage (8 caractères, 40 bits).
const CodeLength = 8

// NewCode tire un code d'appairage (§7.2).
func NewCode() string {
	var b [5]byte
	_, _ = rand.Read(b[:]) // ne renvoie jamais d'erreur
	return EncodeCode(b)
}

// EncodeCode écrit 40 bits en 8 caractères base32 de Crockford, bits de poids fort d'abord.
func EncodeCode(b [5]byte) string {
	bits := uint64(b[0])<<32 | uint64(b[1])<<24 | uint64(b[2])<<16 | uint64(b[3])<<8 | uint64(b[4])
	out := make([]byte, CodeLength)
	for i := range out {
		out[i] = crockford[bits>>(35-5*i)&31]
	}
	return string(out)
}

// NormalizeCode lit un code saisi ou lu dans un lien (§7.2) : espaces et tirets retirés, minuscules
// passées en majuscules, O lu 0, I et L lus 1. Il doit rester exactement 8 caractères de l'alphabet.
// Seuls les caractères ASCII sont transformés : tout autre caractère rend le code invalide.
func NormalizeCode(input string) (string, error) {
	out := make([]byte, 0, CodeLength)
	for i := 0; i < len(input); i++ {
		c := input[i]
		if c == ' ' || c == '-' {
			continue
		}
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		switch c {
		case 'O':
			c = '0'
		case 'I', 'L':
			c = '1'
		}
		if len(out) == CodeLength || strings.IndexByte(crockford, c) < 0 {
			return "", ErrCode
		}
		out = append(out, c)
	}
	if len(out) != CodeLength {
		return "", ErrCode
	}
	return string(out), nil
}

// FormatCode affiche un code normalisé en deux groupes : « 7KQ2-9MXP ».
func FormatCode(code string) string {
	if len(code) != CodeLength {
		return code
	}
	return code[:4] + "-" + code[4:]
}

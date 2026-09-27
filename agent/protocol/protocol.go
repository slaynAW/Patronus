// Package protocol implémente le protocole « wolagent/1 » (voir docs/PROTOCOLE.md).
//
// Chaque échange TCP se déroule ainsi (une ligne JSON par message) :
//
//	agent → app : {"proto":"wolagent/1","nonce":"<32 octets aléatoires>"}
//	app → agent : {"cnonce":"<16 octets>","body":"<JSON>","mac":"<HMAC>"}
//	agent → app : {"body":"<JSON>","mac":"<HMAC>"}   ou   {"error":"unauthorized"}
//
// Les signatures HMAC-SHA256 couvrent les deux nonces : un message capturé ne peut être ni rejoué
// ni modifié, et la clé partagée ne circule jamais sur le réseau.
package protocol

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
)

const (
	// Proto identifie la version du protocole.
	Proto = "wolagent/1"
	// ServerNonceBytes est la taille du défi envoyé par l'agent.
	ServerNonceBytes = 32
	// ClientNonceBytes est la taille du nonce choisi par l'application.
	ClientNonceBytes = 16
	// KeyBytes est la taille de la clé partagée.
	KeyBytes = 32
	// MaxLineBytes borne la taille d'un message.
	MaxLineBytes = 8 * 1024
)

// Erreurs renvoyées (non signées) quand l'authentification est impossible.
const (
	ErrUnauthorized = "unauthorized"
	ErrRateLimited  = "rate_limited"
	ErrBadRequest   = "bad_request"
)

// Hello est le premier message, envoyé par l'agent.
type Hello struct {
	Proto string `json:"proto"`
	Nonce string `json:"nonce"`
}

// Request est la requête signée de l'application.
type Request struct {
	CNonce string `json:"cnonce"`
	Body   string `json:"body"`
	Mac    string `json:"mac"`
}

// RequestBody est le contenu (signé) d'une requête.
type RequestBody struct {
	Cmd   string `json:"cmd"`
	Delay *int   `json:"delay,omitempty"`
	Force *bool  `json:"force,omitempty"`
}

// Response est la réponse de l'agent : signée (Body + Mac) ou erreur d'authentification (Error).
type Response struct {
	Body  string `json:"body,omitempty"`
	Mac   string `json:"mac,omitempty"`
	Error string `json:"error,omitempty"`
}

// ResponseBody est le contenu (signé) d'une réponse.
type ResponseBody struct {
	OK       bool   `json:"ok"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Version  string `json:"version"`
	Uptime   int64  `json:"uptime"`
}

var b64 = base64.RawURLEncoding

// RequestMAC calcule la signature d'une requête.
func RequestMAC(key []byte, nonce, cnonce, body string) string {
	return sign(key, Proto+"\nrequest\n"+nonce+"\n"+cnonce+"\n"+body)
}

// ResponseMAC calcule la signature d'une réponse.
func ResponseMAC(key []byte, nonce, cnonce, body string) string {
	return sign(key, Proto+"\nresponse\n"+nonce+"\n"+cnonce+"\n"+body)
}

// MACEqual compare deux signatures en temps constant.
func MACEqual(expected, received string) bool {
	a, err1 := b64.DecodeString(expected)
	b, err2 := b64.DecodeString(received)
	if err1 != nil || err2 != nil {
		return false
	}
	return hmac.Equal(a, b)
}

// NewNonce renvoie n octets aléatoires encodés en Base64 URL.
func NewNonce(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return b64.EncodeToString(buf), nil
}

// DecodedLen renvoie la taille décodée d'une valeur Base64 URL, ou -1 si elle est invalide.
func DecodedLen(value string) int {
	raw, err := b64.DecodeString(value)
	if err != nil {
		return -1
	}
	return len(raw)
}

// NewKey génère une clé partagée aléatoire de 256 bits.
func NewKey() (string, error) { return NewNonce(KeyBytes) }

// DecodeKey décode et vérifie une clé partagée.
func DecodeKey(value string) ([]byte, error) {
	raw, err := b64.DecodeString(value)
	if err != nil || len(raw) != KeyBytes {
		return nil, errors.New("clé invalide : 32 octets en Base64 URL attendus")
	}
	return raw, nil
}

func sign(key []byte, message string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(message))
	return b64.EncodeToString(mac.Sum(nil))
}

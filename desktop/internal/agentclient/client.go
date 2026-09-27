// Package agentclient est le client du protocole « wolagent/1 » (voir docs/PROTOCOLE.md), équivalent
// de AgentClient côté Android : état du PC, extinction, redémarrage et mise en veille authentifiés.
package agentclient

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol"
	"github.com/slaynaw/wakeonlan/desktop/internal/model"
	"github.com/slaynaw/wakeonlan/desktop/internal/netdial"
)

// Code est la cause d'un échec (mêmes valeurs que AgentError côté Android).
type Code string

const (
	// NoKey : aucune clé configurée pour ce PC.
	NoKey Code = "NO_KEY"
	// UnknownHost : nom d'hôte introuvable.
	UnknownHost Code = "UNKNOWN_HOST"
	// Unreachable : pas de réponse (PC éteint, pare-feu, mauvais réseau...).
	Unreachable Code = "UNREACHABLE"
	// Refused : le PC répond mais l'agent n'écoute pas (PC allumé, agent arrêté).
	Refused Code = "REFUSED"
	// Unauthorized : clé refusée par l'agent.
	Unauthorized Code = "UNAUTHORIZED"
	// RateLimited : trop d'échecs, l'agent bloque temporairement cet ordinateur.
	RateLimited Code = "RATE_LIMITED"
	// Protocol : réponse illisible ou signature invalide.
	Protocol Code = "PROTOCOL"
	// Rejected : l'agent a compris mais n'a pas pu exécuter la commande.
	Rejected Code = "REJECTED"
)

// Error est un échec d'échange avec l'agent.
type Error struct {
	Code   Code
	Detail string
}

func (e *Error) Error() string {
	if e.Detail == "" {
		return string(e.Code)
	}
	return string(e.Code) + " : " + e.Detail
}

// CodeOf renvoie le code d'une erreur de l'agent (Protocol pour une erreur inattendue).
func CodeOf(err error) Code {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return Protocol
}

// HostAnswered indique qu'un échec prouve malgré tout que la machine est allumée (elle a répondu).
func HostAnswered(err error) bool {
	if err == nil {
		return true
	}
	switch CodeOf(err) {
	case Refused, Unauthorized, RateLimited, Protocol, Rejected:
		return true
	}
	return false
}

// Action est une action d'alimentation.
type Action string

const (
	Shutdown Action = "shutdown"
	Reboot   Action = "reboot"
	Sleep    Action = "sleep"
)

// Valid indique une action connue.
func (a Action) Valid() bool { return a == Shutdown || a == Reboot || a == Sleep }

// Status contient les informations renvoyées par la commande « status ».
type Status struct {
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Version  string `json:"version"`
	Uptime   int64  `json:"uptime"`
}

// Client dialogue avec les agents.
type Client struct {
	ConnectTimeout time.Duration
	ReadTimeout    time.Duration
}

// New renvoie un client avec les délais de l'application Android (2 s / 4 s).
func New() *Client { return &Client{ConnectTimeout: 2 * time.Second, ReadTimeout: 4 * time.Second} }

// Status interroge l'agent.
func (c *Client) Status(ctx context.Context, host string, agent model.AgentSettings) (Status, error) {
	body, err := c.exchange(ctx, host, agent, protocol.RequestBody{Cmd: "status"})
	if err != nil {
		return Status{}, err
	}
	return Status{Hostname: body.Hostname, OS: body.OS, Arch: body.Arch, Version: body.Version, Uptime: body.Uptime}, nil
}

// Power demande une action d'alimentation. L'agent répond AVANT d'exécuter l'action.
func (c *Client) Power(ctx context.Context, host string, agent model.AgentSettings, action Action, delaySeconds int, force bool) (string, error) {
	if !action.Valid() {
		return "", &Error{Code: Rejected, Detail: "action inconnue"}
	}
	if delaySeconds < 0 || delaySeconds > 3600 {
		return "", &Error{Code: Rejected, Detail: "délai invalide"}
	}
	body, err := c.exchange(ctx, host, agent, protocol.RequestBody{Cmd: string(action), Delay: &delaySeconds, Force: &force})
	if err != nil {
		return "", err
	}
	return body.Message, nil
}

type responseBody struct {
	OK       *bool   `json:"ok"`
	Code     *string `json:"code"`
	Message  string  `json:"message"`
	Hostname string  `json:"hostname"`
	OS       string  `json:"os"`
	Arch     string  `json:"arch"`
	Version  string  `json:"version"`
	Uptime   int64   `json:"uptime"`
}

func (c *Client) exchange(ctx context.Context, host string, agent model.AgentSettings, request protocol.RequestBody) (*responseBody, error) {
	key := model.DecodeAgentKey(agent.Key)
	if key == nil {
		return nil, &Error{Code: NoKey}
	}
	conn, err := netdial.Dial(ctx, net.JoinHostPort(host, strconv.Itoa(agent.Port)), c.ConnectTimeout)
	if err != nil {
		return nil, dialError(err)
	}
	defer conn.Close()
	// Annulation : fermer la connexion débloque immédiatement les lectures en cours.
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()

	body, err := c.converse(conn, key, request)
	if err != nil {
		var e *Error
		if errors.As(err, &e) {
			return nil, e
		}
		// Connexion établie puis coupée / muette : la machine est allumée mais l'agent répond mal.
		return nil, &Error{Code: Protocol, Detail: err.Error()}
	}
	return body, nil
}

func (c *Client) converse(conn net.Conn, key []byte, request protocol.RequestBody) (*responseBody, error) {
	reader := bufio.NewReaderSize(conn, 4096)
	deadline := func() { _ = conn.SetDeadline(time.Now().Add(c.ReadTimeout)) }

	deadline()
	line, err := readLine(reader)
	if err != nil {
		return nil, err
	}
	var hello struct {
		Proto *string `json:"proto"`
		Nonce *string `json:"nonce"`
	}
	if json.Unmarshal(line, &hello) != nil || hello.Proto == nil || hello.Nonce == nil {
		// Un agent qui bloque cet ordinateur envoie directement une erreur à la place de la salutation.
		return nil, earlyError(line)
	}
	if *hello.Proto != protocol.Proto {
		return nil, protocolError("version de protocole inconnue : " + *hello.Proto)
	}
	if protocol.DecodedLen(*hello.Nonce) != protocol.ServerNonceBytes {
		return nil, protocolError("nonce invalide")
	}

	cnonce, err := protocol.NewNonce(protocol.ClientNonceBytes)
	if err != nil {
		return nil, err
	}
	bodyJSON, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	message, err := json.Marshal(protocol.Request{
		CNonce: cnonce,
		Body:   string(bodyJSON),
		Mac:    protocol.RequestMAC(key, *hello.Nonce, cnonce, string(bodyJSON)),
	})
	if err != nil {
		return nil, err
	}
	deadline()
	if _, err := conn.Write(append(message, '\n')); err != nil {
		return nil, err
	}

	deadline()
	line, err = readLine(reader)
	if err != nil {
		return nil, err
	}
	var response protocol.Response
	if json.Unmarshal(line, &response) != nil {
		return nil, protocolError("réponse illisible")
	}
	if response.Error != "" {
		return nil, errorFrom(response.Error)
	}
	if response.Body == "" {
		return nil, protocolError("réponse sans contenu")
	}
	if response.Mac == "" {
		return nil, protocolError("réponse non signée")
	}
	expected := protocol.ResponseMAC(key, *hello.Nonce, cnonce, response.Body)
	if !protocol.MACEqual(expected, response.Mac) {
		return nil, protocolError("signature de la réponse invalide")
	}
	var body responseBody
	if json.Unmarshal([]byte(response.Body), &body) != nil || body.OK == nil || body.Code == nil {
		return nil, protocolError("contenu de réponse illisible")
	}
	if !*body.OK {
		return nil, &Error{Code: Rejected, Detail: body.Message}
	}
	return &body, nil
}

func earlyError(line []byte) error {
	var response protocol.Response
	if json.Unmarshal(line, &response) == nil && response.Error != "" {
		return errorFrom(response.Error)
	}
	return protocolError("salutation illisible")
}

func errorFrom(code string) error {
	switch code {
	case protocol.ErrUnauthorized:
		return &Error{Code: Unauthorized}
	case protocol.ErrRateLimited:
		return &Error{Code: RateLimited}
	}
	return protocolError("erreur de l'agent : " + code)
}

func protocolError(detail string) error { return &Error{Code: Protocol, Detail: detail} }

// readLine lit une ligne terminée par « \n », de taille bornée (protection contre un pair malveillant).
func readLine(r *bufio.Reader) ([]byte, error) {
	var line []byte
	for {
		chunk, err := r.ReadSlice('\n')
		line = append(line, chunk...)
		if len(line) > protocol.MaxLineBytes+1 {
			return nil, errors.New("message trop long")
		}
		switch {
		case err == nil:
			return bytes.TrimRight(line, "\r\n"), nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF) && len(line) > 0:
			return line, nil
		default:
			return nil, err
		}
	}
}

func dialError(err error) error {
	var dnsErr *net.DNSError
	switch {
	case errors.As(err, &dnsErr):
		return &Error{Code: UnknownHost, Detail: dnsErr.Error()}
	case netdial.IsRefused(err):
		return &Error{Code: Refused, Detail: err.Error()}
	default:
		return &Error{Code: Unreachable, Detail: fmt.Sprint(err)}
	}
}

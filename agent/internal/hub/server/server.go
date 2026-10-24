// Package server sert « patronus-hub/1 » aux applications (docs/PROTOCOLE-HUB.md) : salut, TLS 1.3 à
// authentification mutuelle avec l'identité du hub, puis requêtes et réponses en trames, avec les rôles
// du hub (consulter, démarrer, administrer). Les appareils autorisés et les invitations sont ceux du
// registre de l'agent (internal/devices), gardé dans le stockage chiffré du hub.
package server

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/slaynaw/wakeonlan/agent/hubprotocol"
	"github.com/slaynaw/wakeonlan/agent/internal/config"
	"github.com/slaynaw/wakeonlan/agent/internal/devices"
	"github.com/slaynaw/wakeonlan/agent/internal/guard"
	"github.com/slaynaw/wakeonlan/agent/internal/identity"
	"github.com/slaynaw/wakeonlan/agent/protocol2"
)

const (
	// maxConnections borne les connexions simultanées (§3).
	maxConnections = 16
	// unknownIdleTimeout : une identité inconnue n'a droit qu'à ce délai entre deux requêtes (§3).
	unknownIdleTimeout = 10 * time.Second
	// writeTimeout borne l'envoi d'une réponse.
	writeTimeout = 30 * time.Second
	// LocalName désigne, dans les journaux, la commande patronus-hub lancée sur le NAS (§8).
	LocalName = "patronus-hub (sur le NAS)"
)

// Messages des refus : les mêmes quelle qu'en soit la raison exacte.
const (
	msgUnknownDevice = "appareil inconnu de ce hub : appairez-le (lien du hub ou invitation)"
	msgPairingFailed = "code d'appairage invalide, expiré ou déjà utilisé"
)

var (
	errALPN      = errors.New("protocole applicatif absent ou différent de " + hubprotocol.ALPN)
	errCertCount = errors.New("un seul certificat attendu")
)

// Config décrit le hub servi.
type Config struct {
	Name     string // nom du hub (« Hub NAS »)
	Version  string
	Identity *identity.Identity
	Registry *devices.Registry
	Logger   *log.Logger
	Now      func() time.Time // time.Now par défaut
}

// Server sert patronus-hub/1.
type Server struct {
	cfg      Config
	networks []*net.IPNet
	limiter  *guard.Limiter
	pairing  *guard.Pairing
	tls      *tls.Config
	started  time.Time
	slots    chan struct{}

	mu    sync.Mutex
	conns map[protocol2.Pin]map[*tls.Conn]struct{} // connexions ouvertes, fermées à la révocation
}

// New prépare le serveur. Seules les adresses des réseaux privés sont acceptées, comme la liste
// « allow » par défaut d'un agent.
func New(cfg Config) (*Server, error) {
	if cfg.Identity == nil || cfg.Registry == nil || cfg.Logger == nil {
		return nil, errors.New("hub : identité, registre et journal requis")
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	allow := config.Config{Allow: config.DefaultAllow}
	networks, err := allow.Networks()
	if err != nil {
		return nil, err
	}
	s := &Server{
		cfg:      cfg,
		networks: networks,
		limiter:  guard.NewLimiter(5, time.Minute, 5*time.Minute),
		pairing:  guard.NewPairing(cfg.Now),
		started:  cfg.Now(),
		slots:    make(chan struct{}, maxConnections),
		conns:    map[protocol2.Pin]map[*tls.Conn]struct{}{},
	}
	s.tls = &tls.Config{
		MinVersion:       tls.VersionTLS13,
		MaxVersion:       tls.VersionTLS13,
		Certificates:     []tls.Certificate{cfg.Identity.Cert},
		NextProtos:       []string{hubprotocol.ALPN},
		CurvePreferences: protocol2.Curves,
		// Toute identité P-256 passe TLS ; le registre décide ensuite, commande par commande.
		ClientAuth: tls.RequireAnyClientCert,
		// Chaque connexion s'authentifie complètement : une révocation s'applique tout de suite.
		SessionTicketsDisabled: true,
		VerifyConnection: func(cs tls.ConnectionState) error {
			if cs.NegotiatedProtocol != hubprotocol.ALPN {
				return errALPN
			}
			if len(cs.PeerCertificates) != 1 {
				return errCertCount
			}
			_, err := protocol2.PinFromCertificate(cs.PeerCertificates[0].Raw)
			return err
		},
	}
	return s, nil
}

// Serve accepte les connexions jusqu'à la fermeture de ln.
func (s *Server) Serve(ln net.Listener) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			return err
		}
		select {
		case s.slots <- struct{}{}:
			go func() {
				defer func() { <-s.slots }()
				s.handle(conn)
			}()
		default:
			_ = conn.Close() // trop de connexions simultanées
		}
	}
}

func (s *Server) allowed(ip net.IP) bool {
	for _, n := range s.networks {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// handle mène une connexion : salut, poignée de main TLS, puis requêtes et réponses.
func (s *Server) handle(raw net.Conn) {
	defer raw.Close()
	remote, _ := raw.RemoteAddr().(*net.TCPAddr)
	if remote == nil || !s.allowed(remote.IP) {
		if remote != nil {
			s.cfg.Logger.Printf("connexion refusée depuis %s (réseau non autorisé)", remote.IP)
		}
		return
	}
	ip := remote.IP.String()
	_ = raw.SetDeadline(s.cfg.Now().Add(protocol2.HandshakeTimeout))
	if s.limiter.Blocked(ip) {
		_, _ = raw.Write([]byte(`{"error":"rate_limited"}` + "\n"))
		return
	}
	if _, err := raw.Write([]byte(hubprotocol.Hello + "\n")); err != nil {
		return
	}
	conn := tls.Server(raw, s.tls)
	if err := conn.Handshake(); err != nil {
		if guard.Refused(err) {
			s.failure(ip, "connexion refusée ("+err.Error()+")")
		}
		return
	}
	pin, err := protocol2.PinFromCertificate(conn.ConnectionState().PeerCertificates[0].Raw)
	if err != nil {
		return // déjà vérifiée pendant la poignée de main
	}
	sess := &session{s: s, conn: conn, ip: ip, pin: pin}
	// L'identité du hub lui-même, depuis le NAS : la commande patronus-hub, qui peut lire sa clé (§8).
	sess.local = pin == s.cfg.Identity.Pin && isLoopback(remote.IP)
	s.track(pin, conn)
	defer s.untrack(pin, conn)
	if _, ok := s.cfg.Registry.Lookup(pin); ok {
		s.cfg.Registry.Seen(pin)
		s.limiter.Success(ip)
	}
	for range protocol2.MaxRequestsPerConnection {
		idle := protocol2.IdleTimeout
		if _, registered := sess.device(); !registered {
			idle = unknownIdleTimeout
		}
		_ = conn.SetReadDeadline(s.cfg.Now().Add(idle))
		payload, err := protocol2.ReadFrame(conn, protocol2.MaxRequestBytes)
		if err != nil {
			return // fin, délai dépassé, ou trame vide, trop longue ou tronquée : fermeture sans réponse
		}
		resp, closing := sess.handle(payload)
		_ = conn.SetWriteDeadline(s.cfg.Now().Add(writeTimeout))
		if protocol2.WriteFrame(conn, resp, protocol2.MaxResponseBytes) != nil || closing {
			return
		}
	}
}

func isLoopback(ip net.IP) bool { return ip.IsLoopback() }

func (s *Server) track(pin protocol2.Pin, c *tls.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conns[pin] == nil {
		s.conns[pin] = map[*tls.Conn]struct{}{}
	}
	s.conns[pin][c] = struct{}{}
}

func (s *Server) untrack(pin protocol2.Pin, c *tls.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.conns[pin], c)
	if len(s.conns[pin]) == 0 {
		delete(s.conns, pin)
	}
}

// disconnect ferme les connexions ouvertes d'un appareil révoqué, sauf except, qui répond d'abord.
func (s *Server) disconnect(pin protocol2.Pin, except *tls.Conn) {
	s.mu.Lock()
	var open []*tls.Conn
	for c := range s.conns[pin] {
		if c != except {
			open = append(open, c)
		}
	}
	s.mu.Unlock()
	for _, c := range open {
		_ = c.Close()
	}
}

// failure compte un échec de l'adresse et le note au journal.
func (s *Server) failure(ip, why string) {
	if s.limiter.Fail(ip) {
		s.cfg.Logger.Printf("%s bloqué 5 minutes après plusieurs échecs (%s)", ip, why)
		return
	}
	s.cfg.Logger.Printf("%s : %s", ip, why)
}

// session est l'état d'une connexion.
type session struct {
	s      *Server
	conn   *tls.Conn
	ip     string
	pin    protocol2.Pin
	local  bool
	lastID int64
}

// device renvoie l'appareil de la session ; patronus-hub sur le NAS a le rôle admin sans être enregistré.
func (sess *session) device() (devices.Device, bool) {
	if sess.local {
		return devices.Device{Pin: sess.pin, Name: LocalName, Role: protocol2.RoleAdmin, Via: devices.ViaLocal}, true
	}
	return sess.s.cfg.Registry.Lookup(sess.pin)
}

// by désigne l'auteur d'une invitation : l'empreinte de l'administrateur, ou « local ».
func (sess *session) by() string {
	if sess.local {
		return devices.ByLocal
	}
	return sess.pin.String()
}

func (sess *session) answer(id int64, d devices.Device, code, message string) hubprotocol.Response {
	return hubprotocol.Response{ID: id, OK: code == protocol2.CodeOK, Code: code, Message: message, Role: d.Role.String()}
}

// handle traite une requête et renvoie la réponse, et s'il faut ensuite fermer la connexion.
func (sess *session) handle(payload []byte) ([]byte, bool) {
	var req hubprotocol.Request
	err := json.Unmarshal(payload, &req)
	if err != nil || req.ID < 1 || req.ID > protocol2.MaxRequestID || req.ID <= sess.lastID {
		id := int64(0)
		if err == nil {
			id = req.ID
		}
		return encode(protocol2.Reply{ID: id, Code: protocol2.CodeBadRequest, Message: "requête illisible ou identifiant invalide"}), true
	}
	sess.lastID = req.ID
	d, registered := sess.device()
	switch code := hubprotocol.Authorize(registered, d.Role, req.Cmd); code {
	case protocol2.CodeOK:
	case protocol2.CodeUnknownDevice:
		sess.s.failure(sess.ip, "identité inconnue")
		return encode(protocol2.Reply{ID: req.ID, Code: code, Message: msgUnknownDevice}), true
	case protocol2.CodeAlreadyPaired:
		return encode(sess.answer(req.ID, d, code, "cet appareil est déjà autorisé sur ce hub")), false
	case protocol2.CodeInsufficientRole:
		return encode(sess.answer(req.ID, d, code, fmt.Sprintf("le rôle « %s » ne permet pas « %s »", d.Role, req.Cmd))), false
	default:
		return encode(sess.answer(req.ID, d, code, fmt.Sprintf("commande inconnue : %s", req.Cmd))), false
	}
	if !registered {
		return sess.pair(req)
	}
	return sess.command(req, d)
}

// command exécute la commande d'un appareil enregistré, rôle déjà vérifié.
func (sess *session) command(req hubprotocol.Request, d devices.Device) ([]byte, bool) {
	s, registry := sess.s, sess.s.cfg.Registry
	ok := func() hubprotocol.Response { return sess.answer(req.ID, d, protocol2.CodeOK, "") }
	bad := func(code, message string) ([]byte, bool) { return encode(sess.answer(req.ID, d, code, message)), false }
	force := sess.local && req.Force != nil && *req.Force

	switch req.Cmd {
	case hubprotocol.CmdStatus:
		resp := ok()
		resp.Hub = &hubprotocol.Info{
			Name: s.cfg.Name, Version: s.cfg.Version, Pin: s.cfg.Identity.Pin.String(), Started: s.started.Unix(),
		}
		return encode(resp), false

	case protocol2.CmdDevices:
		resp := ok()
		resp.Devices = []protocol2.DeviceInfo{}
		for _, x := range registry.Devices() {
			resp.Devices = append(resp.Devices, protocol2.DeviceInfo{
				Device: x.Pin.String(), Name: x.Name, Role: x.Role.String(), Added: x.Added, Via: x.Via, Seen: x.Seen,
				Self: x.Pin == sess.pin,
			})
		}
		return encode(resp), false

	case protocol2.CmdDevicesRevoke, protocol2.CmdDevicesRole, protocol2.CmdDevicesRename:
		target, err := protocol2.ParsePin(req.Device)
		if err != nil {
			return bad(protocol2.CodeBadRequest, "empreinte de l'appareil invalide")
		}
		var changed devices.Device
		var action string
		switch req.Cmd {
		case protocol2.CmdDevicesRevoke:
			action = "retiré"
			changed, err = registry.Remove(target, force)
		case protocol2.CmdDevicesRole:
			role, valid := hubprotocol.ParseRole(req.Role)
			if !valid {
				return bad(protocol2.CodeBadRequest, "rôle invalide : view, wake ou admin")
			}
			action = "change de rôle"
			changed, err = registry.SetRole(target, role, force)
		default:
			name := protocol2.CleanName(req.Name, protocol2.MaxDeviceNameLength)
			if name == "" {
				return bad(protocol2.CodeBadRequest, "nom vide")
			}
			action = "renommé"
			changed, err = registry.Rename(target, name)
		}
		switch {
		case errors.Is(err, devices.ErrUnknownDevice):
			return bad(protocol2.CodeBadRequest, "appareil introuvable")
		case errors.Is(err, devices.ErrLastAdmin):
			return bad(protocol2.CodeLastAdmin, "c'est le dernier administrateur du hub : impossible depuis une application")
		case err != nil:
			s.cfg.Logger.Printf("registre des appareils : %v", err)
			return bad(protocol2.CodeBusy, "registre des appareils non enregistré : réessayez")
		}
		s.cfg.Logger.Printf("appareil « %s » [%s] %s par %s, rôle %s", changed.Name, changed.Pin.Short(), action, d.Name, changed.Role)
		if req.Cmd != protocol2.CmdDevicesRevoke {
			return encode(ok()), false
		}
		s.disconnect(target, sess.conn)
		return encode(ok()), target == sess.pin

	case protocol2.CmdInvite:
		role := hubprotocol.DefaultInviteRole
		if req.Role != "" {
			var valid bool
			if role, valid = hubprotocol.ParseRole(req.Role); !valid {
				return bad(protocol2.CodeBadRequest, "rôle invalide : view, wake ou admin")
			}
		}
		ttl := req.TTL
		if ttl == 0 {
			ttl = protocol2.DefaultInviteTTL
		}
		if ttl < protocol2.MinInviteTTL || ttl > protocol2.MaxInviteTTL {
			return bad(protocol2.CodeBadRequest, "durée de validité invalide : de 60 s à 7 jours")
		}
		var only *protocol2.Pin
		if req.Device != "" {
			pin, err := protocol2.ParsePin(req.Device)
			if err != nil {
				return bad(protocol2.CodeBadRequest, "empreinte de l'appareil invalide")
			}
			only = &pin
		}
		name := protocol2.CleanName(req.Name, protocol2.MaxDeviceNameLength)
		inv, code, err := registry.CreateInvite(role, name, time.Duration(ttl)*time.Second, only, sess.by())
		switch {
		case errors.Is(err, devices.ErrTooManyInvites):
			return bad(protocol2.CodeBadRequest, err.Error())
		case err != nil:
			s.cfg.Logger.Printf("registre des appareils : %v", err)
			return bad(protocol2.CodeBusy, "invitation non enregistrée : réessayez")
		}
		s.cfg.Logger.Printf("invitation créée par %s : rôle %s, valable jusqu'au %s", d.Name, role,
			time.Unix(inv.Expires, 0).Format("02/01/2006 15:04"))
		resp := ok()
		resp.Invite = &hubprotocol.InviteInfo{
			ID: inv.ID, Code: code, Role: inv.Role.String(), Name: inv.Name, Expires: inv.Expires, Hub: s.cfg.Identity.Pin.String(),
		}
		if inv.Device != nil {
			resp.Invite.Device = inv.Device.String()
		}
		return encode(resp), false

	case protocol2.CmdInvites:
		resp := ok()
		resp.Invites = []protocol2.InviteInfo{}
		for _, inv := range registry.Invites() {
			info := protocol2.InviteInfo{ID: inv.ID, Role: inv.Role.String(), Name: inv.Name, Expires: inv.Expires}
			if inv.Device != nil {
				info.Device = inv.Device.String()
			}
			resp.Invites = append(resp.Invites, info)
		}
		return encode(resp), false

	case protocol2.CmdInvitesCancel:
		switch err := registry.CancelInvite(req.Invite); {
		case errors.Is(err, devices.ErrUnknownInvite):
			return bad(protocol2.CodeBadRequest, "invitation introuvable")
		case err != nil:
			s.cfg.Logger.Printf("registre des appareils : %v", err)
			return bad(protocol2.CodeBusy, "annulation non enregistrée : réessayez")
		}
		return encode(ok()), false
	}
	// Commande du protocole que ce hub ne sert pas encore (étapes suivantes de la 2.0).
	return bad(protocol2.CodeUnsupported, fmt.Sprintf("commande pas encore disponible sur ce hub : %s", req.Cmd))
}

// pair enregistre une identité inconnue avec une invitation.
func (sess *session) pair(req hubprotocol.Request) ([]byte, bool) {
	s, registry := sess.s, sess.s.cfg.Registry
	refuse := func() ([]byte, bool) {
		s.failure(sess.ip, "code d'appairage refusé")
		if s.pairing.Fail() {
			n, err := registry.CancelInvites()
			if err != nil {
				s.cfg.Logger.Printf("registre des appareils : %v", err)
			}
			s.cfg.Logger.Printf("appairage suspendu 10 minutes après plus de %d échecs ; %d invitation(s) annulée(s)", guard.PairingMaxFailures, n)
		}
		return encode(protocol2.Reply{ID: req.ID, Code: protocol2.CodePairingFailed, Message: msgPairingFailed}), true
	}
	if s.pairing.Suspended() {
		return refuse()
	}
	name := protocol2.CleanName(req.Name, protocol2.MaxDeviceNameLength)
	if name == "" {
		name = "Appareil"
	}
	d, err := registry.Redeem(req.Code, sess.pin, name)
	switch {
	case errors.Is(err, devices.ErrPairingFailed):
		return refuse()
	case errors.Is(err, devices.ErrAlreadyPaired):
		d, _ = registry.Lookup(sess.pin)
		return encode(sess.answer(req.ID, d, protocol2.CodeAlreadyPaired, "cet appareil est déjà autorisé sur ce hub")), false
	case err != nil:
		s.cfg.Logger.Printf("registre des appareils : %v", err)
		return encode(protocol2.Reply{ID: req.ID, Code: protocol2.CodeBusy, Message: "appairage non enregistré : réessayez"}), true
	}
	s.cfg.Logger.Printf("appareil « %s » [%s] ajouté (code d'appairage), rôle %s", d.Name, d.Pin.Short(), d.Role)
	s.limiter.Success(sess.ip)
	resp := sess.answer(req.ID, d, protocol2.CodeOK, "appareil ajouté")
	resp.Device = sess.pin.String()
	return encode(resp), false
}

func encode(v any) []byte {
	raw, _ := json.Marshal(v)
	return raw
}

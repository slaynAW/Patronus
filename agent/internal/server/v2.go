package server

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"syscall"
	"time"

	"github.com/slaynaw/wakeonlan/agent/internal/devices"
	"github.com/slaynaw/wakeonlan/agent/internal/identity"
	"github.com/slaynaw/wakeonlan/agent/protocol"
	"github.com/slaynaw/wakeonlan/agent/protocol2"
)

// writeTimeout borne l'envoi d'une réponse wolagent/2.
const writeTimeout = 30 * time.Second

// Messages des refus : les mêmes quelle qu'en soit la raison exacte (§6.3, §9).
const (
	msgUnknownDevice   = "appareil inconnu de ce PC : appairez-le (QR code ou invitation)"
	msgPairingFailed   = "code d'appairage invalide, expiré ou déjà utilisé"
	msgMigrationFailed = "migration impossible"
)

var (
	errALPN      = errors.New("protocole applicatif absent ou différent de wolagent/2")
	errCertCount = errors.New("un seul certificat attendu")
)

// v2Server sert wolagent/2 (docs/PROTOCOLE-V2.md) sur le port de wolagent/1.
type v2Server struct {
	identity *identity.Identity
	registry *devices.Registry
	tls      *tls.Config
	pairing  *pairingGuard

	mu    sync.Mutex
	conns map[protocol2.Pin]map[*tls.Conn]struct{} // connexions ouvertes, fermées à la révocation
}

// EnableV2 active wolagent/2, avec l'identité de l'agent et le registre des appareils.
func (s *Server) EnableV2(id *identity.Identity, registry *devices.Registry) {
	s.v2 = &v2Server{
		identity: id,
		registry: registry,
		pairing:  newPairingGuard(time.Now),
		conns:    map[protocol2.Pin]map[*tls.Conn]struct{}{},
		tls: &tls.Config{
			MinVersion:       tls.VersionTLS13,
			MaxVersion:       tls.VersionTLS13,
			Certificates:     []tls.Certificate{id.Cert},
			NextProtos:       []string{protocol2.ALPN},
			CurvePreferences: protocol2.Curves,
			// Toute identité P-256 passe TLS ; le registre décide ensuite, commande par commande (§3.2).
			ClientAuth: tls.RequireAnyClientCert,
			// Chaque connexion s'authentifie complètement : une révocation s'applique tout de suite.
			SessionTicketsDisabled: true,
			VerifyConnection: func(cs tls.ConnectionState) error {
				if cs.NegotiatedProtocol != protocol2.ALPN {
					return errALPN
				}
				if len(cs.PeerCertificates) != 1 {
					return errCertCount
				}
				_, err := protocol2.PinFromCertificate(cs.PeerCertificates[0].Raw)
				return err
			},
		},
	}
}

// bufferedConn rend à TLS ce que la lecture du premier octet a déjà mis en mémoire.
type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) { return c.r.Read(p) }

// serve mène une connexion wolagent/2 : poignée de main TLS, puis requêtes et réponses.
func (v *v2Server) serve(s *Server, raw net.Conn, buffered *bufio.Reader, ip string) {
	conn := tls.Server(&bufferedConn{Conn: raw, r: buffered}, v.tls)
	_ = raw.SetDeadline(time.Now().Add(protocol2.HandshakeTimeout))
	if err := conn.Handshake(); err != nil {
		if refused(err) {
			s.failure(ip, "connexion wolagent/2 refusée ("+err.Error()+")")
		}
		return
	}
	pin, err := protocol2.PinFromCertificate(conn.ConnectionState().PeerCertificates[0].Raw)
	if err != nil {
		return // déjà vérifiée pendant la poignée de main
	}
	sess := &v2Session{s: s, v: v, conn: conn, ip: ip, pin: pin}
	// L'identité de l'agent lui-même, depuis le PC : c'est wol-agent, lancé par un administrateur (§8).
	sess.local = pin == v.identity.Pin && isLoopback(ip)
	v.track(pin, conn)
	defer v.untrack(pin, conn)
	if _, ok := v.registry.Lookup(pin); ok {
		v.registry.Seen(pin)
		s.limiter.Success(ip)
	}
	for range protocol2.MaxRequestsPerConnection {
		// Une identité inconnue n'a droit qu'au délai d'une connexion wolagent/1.
		idle := protocol2.IdleTimeout
		if _, registered := sess.device(); !registered {
			idle = connectionTimeout
		}
		_ = conn.SetReadDeadline(time.Now().Add(idle))
		payload, err := protocol2.ReadFrame(conn, protocol2.MaxRequestBytes)
		if err != nil {
			return // fin, délai dépassé, ou trame vide, trop longue ou tronquée : fermeture sans réponse
		}
		resp, closing := sess.handle(payload)
		_ = conn.SetWriteDeadline(time.Now().Add(writeTimeout))
		if protocol2.WriteFrame(conn, resp, protocol2.MaxResponseBytes) != nil || closing {
			return
		}
	}
}

// refused distingue un refus de TLS (version, ALPN, certificat), compté comme un échec de l'adresse,
// d'une connexion simplement interrompue ou refusée par le client lui-même.
func refused(err error) bool {
	var ne net.Error
	var op *net.OpError
	switch {
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, net.ErrClosed),
		errors.Is(err, syscall.ECONNRESET), errors.As(err, &ne) && ne.Timeout():
		return false
	case errors.As(err, &op) && op.Op == "remote error": // alerte du client : son refus, pas le nôtre
		return false
	}
	return true
}

func isLoopback(ip string) bool {
	parsed := net.ParseIP(ip)
	return parsed != nil && parsed.IsLoopback()
}

func (v *v2Server) track(pin protocol2.Pin, c *tls.Conn) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.conns[pin] == nil {
		v.conns[pin] = map[*tls.Conn]struct{}{}
	}
	v.conns[pin][c] = struct{}{}
}

func (v *v2Server) untrack(pin protocol2.Pin, c *tls.Conn) {
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.conns[pin], c)
	if len(v.conns[pin]) == 0 {
		delete(v.conns, pin)
	}
}

// disconnect ferme les connexions ouvertes d'un appareil révoqué (§8), sauf except, qui répond d'abord.
func (v *v2Server) disconnect(pin protocol2.Pin, except *tls.Conn) {
	v.mu.Lock()
	var open []*tls.Conn
	for c := range v.conns[pin] {
		if c != except {
			open = append(open, c)
		}
	}
	v.mu.Unlock()
	for _, c := range open {
		_ = c.Close()
	}
}

// v2Session est l'état d'une connexion wolagent/2.
type v2Session struct {
	s         *Server
	v         *v2Server
	conn      *tls.Conn
	ip        string
	pin       protocol2.Pin
	local     bool
	lastID    int64
	migration *migration
}

// migration garde la première étape d'une migration (§9), le temps de la seconde.
type migration struct {
	nc, na protocol2.Nonce
	name   string
}

// device renvoie l'appareil de la session ; wol-agent sur le PC a le rôle admin sans être enregistré.
func (sess *v2Session) device() (devices.Device, bool) {
	if sess.local {
		return devices.Device{Pin: sess.pin, Name: "wol-agent (sur le PC)", Role: protocol2.RoleAdmin, Via: devices.ViaLocal}, true
	}
	return sess.v.registry.Lookup(sess.pin)
}

// by désigne l'auteur d'une invitation ou d'un ajout : l'empreinte de l'administrateur, ou « local ».
func (sess *v2Session) by() string {
	if sess.local {
		return devices.ByLocal
	}
	return sess.pin.String()
}

// handle traite une requête et renvoie la réponse, et s'il faut ensuite fermer la connexion.
func (sess *v2Session) handle(payload []byte) ([]byte, bool) {
	var req protocol2.Request
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
	switch code := protocol2.Authorize(registered, d.Role, req.Cmd); code {
	case protocol2.CodeOK:
	case protocol2.CodeUnknownDevice:
		sess.s.failure(sess.ip, "identité inconnue")
		return encode(protocol2.Reply{ID: req.ID, Code: code, Message: msgUnknownDevice}), true
	case protocol2.CodeAlreadyPaired:
		return encode(sess.answer(req.ID, d, code, "cet appareil est déjà autorisé sur ce PC")), false
	case protocol2.CodeInsufficientRole:
		return encode(sess.answer(req.ID, d, code, fmt.Sprintf("le rôle « %s » ne permet pas « %s »", d.Role, req.Cmd))), false
	default:
		return encode(sess.answer(req.ID, d, code, fmt.Sprintf("commande inconnue : %s", req.Cmd))), false
	}
	switch {
	case !registered && req.Cmd == protocol2.CmdPair:
		return sess.pair(req)
	case !registered:
		return sess.migrate(req)
	}
	return sess.command(req, d)
}

// answer prépare la réponse à un appareil enregistré.
func (sess *v2Session) answer(id int64, d devices.Device, code, message string) protocol2.Response {
	body := sess.s.newResponse()
	body.OK, body.Code, body.Message = code == protocol2.CodeOK, code, message
	return protocol2.Response{ID: id, ResponseBody: body, Role: d.Role.String()}
}

// command exécute la commande d'un appareil enregistré, rôle déjà vérifié.
func (sess *v2Session) command(req protocol2.Request, d devices.Device) ([]byte, bool) {
	s, v := sess.s, sess.v
	ok := func(message string) protocol2.Response { return sess.answer(req.ID, d, protocol2.CodeOK, message) }
	bad := func(code, message string) ([]byte, bool) { return encode(sess.answer(req.ID, d, code, message)), false }
	force := sess.local && req.Force != nil && *req.Force

	switch req.Cmd {
	case protocol2.CmdDevices:
		resp := ok("")
		resp.Devices = []protocol2.DeviceInfo{}
		for _, x := range v.registry.Devices() {
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
			action = protocol.DeviceRevoked
			changed, err = v.registry.Remove(target, force)
		case protocol2.CmdDevicesRole:
			role, valid := protocol2.ParseRole(req.Role)
			if !valid {
				return bad(protocol2.CodeBadRequest, "rôle invalide : view, wake, control ou admin")
			}
			action = protocol.DeviceRole
			changed, err = v.registry.SetRole(target, role, force)
		default:
			name := CleanName(req.Name)
			if name == "" {
				return bad(protocol2.CodeBadRequest, "nom vide")
			}
			action = protocol.DeviceRenamed
			changed, err = v.registry.Rename(target, name)
		}
		switch {
		case errors.Is(err, devices.ErrUnknownDevice):
			return bad(protocol2.CodeBadRequest, "appareil introuvable")
		case errors.Is(err, devices.ErrLastAdmin):
			return bad(protocol2.CodeLastAdmin, "c'est le dernier administrateur du PC : impossible depuis une application")
		case err != nil:
			s.logger.Printf("registre des appareils : %v", err)
			return bad(protocol2.CodeBusy, "registre des appareils non enregistré : réessayez")
		}
		s.noteDevice(action, changed, d)
		resp := ok("")
		if req.Cmd != protocol2.CmdDevicesRevoke {
			return encode(resp), false
		}
		v.disconnect(target, sess.conn)
		return encode(resp), target == sess.pin

	case protocol2.CmdInvite:
		role := protocol2.RoleControl
		if req.Role != "" {
			var valid bool
			if role, valid = protocol2.ParseRole(req.Role); !valid {
				return bad(protocol2.CodeBadRequest, "rôle invalide : view, wake, control ou admin")
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
		inv, code, err := v.registry.CreateInvite(role, CleanName(req.Name), time.Duration(ttl)*time.Second, only, sess.by())
		switch {
		case errors.Is(err, devices.ErrTooManyInvites):
			return bad(protocol2.CodeBadRequest, err.Error())
		case err != nil:
			s.logger.Printf("registre des appareils : %v", err)
			return bad(protocol2.CodeBusy, "invitation non enregistrée : réessayez")
		}
		s.logger.Printf("invitation créée par %s : rôle %s, valable jusqu'au %s", requester{ip: sess.ip, name: d.Name, id: d.Pin.Short()},
			role, time.Unix(inv.Expires, 0).Format("02/01/2006 15:04"))
		resp := ok("")
		info := inviteInfo(inv)
		info.Code, info.Agent = code, v.identity.Pin.String()
		resp.Invite = &info
		return encode(resp), false

	case protocol2.CmdInvites:
		resp := ok("")
		resp.Invites = []protocol2.InviteInfo{}
		for _, inv := range v.registry.Invites() {
			resp.Invites = append(resp.Invites, inviteInfo(inv))
		}
		return encode(resp), false

	case protocol2.CmdInvitesCancel:
		switch err := v.registry.CancelInvite(req.Invite); {
		case errors.Is(err, devices.ErrUnknownInvite):
			return bad(protocol2.CodeBadRequest, "invitation introuvable")
		case err != nil:
			s.logger.Printf("registre des appareils : %v", err)
			return bad(protocol2.CodeBusy, "annulation non enregistrée : réessayez")
		}
		return encode(ok("")), false
	}

	// Commandes de wolagent/1, liste blanche du PC comprise.
	body := protocol.RequestBody{Cmd: req.Cmd, Delay: req.Delay, Force: req.Force, Wakes: req.Wakes, Day: req.Day}
	resp := protocol2.Response{ID: req.ID, ResponseBody: s.run(body, requester{ip: sess.ip, name: d.Name, id: d.Pin.Short()}), Role: d.Role.String()}
	return encode(resp), false
}

func inviteInfo(inv devices.Invite) protocol2.InviteInfo {
	info := protocol2.InviteInfo{ID: inv.ID, Role: inv.Role.String(), Name: inv.Name, Expires: inv.Expires}
	if inv.Device != nil {
		info.Device = inv.Device.String()
	}
	return info
}

// pair enregistre une identité inconnue avec une invitation (§6.3).
func (sess *v2Session) pair(req protocol2.Request) ([]byte, bool) {
	s, v := sess.s, sess.v
	refuse := func() ([]byte, bool) {
		s.failure(sess.ip, "code d'appairage refusé")
		if v.pairing.Fail() {
			n, err := v.registry.CancelInvites()
			if err != nil {
				s.logger.Printf("registre des appareils : %v", err)
			}
			s.logger.Printf("appairage suspendu 10 minutes après plus de %d échecs ; %d invitation(s) annulée(s)", pairingMaxFailures, n)
		}
		return encode(protocol2.Reply{ID: req.ID, Code: protocol2.CodePairingFailed, Message: msgPairingFailed}), true
	}
	if v.pairing.Suspended() {
		return refuse()
	}
	name := CleanName(req.Name)
	if name == "" {
		name = "Appareil"
	}
	d, err := v.registry.Redeem(req.Code, sess.pin, name)
	switch {
	case errors.Is(err, devices.ErrPairingFailed):
		return refuse()
	case errors.Is(err, devices.ErrAlreadyPaired):
		d, _ = v.registry.Lookup(sess.pin)
		return encode(sess.answer(req.ID, d, protocol2.CodeAlreadyPaired, "cet appareil est déjà autorisé sur ce PC")), false
	case err != nil:
		s.logger.Printf("registre des appareils : %v", err)
		return encode(protocol2.Reply{ID: req.ID, Code: protocol2.CodeBusy, Message: "appairage non enregistré : réessayez"}), true
	}
	s.noteDevice(protocol.DeviceAdded, d, devices.Device{})
	s.limiter.Success(sess.ip)
	resp := sess.answer(req.ID, d, protocol2.CodeOK, "appareil ajouté")
	resp.Device = sess.pin.String()
	return encode(resp), false
}

// migrate fait passer un appareil de wolagent/1 à wolagent/2, en deux étapes (§9).
func (sess *v2Session) migrate(req protocol2.Request) ([]byte, bool) {
	s, v := sess.s, sess.v
	refuse := func(why string) ([]byte, bool) {
		s.failure(sess.ip, "migration refusée : "+why)
		return encode(protocol2.Reply{ID: req.ID, Code: protocol2.CodeMigrationFailed, Message: msgMigrationFailed}), true
	}
	if !s.v1 || s.key == nil {
		return refuse("wolagent/1 désactivé")
	}
	if req.Proof == "" { // première étape : nonce de l'appareil, preuve de l'agent
		nc, err := protocol2.ParseNonce(req.Nonce)
		if err != nil || sess.migration != nil {
			return refuse("demande invalide")
		}
		na := protocol2.NewNonce()
		sess.migration = &migration{nc: nc, na: na, name: CleanName(req.Name)}
		pa := protocol2.AgentProof(s.key, v.identity.Pin, sess.pin, nc, na)
		return encode(protocol2.Reply{ID: req.ID, OK: true, Code: protocol2.CodeOK, Nonce: na.String(), Proof: pa.String()}), false
	}
	m := sess.migration
	sess.migration = nil
	proof, err := protocol2.ParseProof(req.Proof)
	if m == nil || err != nil || !protocol2.DeviceProof(s.key, v.identity.Pin, sess.pin, m.nc, m.na).Equal(proof) {
		return refuse("preuve invalide")
	}
	name := m.name
	if name == "" {
		name = "Appareil"
	}
	d, err := v.registry.Add(devices.Device{Pin: sess.pin, Name: name, Role: protocol2.RoleControl, Via: devices.ViaMigrate})
	switch {
	case errors.Is(err, devices.ErrAlreadyPaired):
		d, _ = v.registry.Lookup(sess.pin)
		return encode(sess.answer(req.ID, d, protocol2.CodeAlreadyPaired, "cet appareil est déjà autorisé sur ce PC")), false
	case err != nil:
		s.logger.Printf("registre des appareils : %v", err)
		return encode(protocol2.Reply{ID: req.ID, Code: protocol2.CodeBusy, Message: "migration non enregistrée : réessayez"}), true
	}
	s.noteDevice(protocol.DeviceAdded, d, devices.Device{})
	s.limiter.Success(sess.ip)
	resp := sess.answer(req.ID, d, protocol2.CodeOK, "appareil migré")
	resp.Device = sess.pin.String()
	return encode(resp), false
}

// failure compte un échec de l'adresse (§3.3) et le note au journal technique.
func (s *Server) failure(ip, why string) {
	if s.limiter.Fail(ip) {
		s.logger.Printf("%s bloqué 5 minutes après plusieurs échecs (%s)", ip, why)
		return
	}
	s.logger.Printf("%s : %s", ip, why)
}

// noteDevice note un changement des appareils autorisés au journal du PC et au journal technique.
func (s *Server) noteDevice(action string, d, by devices.Device) {
	label := map[string]string{
		protocol.DeviceAdded: "ajouté", protocol.DeviceRevoked: "retiré", protocol.DeviceRole: "change de rôle",
		protocol.DeviceRenamed: "renommé",
	}[action]
	how := ""
	switch {
	case by.Name != "":
		how = " par " + by.Name
	case d.Via == devices.ViaMigrate:
		how = " (migration depuis wolagent/1)"
	case d.Via == devices.ViaPair:
		how = " (code d'appairage)"
	}
	s.logger.Printf("appareil « %s » [%s] %s, rôle %s%s", d.Name, d.Pin.Short(), label, d.Role, how)
	if s.history == nil {
		return
	}
	event := protocol.HistoryEvent{T: s.now().Unix(), K: protocol.HistoryDevice, A: action, B: d.Name, I: d.Pin.Short(), X: d.Role.String()}
	if err := s.history.Add(event); err != nil {
		s.logger.Printf("journal : %v", err)
	}
}

func encode(v any) []byte {
	raw, _ := json.Marshal(v)
	return raw
}

// Suspension de l'appairage (§3.3) : plus de 20 codes refusés en 10 minutes, toutes adresses.
const (
	pairingMaxFailures = 20
	pairingWindow      = 10 * time.Minute
	pairingPause       = 10 * time.Minute
)

type pairingGuard struct {
	mu       sync.Mutex
	failures []time.Time
	until    time.Time
	now      func() time.Time
}

func newPairingGuard(now func() time.Time) *pairingGuard { return &pairingGuard{now: now} }

// Suspended indique si l'appairage est suspendu.
func (g *pairingGuard) Suspended() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.now().Before(g.until)
}

// Fail note un code refusé ; vrai si l'appairage vient d'être suspendu.
func (g *pairingGuard) Fail() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	recent := g.failures[:0]
	for _, t := range g.failures {
		if now.Sub(t) < pairingWindow {
			recent = append(recent, t)
		}
	}
	g.failures = append(recent, now)
	if len(g.failures) > pairingMaxFailures {
		g.failures = nil
		g.until = now.Add(pairingPause)
		return true
	}
	return false
}

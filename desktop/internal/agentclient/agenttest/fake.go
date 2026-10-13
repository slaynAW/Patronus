// Package agenttest fournit un faux agent pour les tests (comportements normaux et défaillants), en
// wolagent/1 et, sur demande, en wolagent/2.
package agenttest

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"io"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol"
	"github.com/slaynaw/wakeonlan/agent/protocol2"
)

// Behavior choisit le comportement du faux agent.
type Behavior int

const (
	Normal Behavior = iota
	BadResponseMAC
	Reject
	WrongProto
	Hang
	RateLimited
)

// Server est un faux agent qui écoute sur 127.0.0.1.
type Server struct {
	key      []byte
	behavior Behavior
	listener net.Listener
	mu       sync.Mutex
	history  *protocol.History
	temps    *protocol.Temperatures
	disks    *protocol.Disks
	specs    *protocol.Specs
	metrics  map[string][]protocol.MetricsRow
	commands []string
	requests []protocol.RequestBody
	// attempts compte les demandes reçues (requête wolagent/1, session wolagent/2), acceptées ou non ;
	// une simple connexion TCP n'en est pas une.
	attempts int
	v2       *v2State
	wg       sync.WaitGroup
}

// v2State est l'état wolagent/2 du faux agent : son identité et les appareils autorisés.
type v2State struct {
	cert    tls.Certificate
	pin     protocol2.Pin
	devices map[protocol2.Pin]string // rôle
	names   map[protocol2.Pin]string
	order   []protocol2.Pin                 // ordre d'ajout, pour la liste des appareils
	invites map[string]protocol2.InviteInfo // par code normalisé
}

// Start démarre un faux agent.
func Start(key []byte, behavior Behavior) (*Server, error) {
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &Server{key: key, behavior: behavior, listener: l}
	s.wg.Add(1)
	go s.serve()
	return s, nil
}

// Port renvoie le port d'écoute.
func (s *Server) Port() int { return s.listener.Addr().(*net.TCPAddr).Port }

// SetHistory définit le journal renvoyé par « history » (nil : agent ancien, commande inconnue).
func (s *Server) SetHistory(h *protocol.History) {
	s.mu.Lock()
	s.history = h
	s.mu.Unlock()
}

// SetTemperatures définit les températures jointes à « status » (nil : agent antérieur à 1.5.0).
func (s *Server) SetTemperatures(t *protocol.Temperatures) {
	s.mu.Lock()
	s.temps = t
	s.mu.Unlock()
}

// SetDisks définit les disques joints à « status » (nil : agent antérieur à 1.9.0).
func (s *Server) SetDisks(d *protocol.Disks) {
	s.mu.Lock()
	s.disks = d
	s.mu.Unlock()
}

// SetSpecs définit la fiche renvoyée par « specs » (nil : agent antérieur à 1.10.0).
func (s *Server) SetSpecs(sp *protocol.Specs) {
	s.mu.Lock()
	s.specs = sp
	s.mu.Unlock()
}

// SetMetrics définit les mesures enregistrées, par jour (nil : agent antérieur à 1.8.0).
func (s *Server) SetMetrics(days map[string][]protocol.MetricsRow) {
	s.mu.Lock()
	s.metrics = days
	s.mu.Unlock()
}

// EnableV2 fait aussi servir wolagent/2 au faux agent, sur le même port : il crée son identité et
// l'annonce (protocols et pin) dans ses réponses. Renvoie son empreinte.
func (s *Server) EnableV2() protocol2.Pin {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	der, err := protocol2.NewCertificate(key, time.Now())
	if err != nil {
		panic(err)
	}
	pin, _ := protocol2.PinFromCertificate(der)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.v2 = &v2State{cert: tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, pin: pin,
		devices: map[protocol2.Pin]string{}, names: map[protocol2.Pin]string{}, invites: map[string]protocol2.InviteInfo{}}
	return pin
}

// Invite crée une invitation à usage unique pour le rôle donné ; renvoie son code.
func (s *Server) Invite(role string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.invite(role, "", time.Hour, "").Code
}

// invite crée une invitation (verrou pris) ; device, s'il n'est pas vide, la réserve à cette empreinte.
func (s *Server) invite(role, name string, ttl time.Duration, device string) protocol2.InviteInfo {
	code := protocol2.NewCode()
	inv := protocol2.InviteInfo{ID: protocol2.NewCode(), Code: code, Role: role, Name: name, Expires: time.Now().Add(ttl).Unix(),
		Device: device, Agent: s.v2.pin.String()}
	s.v2.invites[code] = inv
	return inv
}

// InviteList renvoie les invitations en attente, sans leur code.
func (s *Server) InviteList() []protocol2.InviteInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []protocol2.InviteInfo{}
	for _, inv := range s.v2.invites {
		inv.Code, inv.Agent = "", ""
		out = append(out, inv)
	}
	return out
}

// register enregistre un appareil (verrou pris).
func (s *Server) register(device protocol2.Pin, role, name string) {
	if _, known := s.v2.devices[device]; !known {
		s.v2.order = append(s.v2.order, device)
	}
	s.v2.devices[device], s.v2.names[device] = role, name
}

// Register autorise un appareil wolagent/2 avec un rôle ("" : le retire).
func (s *Server) Register(device protocol2.Pin, role string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if role == "" {
		delete(s.v2.devices, device)
		return
	}
	s.register(device, role, s.v2.names[device])
}

// Invites renvoie le nombre d'invitations en attente.
func (s *Server) Invites() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.v2.invites)
}

// administer exécute une commande d'administration (§6.2) pour l'appareil self, de rôle admin.
func (s *Server) administer(self protocol2.Pin, req protocol2.Request) protocol2.Response {
	s.mu.Lock()
	defer s.mu.Unlock()
	resp := protocol2.Response{ID: req.ID, ResponseBody: protocol.ResponseBody{OK: true, Code: protocol2.CodeOK}, Role: s.v2.devices[self]}
	refuse := func(code string) protocol2.Response {
		resp.OK, resp.Code, resp.Message = false, code, "refusé : "+code
		return resp
	}
	admins := 0
	for _, role := range s.v2.devices {
		if role == "admin" {
			admins++
		}
	}
	target, _ := protocol2.ParsePin(req.Device)
	switch req.Cmd {
	case protocol2.CmdDevices:
		resp.Devices = []protocol2.DeviceInfo{}
		for _, pin := range s.v2.order {
			if role, ok := s.v2.devices[pin]; ok {
				resp.Devices = append(resp.Devices, protocol2.DeviceInfo{Device: pin.String(), Name: s.v2.names[pin], Role: role,
					Added: 1790000000, Via: "pair", Self: pin == self})
			}
		}
	case protocol2.CmdDevicesRevoke, protocol2.CmdDevicesRole, protocol2.CmdDevicesRename:
		role, known := s.v2.devices[target]
		switch {
		case !known:
			return refuse(protocol2.CodeBadRequest)
		case req.Cmd == protocol2.CmdDevicesRename:
			s.v2.names[target] = req.Name
		case role == "admin" && admins == 1 && (req.Cmd == protocol2.CmdDevicesRevoke || req.Role != "admin"):
			return refuse(protocol2.CodeLastAdmin)
		case req.Cmd == protocol2.CmdDevicesRevoke:
			delete(s.v2.devices, target)
		default:
			if _, ok := protocol2.ParseRole(req.Role); !ok {
				return refuse(protocol2.CodeBadRequest)
			}
			s.v2.devices[target] = req.Role
		}
	case protocol2.CmdInvite:
		if _, ok := protocol2.ParseRole(req.Role); !ok || req.TTL < protocol2.MinInviteTTL || req.TTL > protocol2.MaxInviteTTL {
			return refuse(protocol2.CodeBadRequest)
		}
		if _, err := protocol2.ParsePin(req.Device); req.Device != "" && err != nil {
			return refuse(protocol2.CodeBadRequest)
		}
		inv := s.invite(req.Role, req.Name, time.Duration(req.TTL)*time.Second, req.Device)
		resp.Invite = &inv
	case protocol2.CmdInvites:
		resp.Invites = []protocol2.InviteInfo{}
		for _, inv := range s.v2.invites {
			inv.Code, inv.Agent = "", ""
			resp.Invites = append(resp.Invites, inv)
		}
	case protocol2.CmdInvitesCancel:
		for code, inv := range s.v2.invites {
			if inv.ID == req.Invite {
				delete(s.v2.invites, code)
				return resp
			}
		}
		return refuse(protocol2.CodeBadRequest)
	}
	return resp
}

// Device renvoie le rôle et le nom d'un appareil wolagent/2 ("" s'il n'est pas autorisé).
func (s *Server) Device(device protocol2.Pin) (role, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.v2.devices[device], s.v2.names[device]
}

// Attempts renvoie le nombre de demandes reçues, acceptées ou non (requêtes wolagent/1, sessions
// wolagent/2).
func (s *Server) Attempts() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.attempts
}

// Commands renvoie les commandes authentifiées reçues.
func (s *Server) Commands() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.commands...)
}

// Requests renvoie les requêtes authentifiées reçues.
func (s *Server) Requests() []protocol.RequestBody {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]protocol.RequestBody(nil), s.requests...)
}

// Close arrête le faux agent.
func (s *Server) Close() {
	s.listener.Close()
	s.wg.Wait()
}

func (s *Server) serve() {
	defer s.wg.Done()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer conn.Close()
			s.handle(conn)
		}()
	}
}

func (s *Server) handle(conn net.Conn) {
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	write := func(v any) {
		b, _ := json.Marshal(v)
		_, _ = conn.Write(append(b, '\n'))
	}
	switch s.behavior {
	case Hang:
		// Muet jusqu'à ce que le client abandonne et ferme la connexion.
		_, _ = io.Copy(io.Discard, conn)
		return
	case RateLimited:
		write(protocol.Response{Error: protocol.ErrRateLimited})
		return
	}
	nonce, _ := protocol.NewNonce(protocol.ServerNonceBytes)
	proto := protocol.Proto
	if s.behavior == WrongProto {
		proto = "wolagent/99"
	}
	write(protocol.Hello{Proto: proto, Nonce: nonce})

	reader := bufio.NewReader(conn)
	s.mu.Lock()
	v2 := s.v2 != nil
	s.mu.Unlock()
	if first, err := reader.Peek(1); err == nil && first[0] == protocol2.TLSHandshakeRecord && v2 {
		s.serveV2(conn, reader)
		return
	}
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return
	}
	s.mu.Lock()
	s.attempts++
	s.mu.Unlock()
	var req protocol.Request
	if json.Unmarshal(line, &req) != nil || !protocol.MACEqual(protocol.RequestMAC(s.key, nonce, req.CNonce, req.Body), req.Mac) {
		write(protocol.Response{Error: protocol.ErrUnauthorized})
		return
	}
	var body protocol.RequestBody
	_ = json.Unmarshal([]byte(req.Body), &body)
	b, _ := json.Marshal(s.answer(body))
	mac := protocol.ResponseMAC(s.key, nonce, req.CNonce, string(b))
	if s.behavior == BadResponseMAC {
		mac = protocol.ResponseMAC(make([]byte, protocol.KeyBytes), nonce, req.CNonce, string(b))
	}
	write(protocol.Response{Body: string(b), Mac: mac})
}

// answer exécute une commande de wolagent/1, reçue par l'un ou l'autre protocole.
func (s *Server) answer(body protocol.RequestBody) protocol.ResponseBody {
	s.mu.Lock()
	s.commands = append(s.commands, body.Cmd)
	s.requests = append(s.requests, body)
	s.mu.Unlock()

	resp := protocol.ResponseBody{OK: true, Code: "ok", Message: "OK", Hostname: "PC-TEST", OS: "windows", Arch: "amd64",
		Version: "1.0.0", Uptime: 3600, Protocols: []string{protocol.Proto}}
	s.mu.Lock()
	if s.v2 != nil {
		resp.Protocols = append(resp.Protocols, protocol2.ALPN)
		resp.Pin = s.v2.pin.String()
	}
	s.mu.Unlock()
	if body.Cmd == protocol.CmdStatus {
		s.mu.Lock()
		resp.Temperatures = s.temps
		resp.Disks = s.disks
		s.mu.Unlock()
	}
	if body.Cmd == protocol.CmdHistory || body.Cmd == protocol.CmdWakes {
		s.mu.Lock()
		if s.history != nil && body.Cmd == protocol.CmdWakes {
			// Agent 1.4.0 : démarrages signalés ajoutés au journal (sans le tri ni les contrôles du vrai).
			h := *s.history
			h.Events = append([]protocol.HistoryEvent(nil), h.Events...)
			for _, t := range body.Wakes {
				h.Events = append(h.Events, protocol.HistoryEvent{T: t, K: protocol.HistoryWake, C: "127.0.0.1", B: body.By})
			}
			s.history = &h
		}
		h := s.history
		s.mu.Unlock()
		if h == nil {
			resp = protocol.ResponseBody{OK: false, Code: "unsupported", Message: "commande inconnue : " + body.Cmd}
		} else {
			resp.History = h
		}
	}
	if body.Cmd == protocol.CmdSpecs {
		s.mu.Lock()
		if s.specs == nil {
			resp = protocol.ResponseBody{OK: false, Code: "forbidden", Message: "commande « specs » désactivée sur ce PC"}
		} else {
			sp := *s.specs
			resp.Specs = &sp
		}
		s.mu.Unlock()
	}
	if body.Cmd == protocol.CmdMetrics {
		s.mu.Lock()
		if s.metrics == nil {
			resp = protocol.ResponseBody{OK: false, Code: "forbidden", Message: "commande « metrics » désactivée sur ce PC"}
		} else {
			m := protocol.Metrics{Day: body.Day, Days: []string{}, Rows: []protocol.MetricsRow{}}
			for day := range s.metrics {
				m.Days = append(m.Days, day)
			}
			sort.Strings(m.Days)
			if body.Day != "" && s.metrics[body.Day] != nil {
				m.Rows = s.metrics[body.Day]
			}
			resp.Metrics = &m
		}
		s.mu.Unlock()
	}
	if s.behavior == Reject {
		resp = protocol.ResponseBody{OK: false, Code: "forbidden", Message: "commande désactivée"}
	}
	return resp
}

// readerConn rend à TLS ce que la lecture du premier octet a déjà mis en mémoire.
type readerConn struct {
	net.Conn
	r *bufio.Reader
}

func (c readerConn) Read(p []byte) (int, error) { return c.r.Read(p) }

// serveV2 mène une session wolagent/2 : TLS 1.3 avec l'identité du faux agent, puis requêtes en
// trames. Comme le vrai agent : une identité inconnue n'a droit qu'à la migration (preuves par la
// clé) et à l'appairage (code à usage unique), un rôle trop faible est refusé.
func (s *Server) serveV2(conn net.Conn, r *bufio.Reader) {
	s.mu.Lock()
	cert, agent := s.v2.cert, s.v2.pin
	s.mu.Unlock()
	tc := tls.Server(readerConn{conn, r}, &tls.Config{
		MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13,
		Certificates: []tls.Certificate{cert}, NextProtos: []string{protocol2.ALPN},
		ClientAuth: tls.RequireAnyClientCert, SessionTicketsDisabled: true,
	})
	if err := tc.Handshake(); err != nil || tc.ConnectionState().NegotiatedProtocol != protocol2.ALPN {
		return
	}
	s.mu.Lock()
	s.attempts++
	s.mu.Unlock()
	device, err := protocol2.PinFromCertificate(tc.ConnectionState().PeerCertificates[0].Raw)
	if err != nil {
		return
	}
	reply := func(v any) {
		b, _ := json.Marshal(v)
		_ = protocol2.WriteFrame(tc, b, protocol2.MaxResponseBytes)
	}
	var nc, na *protocol2.Nonce
	var name string
	for {
		payload, err := protocol2.ReadFrame(tc, protocol2.MaxRequestBytes)
		if err != nil {
			return
		}
		var req protocol2.Request
		if json.Unmarshal(payload, &req) != nil {
			return
		}
		role, _ := s.Device(device)
		switch {
		case req.Cmd == protocol2.CmdPair && role != "":
			reply(protocol2.Response{ID: req.ID, ResponseBody: protocol.ResponseBody{Code: protocol2.CodeAlreadyPaired,
				Message: "cet appareil est déjà autorisé sur ce PC"}, Role: role})
		case req.Cmd == protocol2.CmdPair:
			code, err := protocol2.NormalizeCode(req.Code)
			s.mu.Lock()
			invited, ok := s.v2.invites[code]
			// Une invitation réservée ne sert qu'à l'empreinte prévue (§6.2).
			ok = ok && (invited.Device == "" || invited.Device == device.String())
			if err == nil && ok {
				delete(s.v2.invites, code)
				s.register(device, invited.Role, req.Name)
			}
			s.mu.Unlock()
			if err != nil || !ok {
				reply(protocol2.Reply{ID: req.ID, Code: protocol2.CodePairingFailed, Message: "code d'appairage refusé"})
				return
			}
			reply(protocol2.Response{ID: req.ID, ResponseBody: protocol.ResponseBody{OK: true, Code: protocol2.CodeOK,
				Message: "appareil ajouté"}, Role: invited.Role, Device: device.String()})
		case req.Cmd == protocol2.CmdMigrate && role != "":
			reply(protocol2.Reply{ID: req.ID, Code: protocol2.CodeAlreadyPaired, Message: "appareil déjà appairé"})
		case req.Cmd == protocol2.CmdMigrate && nc == nil:
			client, err := protocol2.ParseNonce(req.Nonce)
			if err != nil {
				return
			}
			agentNonce := protocol2.NewNonce()
			nc, na, name = &client, &agentNonce, req.Name
			proof := protocol2.AgentProof(s.key, agent, device, client, agentNonce)
			reply(protocol2.Reply{ID: req.ID, OK: true, Code: protocol2.CodeOK, Message: "preuve de l'agent",
				Nonce: agentNonce.String(), Proof: proof.String()})
		case req.Cmd == protocol2.CmdMigrate:
			proof, err := protocol2.ParseProof(req.Proof)
			if err != nil || !protocol2.DeviceProof(s.key, agent, device, *nc, *na).Equal(proof) {
				reply(protocol2.Reply{ID: req.ID, Code: protocol2.CodeMigrationFailed, Message: "migration impossible"})
				return
			}
			s.mu.Lock()
			s.register(device, "control", name)
			s.mu.Unlock()
			reply(protocol2.Response{ID: req.ID, ResponseBody: protocol.ResponseBody{OK: true, Code: protocol2.CodeOK,
				Message: "appareil migré"}, Role: "control", Device: device.String()})
		case role == "":
			reply(protocol2.Reply{ID: req.ID, Code: protocol2.CodeUnknownDevice, Message: "appareil inconnu de ce PC"})
			return
		default:
			have, _ := protocol2.ParseRole(role)
			if need, ok := protocol2.MinRole(req.Cmd); ok && have < need {
				reply(protocol2.Reply{ID: req.ID, Code: protocol2.CodeInsufficientRole, Message: "rôle insuffisant"})
				continue
			}
			if need, _ := protocol2.MinRole(req.Cmd); need == protocol2.RoleAdmin {
				reply(s.administer(device, req))
				continue
			}
			_, by := s.Device(device)
			body := s.answer(protocol.RequestBody{Cmd: req.Cmd, Delay: req.Delay, Force: req.Force, Wakes: req.Wakes,
				Day: req.Day, By: by})
			reply(protocol2.Response{ID: req.ID, ResponseBody: body, Role: role})
		}
	}
}

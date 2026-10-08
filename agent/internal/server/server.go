// Package server implémente le serveur TCP de l'agent (protocole « wolagent/1 »).
package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/slaynaw/wakeonlan/agent/internal/config"
	"github.com/slaynaw/wakeonlan/agent/internal/history"
	"github.com/slaynaw/wakeonlan/agent/internal/power"
	"github.com/slaynaw/wakeonlan/agent/internal/sysinfo"
	"github.com/slaynaw/wakeonlan/agent/protocol"
)

const (
	connectionTimeout = 10 * time.Second
	maxConnections    = 16
	maxDelaySeconds   = 3600
)

// minActionDelay laisse le temps à la réponse d'arriver au téléphone avant l'extinction.
var minActionDelay = 1500 * time.Millisecond

// Server répond aux demandes de l'application.
type Server struct {
	cfg      *config.Config
	key      []byte
	networks []*net.IPNet
	power    power.Controller
	version  string
	logger   *log.Logger
	limiter  *rateLimiter
	slots    chan struct{}
	info     func() sysinfo.Info
	history  *history.Log
	temps    func() *protocol.Temperatures
	disks    func() *protocol.Disks
	specs    func() *protocol.Specs
	metrics  Metrics
	now      func() time.Time

	mu      sync.Mutex
	pending *time.Timer // action d'alimentation programmée
}

// New prépare un serveur à partir d'une configuration validée.
func New(cfg *config.Config, controller power.Controller, version string, logger *log.Logger) (*Server, error) {
	key, err := cfg.KeyBytes()
	if err != nil {
		return nil, err
	}
	networks, err := cfg.Networks()
	if err != nil {
		return nil, err
	}
	return &Server{
		cfg:      cfg,
		key:      key,
		networks: networks,
		power:    controller,
		version:  version,
		logger:   logger,
		limiter:  newRateLimiter(5, time.Minute, 5*time.Minute),
		slots:    make(chan struct{}, maxConnections),
		info:     sysinfo.Current,
		now:      time.Now,
	}, nil
}

// SetHistory branche le journal du PC (commande « history » et enregistrement des commandes reçues).
func (s *Server) SetHistory(l *history.Log) { s.history = l }

// SetTemperatures branche la lecture des températures, jointes aux réponses à « status » (elle doit
// répondre tout de suite : voir sensors.Cache).
func (s *Server) SetTemperatures(read func() *protocol.Temperatures) { s.temps = read }

// SetDisks branche la lecture des disques, jointe aux réponses à « status » (elle doit répondre tout
// de suite : voir disks.Reader).
func (s *Server) SetDisks(read func() *protocol.Disks) { s.disks = read }

// SetSpecs branche la fiche du PC (commande « specs » ; voir specs.Cache).
func (s *Server) SetSpecs(read func() *protocol.Specs) { s.specs = read }

// Metrics donne les mesures enregistrées en continu (voir metrics.Recorder).
type Metrics interface {
	Days() []string
	Day(day string) ([]protocol.MetricsRow, error)
}

// SetMetrics branche les mesures enregistrées (commande « metrics »).
func (s *Server) SetMetrics(m Metrics) { s.metrics = m }

// ListenAndServe écoute sur le port configuré jusqu'à l'annulation de ctx.
func (s *Server) ListenAndServe(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.cfg.ListenAddr())
	if err != nil {
		return fmt.Errorf("impossible d'écouter sur le port %d : %w", s.cfg.Port, err)
	}
	s.logger.Printf("agent %s à l'écoute sur le port TCP %d", s.version, s.cfg.Port)
	return s.Serve(ctx, ln)
}

// Serve accepte les connexions sur ln jusqu'à l'annulation de ctx.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
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

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()
	remote, _ := conn.RemoteAddr().(*net.TCPAddr)
	if remote == nil || !s.allowed(remote.IP) {
		if remote != nil {
			s.logger.Printf("connexion refusée depuis %s (réseau non autorisé)", remote.IP)
		}
		return
	}
	ip := remote.IP.String()
	_ = conn.SetDeadline(time.Now().Add(connectionTimeout))
	writer := bufio.NewWriter(conn)
	send := func(v any) {
		raw, _ := json.Marshal(v)
		_, _ = writer.Write(append(raw, '\n'))
		_ = writer.Flush()
	}

	if s.limiter.Blocked(ip) {
		send(protocol.Response{Error: protocol.ErrRateLimited})
		return
	}

	nonce, err := protocol.NewNonce(protocol.ServerNonceBytes)
	if err != nil {
		return
	}
	send(protocol.Hello{Proto: protocol.Proto, Nonce: nonce})

	reader := bufio.NewReaderSize(conn, protocol.MaxLineBytes)
	line, err := reader.ReadSlice('\n')
	if err != nil {
		return // délai dépassé, message trop long ou connexion fermée (simple sonde TCP)
	}
	var req protocol.Request
	if json.Unmarshal(line, &req) != nil || protocol.DecodedLen(req.CNonce) != protocol.ClientNonceBytes {
		send(protocol.Response{Error: protocol.ErrBadRequest})
		return
	}
	if !protocol.MACEqual(protocol.RequestMAC(s.key, nonce, req.CNonce, req.Body), req.Mac) {
		if s.limiter.Fail(ip) {
			s.logger.Printf("%s bloqué 5 minutes après plusieurs échecs d'authentification", ip)
		} else {
			s.logger.Printf("authentification refusée pour %s", ip)
		}
		send(protocol.Response{Error: protocol.ErrUnauthorized})
		return
	}
	s.limiter.Success(ip)

	body := s.execute(req.Body, ip)
	raw, _ := json.Marshal(body)
	send(protocol.Response{Body: string(raw), Mac: protocol.ResponseMAC(s.key, nonce, req.CNonce, string(raw))})
}

// execute traite une requête authentifiée.
func (s *Server) execute(rawBody, ip string) protocol.ResponseBody {
	info := s.info()
	resp := protocol.ResponseBody{
		Hostname: info.Hostname,
		OS:       info.OS,
		Arch:     info.Arch,
		Version:  s.version,
		Uptime:   int64(info.Uptime.Seconds()),
	}
	fail := func(code, message string) protocol.ResponseBody {
		resp.Code, resp.Message = code, message
		return resp
	}

	var body protocol.RequestBody
	if err := json.Unmarshal([]byte(rawBody), &body); err != nil {
		return fail("bad_request", "requête illisible")
	}
	by := CleanName(body.By)
	// Démarrages demandés par l'application : simple ajout au journal, comme sa lecture.
	if body.Cmd == protocol.CmdWakes {
		if !s.cfg.Allows(protocol.CmdStatus) && !s.cfg.Allows(protocol.CmdHistory) {
			return fail("forbidden", "commande « wakes » désactivée sur ce PC")
		}
		if s.history == nil {
			return fail("unsupported", "journal indisponible sur ce PC")
		}
		if len(body.Wakes) == 0 || len(body.Wakes) > protocol.MaxWakes {
			return fail("bad_request", "liste de démarrages invalide")
		}
		added, err := s.history.AddWakes(body.Wakes, ip, by)
		if err != nil {
			s.logger.Printf("journal : %v", err)
		}
		if added > 0 {
			s.logger.Printf("%d démarrage(s) demandé(s) par %s noté(s) au journal", added, describeClient(by, ip))
		}
		snapshot := s.history.Snapshot()
		resp.History = &snapshot
		resp.OK, resp.Code = true, "ok"
		return resp
	}
	// Le journal est en lecture seule, comme l'état : autorisé dès que « status » l'est.
	if body.Cmd == protocol.CmdHistory {
		if !s.cfg.Allows(protocol.CmdStatus) && !s.cfg.Allows(protocol.CmdHistory) {
			return fail("forbidden", "commande « history » désactivée sur ce PC")
		}
		if s.history == nil {
			return fail("unsupported", "journal indisponible sur ce PC")
		}
		snapshot := s.history.Snapshot()
		resp.History = &snapshot
		resp.OK, resp.Code = true, "ok"
		return resp
	}
	// Les mesures sont en lecture seule, comme l'état : autorisées dès que « status » l'est.
	if body.Cmd == protocol.CmdMetrics {
		if !s.cfg.Allows(protocol.CmdStatus) && !s.cfg.Allows(protocol.CmdHistory) {
			return fail("forbidden", "commande « metrics » désactivée sur ce PC")
		}
		if s.metrics == nil {
			return fail("unsupported", "mesures non enregistrées sur ce PC")
		}
		m := protocol.Metrics{Day: body.Day, Days: s.metrics.Days(), Rows: []protocol.MetricsRow{}}
		if body.Day != "" {
			rows, err := s.metrics.Day(body.Day)
			if err != nil {
				return fail("bad_request", "jour invalide ou illisible")
			}
			m.Rows = rows
		}
		resp.Metrics = &m
		resp.OK, resp.Code = true, "ok"
		return resp
	}
	// La fiche est en lecture seule, comme l'état : autorisée dès que « status » l'est.
	if body.Cmd == protocol.CmdSpecs {
		if !s.cfg.Allows(protocol.CmdStatus) && !s.cfg.Allows(protocol.CmdHistory) {
			return fail("forbidden", "commande « specs » désactivée sur ce PC")
		}
		if s.specs == nil {
			return fail("unsupported", "fiche indisponible sur ce PC")
		}
		specs := s.specs()
		if specs == nil {
			return fail("busy", "fiche en cours de lecture, réessayez dans un instant")
		}
		resp.Specs = specs
		resp.OK, resp.Code = true, "ok"
		return resp
	}
	if !s.cfg.Allows(body.Cmd) {
		return fail("forbidden", fmt.Sprintf("commande « %s » désactivée sur ce PC", body.Cmd))
	}
	if body.Cmd == "status" {
		resp.OK, resp.Code = true, "ok"
		if s.temps != nil {
			resp.Temperatures = s.temps()
		}
		if s.disks != nil {
			resp.Disks = s.disks()
		}
		return resp
	}
	action, ok := power.Parse(body.Cmd)
	if !ok {
		return fail("unsupported", fmt.Sprintf("commande inconnue : %s", body.Cmd))
	}
	delay := 0
	if body.Delay != nil {
		delay = *body.Delay
	}
	if delay < 0 || delay > maxDelaySeconds {
		return fail("bad_request", "délai invalide")
	}
	force := body.Force != nil && *body.Force

	s.schedule(action, force, time.Duration(delay)*time.Second)
	s.logger.Printf("%s demandée par %s (délai %d s, forcer=%v)", action.Label(), describeClient(by, ip), delay, force)
	if s.history != nil {
		event := protocol.HistoryEvent{T: s.now().Unix(), K: protocol.HistoryCommand, A: string(action), C: ip, B: by}
		if err := s.history.Add(event); err != nil {
			s.logger.Printf("journal : %v", err)
		}
	}
	resp.OK, resp.Code = true, "ok"
	resp.Message = fmt.Sprintf("%s dans %d s", action.Label(), delay)
	return resp
}

// CleanName ne garde du nom indiqué par l'application que des caractères affichables, sans espaces
// superflus, dans la limite de protocol.MaxByLength caractères.
func CleanName(name string) string {
	var b strings.Builder
	n := 0
	for _, r := range strings.Join(strings.Fields(name), " ") {
		if !unicode.IsPrint(r) {
			continue
		}
		if n == protocol.MaxByLength {
			break
		}
		b.WriteRune(r)
		n++
	}
	return strings.TrimSpace(b.String())
}

func describeClient(by, ip string) string {
	if by == "" {
		return ip
	}
	return by + " (" + ip + ")"
}

// schedule programme l'action ; une nouvelle demande remplace la précédente.
func (s *Server) schedule(action power.Action, force bool, delay time.Duration) {
	if delay < minActionDelay {
		delay = minActionDelay
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending != nil {
		s.pending.Stop()
	}
	s.pending = time.AfterFunc(delay, func() {
		if err := s.power.Do(action, force); err != nil {
			s.logger.Printf("échec de l'action %s : %v", action.Label(), err)
		}
	})
}

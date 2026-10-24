// Package monitor relève l'état des PC confiés au hub et des appareils sans agent (docs/HUB.md, §7) :
// « status » de chaque agent toutes les 15 s, sur une session gardée ouverte, et sonde de chaque appareil
// toutes les 30 s. Les refus d'un agent espacent ses relevés, comme le font les applications
// (docs/PROTOCOLE-V2.md, §3.3).
package monitor

import (
	"context"
	"crypto/tls"
	"errors"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/slaynaw/wakeonlan/agent/hubprotocol"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/agents"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/archive"
	"github.com/slaynaw/wakeonlan/agent/internal/hub/fleet"
	"github.com/slaynaw/wakeonlan/agent/internal/netinfo"
	"github.com/slaynaw/wakeonlan/agent/protocol"
	"github.com/slaynaw/wakeonlan/agent/protocol2"
	"github.com/slaynaw/wakeonlan/agent/protocol2/client"
)

// Rythme des relevés (§7.1, §7.2).
const (
	AgentEvery = 15 * time.Second
	ProbeEvery = 30 * time.Second
	// Timeout borne un relevé (connexion et réponse).
	Timeout = 5 * time.Second
	// parallel borne les relevés simultanés.
	parallel = 8
	// Délais après un refus : blocage de l'adresse par l'agent, puis hub retiré ou empreinte différente
	// (doublé à chaque refus, 30 minutes au plus).
	blockedPause = 5 * time.Minute
	refusedFirst = time.Minute
	refusedMax   = 30 * time.Minute
	// Archive (§7.1) : journal de l'agent toutes les 10 minutes et à chaque retour en ligne ; liste des
	// jours de mesures toutes les heures, puis les jours manquants, quelques-uns par relevé.
	HistoryEvery = 10 * time.Minute
	MetricsEvery = time.Hour
	daysPerCheck = 5
)

// Monitor relève les PC de la liste, en continu.
type Monitor struct {
	fleet   *fleet.Fleet
	dialer  *agents.Dialer
	archive *archive.Archive
	logger  *log.Logger
	now     func() time.Time

	// Rythmes et délai, réglables pour les tests.
	agentEvery, probeEvery, timeout, historyEvery, metricsEvery time.Duration

	mu      sync.Mutex
	entries map[string]*entry
	slots   chan struct{}
}

// entry est l'état d'un PC et sa session avec l'agent.
type entry struct {
	state   hubprotocol.PCState
	target  string // adresse relevée (hôte:port) : une adresse changée ferme la session
	sess    *client.Session
	next    time.Time
	refused time.Duration // délai après le dernier refus de l'agent
	busy    bool
	// Archive : derniers relevés du journal et de la liste des jours de mesures, jours encore à lire.
	history, metrics time.Time
	missing          []string
}

// New prépare la surveillance des PC de list ; arch garde leur journal et leurs mesures.
func New(list *fleet.Fleet, dialer *agents.Dialer, arch *archive.Archive, logger *log.Logger) *Monitor {
	return &Monitor{
		fleet: list, dialer: dialer, archive: arch, logger: logger, now: time.Now,
		agentEvery: AgentEvery, probeEvery: ProbeEvery, timeout: Timeout, historyEvery: HistoryEvery, metricsEvery: MetricsEvery,
		entries: map[string]*entry{}, slots: make(chan struct{}, parallel),
	}
}

// Run relève les PC jusqu'à l'annulation de ctx.
func (m *Monitor) Run(ctx context.Context) {
	tick := time.NewTicker(time.Second / 4)
	defer tick.Stop()
	for {
		m.tick(ctx)
		select {
		case <-ctx.Done():
			m.closeAll()
			return
		case <-tick.C:
		}
	}
}

// States renvoie l'état de chaque PC, dans l'ordre de la liste.
func (m *Monitor) States() []hubprotocol.PCState {
	pcs := m.fleet.List()
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]hubprotocol.PCState, 0, len(pcs))
	for _, p := range pcs {
		if e, ok := m.entries[p.ID]; ok {
			out = append(out, e.state)
		} else {
			out = append(out, hubprotocol.PCState{ID: p.ID, State: hubprotocol.StateUnknown})
		}
	}
	return out
}

// tick lance les relevés dus, et oublie les PC retirés de la liste.
func (m *Monitor) tick(ctx context.Context) {
	pcs := m.fleet.List()
	now := m.now()
	m.mu.Lock()
	defer m.mu.Unlock()
	present := map[string]bool{}
	for _, p := range pcs {
		present[p.ID] = true
		e, ok := m.entries[p.ID]
		if !ok {
			e = &entry{state: hubprotocol.PCState{ID: p.ID, State: hubprotocol.StateUnknown}}
			m.entries[p.ID] = e
		}
		if e.busy || now.Before(e.next) {
			continue
		}
		select {
		case m.slots <- struct{}{}:
		default:
			return // assez de relevés en cours : les autres attendent le prochain tour
		}
		e.busy = true
		go func(p fleet.PC) {
			defer func() { <-m.slots }()
			m.check(ctx, p)
		}(p)
	}
	for id, e := range m.entries {
		if !present[id] && !e.busy {
			if e.sess != nil {
				_ = e.sess.Close()
			}
			delete(m.entries, id)
		}
	}
}

func (m *Monitor) closeAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.entries {
		if e.sess != nil {
			_ = e.sess.Close()
			e.sess = nil
		}
	}
}

// result est le résultat d'un relevé.
type result struct {
	state   string
	problem string
	latency time.Duration
	agent   *protocol.ResponseBody // réponse de l'agent à « status »
	pause   time.Duration          // délai avant le prochain relevé, s'il diffère du rythme normal
	refused bool                   // refus de l'agent : délai doublé
}

func (m *Monitor) check(ctx context.Context, p fleet.PC) {
	var r result
	if p.Agent != nil {
		r = m.checkAgent(ctx, p)
	} else {
		r = m.checkProbe(ctx, p)
	}
	now := m.now()
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.entries[p.ID]
	if e == nil {
		return // retiré pendant le relevé
	}
	e.busy = false
	if e.state.State != r.state {
		e.state.Since = now.Unix()
	}
	e.state.State, e.state.Problem, e.state.Checked = r.state, r.problem, now.Unix()
	if r.latency > 0 {
		e.state.Latency = r.latency.Milliseconds()
	}
	switch {
	case r.agent != nil:
		e.state.Agent = r.agent
	case r.state != hubprotocol.StateOnline:
		e.state.Agent = nil
	}
	every := m.probeEvery
	if p.Agent != nil {
		every = m.agentEvery
	}
	switch {
	case r.refused:
		if e.refused = 2 * e.refused; e.refused < refusedFirst {
			e.refused = refusedFirst
		}
		e.refused = min(e.refused, refusedMax)
		every = e.refused
	case r.pause > 0:
		e.refused, every = 0, r.pause
	default:
		e.refused = 0
	}
	e.next = now.Add(every)
}

// session renvoie la session ouverte avec l'agent, ou en ouvre une ; fresh indique une nouvelle session.
func (m *Monitor) session(ctx context.Context, p fleet.PC) (sess *client.Session, fresh bool, err error) {
	target := net.JoinHostPort(p.Host, strconv.Itoa(p.Port))
	m.mu.Lock()
	e := m.entries[p.ID]
	if e.sess != nil && e.target == target {
		sess = e.sess
	} else if e.sess != nil {
		_ = e.sess.Close()
		e.sess = nil
	}
	m.mu.Unlock()
	if sess != nil {
		return sess, false, nil
	}
	sess, err = m.dialer.Dial(ctx, p.Host, p.Port, *p.Agent)
	if err != nil {
		return nil, false, err
	}
	m.mu.Lock()
	e.sess, e.target = sess, target
	m.mu.Unlock()
	return sess, true, nil
}

func (m *Monitor) drop(p fleet.PC, sess *client.Session) {
	_ = sess.Close()
	m.mu.Lock()
	defer m.mu.Unlock()
	if e := m.entries[p.ID]; e != nil && e.sess == sess {
		e.sess = nil
	}
}

func (m *Monitor) checkAgent(ctx context.Context, p fleet.PC) result {
	for attempt := 0; ; attempt++ {
		ctx, cancel := context.WithTimeout(ctx, m.timeout)
		start := time.Now()
		sess, fresh, err := m.session(ctx, p)
		if err != nil {
			cancel()
			return dialFailure(err)
		}
		resp, err := sess.Do(ctx, protocol2.Request{Cmd: "status"})
		cancel()
		if err != nil {
			m.drop(p, sess)
			if !fresh && attempt == 0 {
				continue // session fermée par l'agent (inactivité, 100 requêtes) : une nouvelle, tout de suite
			}
			return result{state: hubprotocol.StateOffline}
		}
		switch resp.Code {
		case protocol2.CodeOK:
			body := resp.ResponseBody
			latency := time.Since(start)
			m.collect(ctx, p, sess)
			return result{state: hubprotocol.StateOnline, latency: latency, agent: &body}
		case protocol2.CodeUnknownDevice:
			m.drop(p, sess)
			return result{state: hubprotocol.StateOnline, problem: hubprotocol.ProblemRevoked, refused: true}
		default:
			// busy, forbidden… : le PC répond, l'état n'est pas lu cette fois.
			return result{state: hubprotocol.StateOnline, latency: time.Since(start)}
		}
	}
}

// collect garde le journal et les mesures de l'agent dans l'archive, quand c'est le moment (§7.1). Un
// échec n'empêche pas le relevé : il sera retenté au prochain.
func (m *Monitor) collect(parent context.Context, p fleet.PC, sess *client.Session) {
	if m.archive == nil {
		return
	}
	now := m.now()
	m.mu.Lock()
	e := m.entries[p.ID]
	wantHistory := e.state.State != hubprotocol.StateOnline || now.Sub(e.history) >= m.historyEvery
	wantDays := now.Sub(e.metrics) >= m.metricsEvery
	missing := e.missing
	m.mu.Unlock()

	if wantHistory {
		ctx, cancel := context.WithTimeout(parent, m.timeout)
		resp, err := sess.Do(ctx, protocol2.Request{Cmd: "history"})
		cancel()
		if err == nil && resp.OK && resp.History != nil {
			if n, err := m.archive.AddEvents(p.ID, resp.History.Events); err != nil {
				m.logger.Printf("archive de « %s » : %v", p.Name, err)
			} else {
				if n > 0 {
					m.logger.Printf("archive de « %s » : %d évènement(s) ajouté(s)", p.Name, n)
				}
				m.mu.Lock()
				e.history = now
				m.mu.Unlock()
			}
		}
	}
	if wantDays {
		ctx, cancel := context.WithTimeout(parent, m.timeout)
		resp, err := sess.Do(ctx, protocol2.Request{Cmd: "metrics"})
		cancel()
		if err != nil || !resp.OK || resp.Metrics == nil {
			return
		}
		today := now.UTC().Format("2006-01-02")
		missing = nil
		for _, day := range resp.Metrics.Days {
			if day >= today {
				continue // le jour en cours s'écrit encore
			}
			if kept, err := m.archive.HasDay(p.ID, day); err == nil && !kept {
				missing = append(missing, day)
			}
		}
		m.mu.Lock()
		e.metrics, e.missing = now, missing
		m.mu.Unlock()
	}
	for i, day := range missing {
		if i == daysPerCheck {
			break
		}
		ctx, cancel := context.WithTimeout(parent, m.timeout)
		resp, err := sess.Do(ctx, protocol2.Request{Cmd: "metrics", Day: day})
		cancel()
		if err != nil || !resp.OK || resp.Metrics == nil || resp.Metrics.Day != day {
			return
		}
		if err := m.archive.AddDay(p.ID, day, resp.Metrics.Rows); err != nil {
			m.logger.Printf("mesures de « %s » : %v", p.Name, err)
			return
		}
		m.mu.Lock()
		e.missing = e.missing[1:]
		m.mu.Unlock()
	}
}

// dialFailure classe un échec de connexion à l'agent.
func dialFailure(err error) result {
	switch {
	case errors.Is(err, client.ErrRateLimited):
		return result{state: hubprotocol.StateOnline, problem: hubprotocol.ProblemBlocked, pause: blockedPause}
	case errors.Is(err, client.ErrPinMismatch):
		return result{state: hubprotocol.StateOnline, problem: hubprotocol.ProblemPinMismatch, refused: true}
	case netinfo.ConnRefused(err):
		return result{state: hubprotocol.StateAgentDown}
	}
	return result{state: hubprotocol.StateOffline}
}

// checkProbe sonde un appareil sans agent : seule compte une réponse, jamais son contenu (§7.2).
func (m *Monitor) checkProbe(ctx context.Context, p fleet.PC) result {
	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()
	start := time.Now()
	port := p.Probe.Port
	switch p.Probe.Kind {
	case hubprotocol.ProbeTCP:
		// Le port doit accepter la connexion : un port fermé compte comme un service arrêté.
		conn, err := m.dialer.DialContext(ctx, "tcp", net.JoinHostPort(p.Host, strconv.Itoa(port)))
		if err != nil {
			return result{state: hubprotocol.StateOffline}
		}
		_ = conn.Close()
		return result{state: hubprotocol.StateOnline, latency: time.Since(start)}
	case hubprotocol.ProbeHTTP, hubprotocol.ProbeHTTPS:
		scheme := p.Probe.Kind
		if port == 0 {
			port = map[string]int{hubprotocol.ProbeHTTP: 80, hubprotocol.ProbeHTTPS: 443}[scheme]
		}
		httpClient := &http.Client{
			Transport: &http.Transport{
				DialContext: m.dialer.DialContext,
				Proxy:       nil, // jamais de mandataire : la sonde vise le réseau local
				// Le certificat d'une box ou d'une caméra est souvent autosigné : il n'est pas vérifié, car
				// rien n'est envoyé ni lu, seule la réponse compte.
				TLSClientConfig:   &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // sonde sans données
				DisableKeepAlives: true,
			},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, scheme+"://"+net.JoinHostPort(p.Host, strconv.Itoa(port))+"/", nil)
		if err != nil {
			return result{state: hubprotocol.StateOffline}
		}
		req.Header.Set("User-Agent", "Patronus-hub")
		resp, err := httpClient.Do(req)
		if err != nil {
			return result{state: hubprotocol.StateOffline}
		}
		_ = resp.Body.Close()
		return result{state: hubprotocol.StateOnline, latency: time.Since(start)}
	}
	return result{state: hubprotocol.StateUnknown}
}

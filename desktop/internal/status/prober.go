package status

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/slaynaw/wakeonlan/desktop/internal/agentclient"
	"github.com/slaynaw/wakeonlan/desktop/internal/model"
	"github.com/slaynaw/wakeonlan/desktop/internal/netdial"
)

// DefaultProbeTimeout borne la durée d'une sonde (identique à Android).
const DefaultProbeTimeout = 1500 * time.Millisecond

// Après un refus de l'agent (identité inconnue, clé refusée), il n'est recontacté qu'après un délai,
// doublé à chaque nouveau refus : chaque refus compte comme un échec, et l'agent bloque une adresse
// après 5 échecs en une minute. Une adresse bloquée l'est 5 minutes.
const (
	refusalMinDelay  = time.Minute
	refusalMaxDelay  = 30 * time.Minute
	rateLimitedPause = 5 * time.Minute
)

// Prober sonde un PC ; interface pour pouvoir le simuler dans les tests.
type Prober interface {
	Probe(ctx context.Context, d model.Device) ProbeResult
}

// ProberFunc adapte une fonction en Prober.
type ProberFunc func(ctx context.Context, d model.Device) ProbeResult

func (f ProberFunc) Probe(ctx context.Context, d model.Device) ProbeResult { return f(ctx, d) }

// HostProber détermine si une machine est allumée en combinant plusieurs méthodes, en parallèle :
//
//   - l'agent (si configuré) : échange authentifié qui donne aussi le nom et l'uptime du PC ;
//   - des connexions TCP vers des ports courants : acceptée OU refusée = machine allumée
//     (une machine éteinte ne répond pas du tout) ;
//   - un ping ICMP, souvent bloqué par le pare-feu Windows mais utile pour les autres systèmes.
//
// La première réponse positive l'emporte ; le tout est borné par Timeout.
type HostProber struct {
	Agent   *agentclient.Client
	Timeout time.Duration
	Ping    func(ctx context.Context, addr netip.Addr, timeout time.Duration) bool
	TCP     func(ctx context.Context, addr netip.Addr, port int, timeout time.Duration) bool
	Resolve func(ctx context.Context, host string) (netip.Addr, error)
	// Now est l'horloge (remplaçable pour les tests).
	Now func() time.Time

	mu sync.Mutex
	// refusals : derniers refus de l'agent, par PC (voir refusalMinDelay).
	refusals map[string]refusal
}

// refusal est un refus récent de l'agent d'un PC.
type refusal struct {
	settings [32]byte // adresse et réglages de l'agent au moment du refus
	code     agentclient.Code
	count    int
	until    time.Time
}

// NewHostProber renvoie une sonde avec les méthodes réelles.
func NewHostProber() *HostProber {
	return &HostProber{Agent: agentclient.New(), Timeout: DefaultProbeTimeout, Ping: Ping, TCP: TCPAnswers, Resolve: Resolve,
		Now: time.Now}
}

type agentOutcome struct {
	status   agentclient.Status
	err      error
	timedOut bool
}

// Probe sonde le PC.
func (p *HostProber) Probe(parent context.Context, d model.Device) ProbeResult {
	if !d.HasHost() {
		return ProbeResult{}
	}
	start := time.Now()
	ctx, cancel := context.WithTimeout(parent, p.Timeout)
	defer cancel()
	addr, err := p.Resolve(ctx, d.Host)
	if err != nil {
		return ProbeResult{}
	}

	var agentCh chan agentOutcome
	var agentError agentclient.Code
	ports := d.ProbePorts
	if d.Agent != nil && d.Agent.Ready() {
		if code, wait := p.refused(d); wait {
			// Agent qui refuse cet ordinateur : pas de nouvelle demande avant le délai, mais une simple
			// connexion à son port (qu'il ne compte pas comme un échec) dit si le PC est allumé.
			agentError = code
			ports = append(slices.Clone(ports), d.Agent.Port)
		} else {
			agentCh = make(chan agentOutcome, 1)
			settings := *d.Agent
			go func() {
				st, err := p.Agent.Status(ctx, addr.String(), settings)
				agentCh <- agentOutcome{status: st, err: err, timedOut: err != nil && ctx.Err() != nil}
			}()
		}
	}
	fallbackCh := make(chan *ProbeResult, 1)
	go func() { fallbackCh <- p.firstReachable(ctx, addr, ports, start) }()

	if agentCh != nil {
		out := <-agentCh
		if !out.timedOut {
			p.noteAgent(d, out.err)
		}
		if !out.timedOut && agentclient.HostAnswered(out.err) {
			cancel()
			latency := time.Since(start).Milliseconds()
			if out.err == nil {
				st := out.status
				return ProbeResult{Reachable: true, LatencyMs: latency, Method: MethodAgent, Agent: &st}
			}
			return ProbeResult{Reachable: true, LatencyMs: latency, Method: MethodTCP, AgentError: agentclient.CodeOf(out.err)}
		}
		if out.timedOut {
			agentError = agentclient.Unreachable
		} else {
			agentError = agentclient.CodeOf(out.err)
		}
	}
	result := ProbeResult{}
	if fb := <-fallbackCh; fb != nil {
		result = *fb
	}
	result.AgentError = agentError
	return result
}

// refused indique que l'agent d'un PC a refusé cet ordinateur il y a peu, avec les mêmes réglages :
// il n'est pas recontacté avant la fin du délai.
func (p *HostProber) refused(d model.Device) (agentclient.Code, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	r, ok := p.refusals[d.ID]
	if !ok || r.settings != agentSettingsKey(d) || !p.now().Before(r.until) {
		return "", false
	}
	return r.code, true
}

// Forget oublie le dernier refus de l'agent d'un PC : il sera recontacté à la prochaine sonde (cet
// ordinateur vient d'y être enregistré, ou une action manuelle a réussi).
func (p *HostProber) Forget(id string) {
	p.mu.Lock()
	delete(p.refusals, id)
	p.mu.Unlock()
}

// noteAgent retient un refus de l'agent (délai doublé à chaque refus), ou l'oublie après une réponse.
func (p *HostProber) noteAgent(d model.Device, err error) {
	code := agentclient.CodeOf(err)
	p.mu.Lock()
	defer p.mu.Unlock()
	switch {
	case err == nil:
		delete(p.refusals, d.ID)
		return
	case code != agentclient.Unregistered && code != agentclient.Unauthorized && code != agentclient.RateLimited:
		return
	}
	if p.refusals == nil {
		p.refusals = map[string]refusal{}
	}
	r := p.refusals[d.ID]
	if key := agentSettingsKey(d); r.settings != key {
		r = refusal{settings: key}
	}
	r.code = code
	if code == agentclient.RateLimited {
		r.until = p.now().Add(rateLimitedPause)
	} else {
		r.count++
		r.until = p.now().Add(min(refusalMaxDelay, refusalMinDelay<<min(r.count-1, 5)))
	}
	p.refusals[d.ID] = r
}

func (p *HostProber) now() time.Time {
	if p.Now == nil {
		return time.Now()
	}
	return p.Now()
}

// agentSettingsKey résume l'adresse et les réglages de l'agent d'un PC (sans garder la clé) : un
// changement (nouvelle clé, appairage, autre adresse) permet de recontacter l'agent tout de suite.
func agentSettingsKey(d model.Device) [32]byte {
	return sha256.Sum256(fmt.Appendf(nil, "%q|%d|%q|%q", d.Host, d.Agent.Port, d.Agent.Key, d.Agent.Pin))
}

func (p *HostProber) firstReachable(ctx context.Context, addr netip.Addr, ports []int, start time.Time) *ProbeResult {
	unique := map[int]bool{}
	var list []int
	for _, port := range ports {
		if !unique[port] {
			unique[port] = true
			list = append(list, port)
		}
	}
	results := make(chan Method, len(list)+1)
	for _, port := range list {
		go func() {
			if p.TCP(ctx, addr, port, p.Timeout) {
				results <- MethodTCP
			} else {
				results <- ""
			}
		}()
	}
	go func() {
		if p.Ping(ctx, addr, p.Timeout) {
			results <- MethodPing
		} else {
			results <- ""
		}
	}()
	for range len(list) + 1 {
		select {
		case m := <-results:
			if m != "" {
				return &ProbeResult{Reachable: true, LatencyMs: time.Since(start).Milliseconds(), Method: m}
			}
		case <-ctx.Done():
			return nil
		}
	}
	return nil
}

// TCPAnswers indique si la machine répond sur ce port, que la connexion soit acceptée ou refusée.
func TCPAnswers(ctx context.Context, addr netip.Addr, port int, timeout time.Duration) bool {
	conn, err := netdial.Dial(ctx, net.JoinHostPort(addr.String(), strconv.Itoa(port)), timeout)
	if err == nil {
		conn.Close()
		return true
	}
	return netdial.IsRefused(err)
}

// Resolve résout un nom d'hôte, en préférant IPv4 (comme Java / Android).
func Resolve(ctx context.Context, host string) (netip.Addr, error) {
	if addr, err := netip.ParseAddr(host); err == nil {
		return addr, nil
	}
	addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return netip.Addr{}, err
	}
	for _, a := range addrs {
		if a.Unmap().Is4() {
			return a.Unmap(), nil
		}
	}
	if len(addrs) == 0 {
		return netip.Addr{}, errors.New("aucune adresse")
	}
	return addrs[0], nil
}

package status

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strconv"
	"time"

	"github.com/slaynaw/wakeonlan/desktop/internal/agentclient"
	"github.com/slaynaw/wakeonlan/desktop/internal/model"
	"github.com/slaynaw/wakeonlan/desktop/internal/netdial"
)

// DefaultProbeTimeout borne la durée d'une sonde (identique à Android).
const DefaultProbeTimeout = 1500 * time.Millisecond

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
}

// NewHostProber renvoie une sonde avec les méthodes réelles.
func NewHostProber() *HostProber {
	return &HostProber{Agent: agentclient.New(), Timeout: DefaultProbeTimeout, Ping: Ping, TCP: TCPAnswers, Resolve: Resolve}
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
	if d.Agent != nil && d.Agent.HasKey() {
		agentCh = make(chan agentOutcome, 1)
		settings := *d.Agent
		go func() {
			st, err := p.Agent.Status(ctx, addr.String(), settings)
			agentCh <- agentOutcome{status: st, err: err, timedOut: err != nil && ctx.Err() != nil}
		}()
	}
	fallbackCh := make(chan *ProbeResult, 1)
	go func() { fallbackCh <- p.firstReachable(ctx, addr, d.ProbePorts, start) }()

	var agentError agentclient.Code
	if agentCh != nil {
		out := <-agentCh
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

// Package status détermine en temps réel si chaque PC est allumé, avec la même logique que
// l'application Android (core/status) : sondes combinées, machine à états avec hystérésis,
// états transitoires après un réveil / une extinction / un redémarrage.
package status

import "github.com/slaynaw/wakeonlan/desktop/internal/agentclient"

// PowerState est l'état affiché d'un PC.
type PowerState string

const (
	// Unknown : pas encore vérifié, ou vérification impossible (voir UnknownReason).
	Unknown PowerState = "UNKNOWN"
	Online  PowerState = "ONLINE"
	Offline PowerState = "OFFLINE"
	// Waking : paquet magique envoyé, en attente de la première réponse.
	Waking PowerState = "WAKING"
	// ShuttingDown : extinction / mise en veille demandée, en attente de la disparition.
	ShuttingDown PowerState = "SHUTTING_DOWN"
	// Restarting : redémarrage demandé, attente de la disparition puis du retour.
	Restarting PowerState = "RESTARTING"
)

// IsTransitional indique un état d'attente (réveil, extinction, redémarrage).
func (s PowerState) IsTransitional() bool { return s == Waking || s == ShuttingDown || s == Restarting }

// UnknownReason explique pourquoi l'état ne peut pas être déterminé (« inconnu » plutôt qu'un faux « éteint »).
type UnknownReason string

const (
	NoHost    UnknownReason = "NO_HOST"
	NoNetwork UnknownReason = "NO_NETWORK"
)

// Notice est un évènement notable à signaler.
type Notice string

const (
	// WakeTimeout : le PC n'a pas répondu dans le délai après l'envoi du paquet magique.
	WakeTimeout Notice = "WAKE_TIMEOUT"
	// ShutdownTimeout : le PC répond toujours longtemps après la demande d'extinction.
	ShutdownTimeout Notice = "SHUTDOWN_TIMEOUT"
)

// Method est la méthode qui a permis de détecter la machine.
type Method string

const (
	MethodAgent Method = "AGENT"
	MethodTCP   Method = "TCP"
	MethodPing  Method = "PING"
)

// DeviceStatus est l'état d'un PC, tel qu'envoyé à l'interface (horodatages en millisecondes).
type DeviceStatus struct {
	State           PowerState          `json:"state"`
	Since           int64               `json:"since"`
	LastSeen        *int64              `json:"lastSeen,omitempty"`
	LatencyMs       *int64              `json:"latencyMs,omitempty"`
	Method          Method              `json:"method,omitempty"`
	Agent           *agentclient.Status `json:"agent,omitempty"`
	AgentError      agentclient.Code    `json:"agentError,omitempty"`
	UnknownReason   UnknownReason       `json:"unknownReason,omitempty"`
	ActionStartedAt *int64              `json:"actionStartedAt,omitempty"`
	Notice          Notice              `json:"notice,omitempty"`
}

// ProbeResult est le résultat d'une sonde ponctuelle.
type ProbeResult struct {
	Reachable  bool
	LatencyMs  int64
	Method     Method
	Agent      *agentclient.Status
	AgentError agentclient.Code
}

// Tracker transforme une suite de sondes brutes en un état fiable :
//
//   - une seule réponse suffit pour passer « allumé » ;
//   - il faut OfflineThreshold échecs consécutifs pour passer « éteint » depuis « allumé »
//     (un paquet perdu sur le Wi-Fi ne fait pas clignoter l'indicateur) ;
//   - après une action, l'état transitoire est conservé jusqu'à confirmation ou expiration du délai,
//     avec une notification en cas d'échec.
//
// Pure et déterministe (horloge injectée) : entièrement testée. Non synchronisée (voir Monitor).
type Tracker struct {
	clock             func() int64
	WakeTimeoutMs     int64
	ShutdownTimeoutMs int64
	RestartTimeoutMs  int64
	OfflineThreshold  int

	status                  DeviceStatus
	failures                int
	sawOfflineDuringRestart bool
}

// NewTracker crée une machine à états avec les délais de l'application Android.
func NewTracker(clock func() int64) *Tracker {
	return &Tracker{
		clock:             clock,
		WakeTimeoutMs:     180_000,
		ShutdownTimeoutMs: 120_000,
		RestartTimeoutMs:  300_000,
		OfflineThreshold:  2,
		status:            DeviceStatus{State: Unknown, Since: clock()},
	}
}

// Status renvoie l'état courant.
func (t *Tracker) Status() DeviceStatus { return t.status }

// OnProbe intègre le résultat d'une sonde.
func (t *Tracker) OnProbe(r ProbeResult) DeviceStatus {
	now := t.clock()
	current := t.status
	base := current
	base.UnknownReason = ""
	base.AgentError = r.AgentError

	if r.Reachable {
		t.failures = 0
		seen := base
		seen.LastSeen = &now
		latency := r.LatencyMs
		seen.LatencyMs = &latency
		seen.Method = r.Method
		if r.Agent != nil {
			seen.Agent = r.Agent
		}
		switch current.State {
		case ShuttingDown:
			if t.elapsed(now) < t.ShutdownTimeoutMs {
				t.status = seen
			} else {
				t.status = enter(seen, Online, now)
				t.status.Notice = ShutdownTimeout
			}
		case Restarting:
			if !t.sawOfflineDuringRestart && t.elapsed(now) < t.RestartTimeoutMs {
				t.status = seen
			} else {
				t.status = enter(seen, Online, now)
			}
		case Online:
			t.status = seen
		default:
			t.status = enter(seen, Online, now)
		}
		return t.status
	}

	t.failures++
	switch current.State {
	case Waking:
		if t.elapsed(now) < t.WakeTimeoutMs {
			t.status = base
		} else {
			t.status = enter(base, Offline, now)
			t.status.Notice = WakeTimeout
		}
	case Restarting:
		t.sawOfflineDuringRestart = true
		if t.elapsed(now) < t.RestartTimeoutMs {
			t.status = base
		} else {
			t.status = enter(base, Offline, now)
		}
	case Online, ShuttingDown:
		if t.failures >= t.OfflineThreshold {
			t.status = enter(base, Offline, now)
		} else {
			t.status = base
		}
	case Offline:
		t.status = base
	default:
		t.status = enter(base, Offline, now)
	}
	return t.status
}

// OnUnavailable signale que la vérification est impossible (pas de réseau, pas d'adresse).
func (t *Tracker) OnUnavailable(reason UnknownReason) DeviceStatus {
	t.failures = 0
	t.status = enter(t.status, Unknown, t.clock())
	t.status.UnknownReason = reason
	t.status.LatencyMs = nil
	return t.status
}

// OnWakeSent démarre l'attente du réveil.
func (t *Tracker) OnWakeSent() DeviceStatus { return t.startAction(Waking) }

// OnShutdownSent démarre l'attente de l'extinction (ou de la mise en veille).
func (t *Tracker) OnShutdownSent() DeviceStatus { return t.startAction(ShuttingDown) }

// OnRestartSent démarre l'attente du redémarrage.
func (t *Tracker) OnRestartSent() DeviceStatus {
	t.sawOfflineDuringRestart = false
	return t.startAction(Restarting)
}

// ClearNotice efface la notification affichée.
func (t *Tracker) ClearNotice() DeviceStatus {
	t.status.Notice = ""
	return t.status
}

func (t *Tracker) startAction(state PowerState) DeviceStatus {
	now := t.clock()
	t.failures = 0
	t.status.State = state
	t.status.Since = now
	t.status.ActionStartedAt = &now
	t.status.Notice = ""
	t.status.UnknownReason = ""
	return t.status
}

func (t *Tracker) elapsed(now int64) int64 {
	if t.status.ActionStartedAt == nil {
		return 0
	}
	return now - *t.status.ActionStartedAt
}

func enter(s DeviceStatus, state PowerState, now int64) DeviceStatus {
	if s.State == state {
		return s
	}
	s.State = state
	s.Since = now
	if !state.IsTransitional() {
		s.ActionStartedAt = nil
	}
	s.Notice = ""
	return s
}

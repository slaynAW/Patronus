package status

import (
	"time"

	"github.com/slaynaw/wakeonlan/desktop/internal/agentclient"
)

// Relevés de latence récents de chaque PC, pour le tracé en direct (mêmes règles qu'Android) :
// conservés LatencyWindow en mémoire seulement.
const (
	// LatencyWindow est la durée des relevés conservés.
	LatencyWindow = 5 * time.Minute
	// LatencyMaxSamples borne le nombre de relevés conservés par PC.
	LatencyMaxSamples = 400
	// LiveInterval est l'intervalle de vérification du PC affiché en détail (tracé en direct).
	LiveInterval = time.Second
)

// LatencySample est une mesure ; Ms est nul si le PC n'a pas répondu à cette sonde.
type LatencySample struct {
	T  int64  `json:"t"`
	Ms *int64 `json:"ms"`
}

// AppendLatency ajoute une mesure en oubliant celles de plus de LatencyWindow.
func AppendLatency(samples []LatencySample, s LatencySample) []LatencySample {
	from := s.T - LatencyWindow.Milliseconds()
	out := make([]LatencySample, 0, min(len(samples)+1, LatencyMaxSamples))
	for _, x := range samples {
		if x.T >= from && x.T <= s.T {
			out = append(out, x)
		}
	}
	out = append(out, s)
	if len(out) > LatencyMaxSamples {
		out = out[len(out)-LatencyMaxSamples:]
	}
	return out
}

// Relevés de températures récents de chaque PC (agent 1.5.0 ou plus), pour le grand tracé du
// panneau de détail : conservés TempWindow en mémoire seulement.
const (
	// TempWindow est la durée des relevés conservés (et tracés).
	TempWindow = 5 * time.Minute
	// TempRepeat : un relevé identique au précédent n'est gardé qu'après ce délai. L'agent renouvelle
	// ses mesures toutes les 5 à 10 s alors que le PC affiché est sondé chaque seconde.
	TempRepeat = 10 * time.Second
	// TempMaxSamples borne le nombre de relevés conservés par PC.
	TempMaxSamples = 400
)

// TempSample est un relevé de températures (°C) ; une valeur absente n'a pas été lue.
type TempSample struct {
	T   int64    `json:"t"`
	CPU *float64 `json:"c,omitempty"`
	GPU *float64 `json:"g,omitempty"`
}

func sameValue(a, b *float64) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }

// AppendTemp ajoute un relevé en oubliant ceux de plus de TempWindow ; un relevé identique au
// précédent, pris moins de TempRepeat après lui, est ignoré.
func AppendTemp(samples []TempSample, s TempSample) []TempSample {
	if n := len(samples); n > 0 {
		last := samples[n-1]
		if sameValue(last.CPU, s.CPU) && sameValue(last.GPU, s.GPU) && s.T >= last.T && s.T-last.T < TempRepeat.Milliseconds() {
			return samples
		}
	}
	from := s.T - TempWindow.Milliseconds()
	out := make([]TempSample, 0, min(len(samples)+1, TempMaxSamples))
	for _, x := range samples {
		if x.T >= from && x.T <= s.T {
			out = append(out, x)
		}
	}
	out = append(out, s)
	if len(out) > TempMaxSamples {
		out = out[len(out)-TempMaxSamples:]
	}
	return out
}

// TempOf extrait le relevé d'une réponse de l'agent (ok faux sans aucune température). La puce
// graphique intégrée sans sonde à part (GPUShared) reprend la température du processeur : elle
// n'est pas tracée deux fois.
func TempOf(t int64, agent *agentclient.Status) (TempSample, bool) {
	if agent == nil || agent.Temperatures == nil {
		return TempSample{}, false
	}
	temps := agent.Temperatures
	s := TempSample{T: t, CPU: temps.CPU}
	if !temps.GPUShared {
		s.GPU = temps.GPU
	}
	return s, s.CPU != nil || s.GPU != nil
}

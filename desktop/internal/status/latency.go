package status

import "time"

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

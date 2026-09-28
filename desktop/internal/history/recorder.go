package history

import "github.com/slaynaw/wakeonlan/desktop/internal/status"

// Transitions déduit les évènements des changements d'état constatés entre deux relevés.
//
//   - Éteint / en démarrage / en redémarrage → allumé : « allumé ». Si l'agent répond, l'heure réelle
//     du démarrage est calculée depuis son uptime ; sinon l'heure est celle du constat (approximative).
//   - Allumé / en arrêt / en redémarrage → éteint : « éteint », daté de la dernière réponse du PC
//     (et jamais avant la demande d'extinction ou de redémarrage).
//   - Démarrage demandé sans réponse dans le délai : « pas de réponse ».
//
// Un état précédent inconnu (application qui démarre, réseau absent) ne produit rien : on ne sait
// pas ce qui s'est passé pendant ce temps (le journal de l'agent le dira).
func Transitions(prev, cur map[string]status.DeviceStatus, now int64) []Event {
	var out []Event
	for id, c := range cur {
		p, ok := prev[id]
		if !ok || p.State == c.State {
			continue
		}
		switch {
		case c.State == status.Online && (p.State == status.Offline || p.State == status.Waking || p.State == status.Restarting):
			e := Event{Device: id, Time: now, Kind: On, Source: App, Approx: true}
			if c.Agent != nil && c.Agent.Uptime > 0 {
				boot := now - c.Agent.Uptime*1000
				// Démarrage postérieur à la dernière extinction constatée : c'est bien ce démarrage-ci
				// (sinon, sortie de veille : l'uptime date d'avant).
				if boot >= p.Since-60_000 && boot <= now {
					e.Time, e.Approx = boot, false
				}
			}
			out = append(out, e)
		case c.State == status.Offline && (p.State == status.Online || p.State == status.ShuttingDown || p.State == status.Restarting):
			at := now
			if p.LastSeen != nil {
				at = *p.LastSeen
			}
			// Après une demande d'extinction, l'arrêt ne peut pas la précéder.
			if p.State != status.Online && at < p.Since {
				at = p.Since
			}
			out = append(out, Event{Device: id, Time: at, Kind: Off, Source: App, Approx: true})
		case c.State == status.Offline && p.State == status.Waking && c.Notice == status.WakeTimeout:
			out = append(out, Event{Device: id, Time: now, Kind: WakeTimeout, Source: App})
		}
	}
	return out
}

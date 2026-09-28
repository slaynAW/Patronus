package history

import (
	"context"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol"
)

const (
	// WatchInterval est la période de surveillance (veille, signe de vie).
	WatchInterval = 15 * time.Second
	// aliveInterval espace les écritures du signe de vie (datation d'un arrêt non enregistré).
	aliveInterval = time.Minute
	// minSleep : en deçà, une interruption n'est pas considérée comme une mise en veille.
	minSleep = time.Minute
)

// Watcher détecte les mises en veille et enregistre régulièrement un signe de vie.
type Watcher struct {
	Log       *Log
	AlivePath string
	Interval  time.Duration
	Now       func() time.Time
	// Suspended renvoie le temps total passé en veille depuis le démarrage (ok = false si le système
	// ne le fournit pas : la veille est alors déduite d'un saut de l'horloge murale).
	Suspended func() (time.Duration, bool)
	// OnError reçoit les erreurs d'écriture (journalisation).
	OnError func(error)
}

// Run surveille jusqu'à l'annulation de ctx.
func (w *Watcher) Run(ctx context.Context) {
	if w.Now == nil {
		w.Now = time.Now
	}
	if w.Interval <= 0 {
		w.Interval = WatchInterval
	}
	if w.Suspended == nil {
		w.Suspended = SuspendedTotal
	}
	report := func(err error) {
		if err != nil && w.OnError != nil {
			w.OnError(err)
		}
	}
	prev := w.Now()
	prevSuspended, suspendedOK := w.Suspended()
	report(WriteAlive(w.AlivePath, prev))
	lastAlive := prev

	ticker := time.NewTicker(w.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		now := w.Now()
		var slept time.Duration
		if suspendedOK {
			if total, ok := w.Suspended(); ok {
				slept = total - prevSuspended
				prevSuspended = total
			}
		} else if gap := now.Round(0).Sub(prev.Round(0)) - w.Interval; gap > 0 {
			// Horloge monotone arrêtée pendant la veille : l'horloge murale, elle, a avancé.
			slept = gap
		}
		if slept >= minSleep {
			sleepAt := prev.Round(0)
			report(w.Log.Add(protocol.HistoryEvent{T: sleepAt.Unix(), K: protocol.HistorySleep}))
			report(w.Log.Add(protocol.HistoryEvent{T: sleepAt.Add(slept).Unix(), K: protocol.HistoryResume}))
		}
		prev = now
		if slept >= minSleep || now.Round(0).Sub(lastAlive.Round(0)) >= aliveInterval {
			report(WriteAlive(w.AlivePath, now))
			lastAlive = now
		}
	}
}

// CurrentBoot décrit le démarrage en cours : identifiant du système et heure (maintenant - uptime).
func CurrentBoot(uptime time.Duration, now time.Time) Boot {
	return Boot{ID: BootID(), At: now.Add(-uptime).Round(time.Second)}
}

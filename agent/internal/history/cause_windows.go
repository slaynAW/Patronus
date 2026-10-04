package history

import (
	"time"

	"github.com/slaynaw/wakeonlan/agent/internal/eventlog"
)

// SystemCause cherche dans le journal d'événements de Windows la cause d'un arrêt anormal survenu
// après since (événements écrits jusqu'à maintenant).
func SystemCause(since, _ time.Time) Cause {
	events, err := eventlog.Query("System", eventlog.XPath(CrashSelectors, since), 50)
	if err != nil {
		return Cause{}
	}
	return ExplainEvents(events)
}

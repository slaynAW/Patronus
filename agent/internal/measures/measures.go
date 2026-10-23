// Package measures lit les mesures que l'agent décrit lui-même dans sa réponse à « status » (mémoire
// utilisée, batterie) : les applications les affichent sans les connaître (docs/PROTOCOLE.md, « Mesures »).
package measures

import (
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol"
)

// Seuils : mémoire utilisée à surveiller puis grave ; batterie faible puis très faible.
const (
	memoryWarn   = 90
	memoryCrit   = 97
	batteryWarn  = 20
	batteryCrit  = 10
	refreshEvery = 5 * time.Second
)

// Read renvoie les mesures lisibles sur ce système (aucune si rien n'est lisible).
func Read() []protocol.Measure {
	var out []protocol.Measure
	if m := memory(); m != nil {
		out = append(out, *m)
	}
	if m := battery(); m != nil {
		out = append(out, *m)
	}
	return out
}

// Cache garde le dernier relevé quelques secondes : chaque réponse à « status » reste immédiate.
type Cache struct {
	read func() []protocol.Measure
	now  func() time.Time
	mu   sync.Mutex
	at   time.Time
	last []protocol.Measure
}

// NewCache prépare un cache autour de read.
func NewCache(read func() []protocol.Measure) *Cache { return &Cache{read: read, now: time.Now} }

// Get renvoie les mesures, relues au plus toutes les 5 s.
func (c *Cache) Get() []protocol.Measure {
	c.mu.Lock()
	defer c.mu.Unlock()
	if now := c.now(); c.at.IsZero() || now.Sub(c.at) >= refreshEvery || now.Before(c.at) {
		c.last, c.at = c.read(), now
	}
	return c.last
}

// memoryMeasure : mémoire vive utilisée, en % (total et disponible en octets).
func memoryMeasure(total, available uint64) *protocol.Measure {
	if total == 0 || available > total {
		return nil
	}
	used := total - available
	return &protocol.Measure{
		ID: "mem", Label: "Mémoire", Unit: "%",
		Value: math.Round(float64(used)*1000/float64(total)) / 10,
		Min:   ptr(0), Max: ptr(100), Warn: ptr(memoryWarn), Crit: ptr(memoryCrit),
		Text: gigabytes(used) + " sur " + gigabytes(total),
	}
}

// Comment la batterie est alimentée.
const (
	charging    = "en charge"
	discharging = "sur batterie"
	pluggedIn   = "sur secteur"
	full        = "chargée"
)

// batteryMeasure : charge de la batterie, en %, et son état (charging…), vide s'il est inconnu.
func batteryMeasure(percent int, state string) *protocol.Measure {
	if percent < 0 || percent > 100 {
		return nil
	}
	return &protocol.Measure{
		ID: "battery", Label: "Batterie", Unit: "%", Value: float64(percent),
		Min: ptr(0), Max: ptr(100), Warn: ptr(batteryWarn), Crit: ptr(batteryCrit), Low: true,
		Text: state,
	}
}

// gigabytes écrit une quantité de mémoire comme l'affichent les applications : « 6,4 Go », « 16 Go ».
func gigabytes(b uint64) string {
	v := math.Round(float64(b)/(1<<30)*10) / 10
	s := strconv.FormatFloat(v, 'f', 1, 64)
	s = strings.TrimSuffix(s, ".0")
	return strings.Replace(s, ".", ",", 1) + " Go"
}

func ptr(v float64) *float64 { return &v }

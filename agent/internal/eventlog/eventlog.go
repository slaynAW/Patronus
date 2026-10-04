// Package eventlog lit le journal d'événements de Windows (plantages, arrêts anormaux, erreurs de
// disque). Les événements sont rendus en XML par Windows : leurs champs ne dépendent pas de la langue.
package eventlog

import (
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Event est un événement du journal.
type Event struct {
	Provider string
	ID       int
	Time     time.Time
	// Data : champs nommés de l'événement (EventData).
	Data map[string]string
}

// ErrUnsupported : pas de journal d'événements sur ce système.
var ErrUnsupported = errors.New("journal d'événements indisponible sur ce système")

// Selector désigne des événements d'une source.
type Selector struct {
	Provider string
	IDs      []int
}

// XPath renvoie la requête des événements de selectors survenus depuis since.
func XPath(selectors []Selector, since time.Time) string {
	var alts []string
	for _, s := range selectors {
		var ids []string
		for _, id := range s.IDs {
			ids = append(ids, fmt.Sprintf("EventID=%d", id))
		}
		alts = append(alts, fmt.Sprintf("(Provider[@Name='%s'] and (%s))", s.Provider, strings.Join(ids, " or ")))
	}
	return fmt.Sprintf("*[System[(%s) and TimeCreated[@SystemTime>='%s']]]",
		strings.Join(alts, " or "), since.UTC().Format("2006-01-02T15:04:05.000Z"))
}

type xmlEvent struct {
	System struct {
		Provider struct {
			Name string `xml:"Name,attr"`
		} `xml:"Provider"`
		EventID     string `xml:"EventID"`
		TimeCreated struct {
			SystemTime string `xml:"SystemTime,attr"`
		} `xml:"TimeCreated"`
	} `xml:"System"`
	Data []struct {
		Name  string `xml:"Name,attr"`
		Value string `xml:",chardata"`
	} `xml:"EventData>Data"`
}

// Parse lit un événement rendu en XML par Windows.
func Parse(text string) (Event, error) {
	var x xmlEvent
	if err := xml.Unmarshal([]byte(text), &x); err != nil {
		return Event{}, err
	}
	e := Event{Provider: x.System.Provider.Name, Data: map[string]string{}}
	if _, err := fmt.Sscan(strings.TrimSpace(x.System.EventID), &e.ID); err != nil {
		return Event{}, fmt.Errorf("identifiant d'événement illisible : %q", x.System.EventID)
	}
	t, err := time.Parse(time.RFC3339Nano, x.System.TimeCreated.SystemTime)
	if err != nil {
		return Event{}, fmt.Errorf("heure d'événement illisible : %w", err)
	}
	e.Time = t
	for i, d := range x.Data {
		name := d.Name
		if name == "" {
			name = fmt.Sprintf("param%d", i+1)
		}
		e.Data[name] = strings.TrimSpace(d.Value)
	}
	return e, nil
}

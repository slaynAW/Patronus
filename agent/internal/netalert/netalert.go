// Package netalert prévient l'utilisateur du PC quand son réseau est classé « Public » par Windows :
// le pare-feu bloque alors l'agent et les applications voient le PC éteint. Le service propose de
// classer le réseau en Privé (une fenêtre « Oui / Non » dans la session de l'utilisateur) et le fait
// seulement avec son accord ; un refus n'est pas redemandé pour ce réseau avant RemindAfter.
package netalert

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/slaynaw/wakeonlan/agent/internal/netprofile"
	"github.com/slaynaw/wakeonlan/agent/internal/session"
)

const (
	// FirstDelay laisse le PC finir de démarrer (réseau identifié) avant la première vérification.
	FirstDelay = time.Minute
	// Interval sépare deux vérifications (changement de réseau, câble rebranché…).
	Interval = 2 * time.Minute
	// AskTimeout : sans réponse, la fenêtre se ferme et la question sera reposée plus tard.
	AskTimeout = time.Hour
	// RemindAfter : délai avant de reposer la question pour un réseau refusé (ou sans réponse).
	RemindAfter = 7 * 24 * time.Hour
)

// Checker vérifie le réseau du PC et prévient l'utilisateur. Les fonctions sont remplaçables pour les tests.
type Checker struct {
	// Interface renvoie le nom de la carte du réseau local ("" : inconnue, tous les réseaux comptent).
	Interface func() string
	Profiles  func() ([]netprofile.Profile, error)
	// Firewall renvoie l'état du pare-feu pour les réseaux Publics.
	Firewall   func() (netprofile.Firewall, error)
	Ask        func(message string) (session.Answer, error)
	Notify     func(message string, warning bool)
	SetPrivate func(index int) error
	// StatePath : fichier des réseaux pour lesquels la question a déjà été posée.
	StatePath string
	Logf      func(format string, args ...any)
	Now       func() time.Time
}

type state struct {
	// Asked associe le nom d'un réseau à la date (Unix) de la dernière question restée sans suite.
	Asked map[string]int64 `json:"asked"`
}

// Run vérifie le réseau régulièrement jusqu'à l'annulation de ctx.
func (c *Checker) Run(ctx context.Context) {
	timer := time.NewTimer(FirstDelay)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		c.Once()
		timer.Reset(Interval)
	}
}

// Once fait une vérification (et pose la question si nécessaire).
func (c *Checker) Once() {
	profiles, err := c.Profiles()
	if err != nil {
		c.logf("réseau : type du réseau illisible : %v", err)
		return
	}
	iface := ""
	if c.Interface != nil {
		iface = c.Interface()
	}
	public, ok := netprofile.FirstPublic(netprofile.Relevant(profiles, iface))
	if !ok {
		return
	}
	fw, err := c.Firewall()
	if err != nil {
		c.logf("réseau : pare-feu illisible : %v", err)
		return
	}
	if !fw.BlocksPublic() {
		return
	}
	st := c.load()
	if at, asked := st.Asked[public.Name]; asked && c.now().Sub(time.Unix(at, 0)) < RemindAfter {
		return
	}
	c.logf("réseau « %s » (carte %s) classé Public : le pare-feu de Windows bloque l'agent", public.Name, public.Interface)
	answer, err := c.Ask(Question(public))
	switch {
	case answer == session.NoUser:
		return // personne n'est connecté : question posée à la prochaine vérification
	case err != nil:
		c.logf("réseau : question impossible : %v", err)
		return
	case answer == session.Yes:
		if err := c.SetPrivate(public.Index); err != nil {
			c.logf("réseau « %s » : classement en Privé impossible : %v", public.Name, err)
			c.Notify(fmt.Sprintf("Le réseau « %s » n'a pas pu être classé en Privé (%v).\n\nFaites-le à la main : %s",
				public.Name, err, netprofile.Advice), true)
			return
		}
		c.logf("réseau « %s » classé en Privé à la demande de l'utilisateur", public.Name)
		c.Notify(fmt.Sprintf("Le réseau « %s » est désormais Privé : le téléphone et l'application Windows peuvent "+
			"joindre l'agent Patronus de ce PC.", public.Name), false)
		delete(st.Asked, public.Name)
	default:
		c.logf("réseau « %s » laissé en Public (réponse : %v) ; question reposée dans %s", public.Name, answer, RemindAfter)
		st.Asked[public.Name] = c.now().Unix()
	}
	c.save(st)
}

// Question est le texte de la fenêtre proposée à l'utilisateur.
func Question(p netprofile.Profile) string {
	return fmt.Sprintf("Ce PC est connecté au réseau « %s », classé « Public » par Windows.\n\n"+
		"Sur un réseau Public, le pare-feu de Windows bloque l'agent Patronus : le téléphone et "+
		"l'application Windows voient ce PC comme éteint et ne peuvent pas l'éteindre.\n\n"+
		"S'il s'agit de votre réseau domestique (votre box, un réseau de confiance), le classer en "+
		"« Privé » règle le problème. Le faire maintenant ?\n\n"+
		"Répondez « Non » sur un réseau vraiment public (café, hôtel, gare…).", p.Name)
}

func (c *Checker) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Checker) logf(format string, args ...any) {
	if c.Logf != nil {
		c.Logf(format, args...)
	}
}

func (c *Checker) load() state {
	st := state{Asked: map[string]int64{}}
	if data, err := os.ReadFile(c.StatePath); err == nil {
		_ = json.Unmarshal(data, &st)
	}
	if st.Asked == nil {
		st.Asked = map[string]int64{}
	}
	return st
}

func (c *Checker) save(st state) {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return
	}
	tmp := c.StatePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		c.logf("réseau : état non enregistré : %v", err)
		return
	}
	if err := os.Rename(tmp, c.StatePath); err != nil {
		_ = os.Remove(tmp)
		c.logf("réseau : état non enregistré : %v", err)
	}
}

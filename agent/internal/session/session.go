// Package session affiche des fenêtres à l'utilisateur connecté au PC depuis le service de l'agent,
// qui tourne sans bureau (Windows : session 0). Hors Windows, rien n'est affiché.
package session

import "time"

// Answer est la réponse de l'utilisateur à une question.
type Answer int

const (
	// NoUser : personne n'est connecté au PC (ou affichage impossible sur ce système).
	NoUser Answer = iota
	// Yes : l'utilisateur a répondu « Oui ».
	Yes
	// No : l'utilisateur a répondu « Non », ou fermé la fenêtre.
	No
	// Timeout : la fenêtre s'est fermée sans réponse.
	Timeout
)

// Title est le titre des fenêtres affichées par l'agent.
const Title = "Patronus – agent"

// Ask pose une question « Oui / Non » à l'utilisateur connecté et attend sa réponse (au plus timeout) ;
// warning : icône d'avertissement (sinon d'information).
func Ask(message string, timeout time.Duration, warning bool) (Answer, error) {
	return ask(message, timeout, warning)
}

// Notify informe l'utilisateur connecté, sans attendre ; sans effet si personne n'est connecté.
func Notify(message string, warning bool) { notify(message, warning) }

func (a Answer) String() string {
	switch a {
	case Yes:
		return "oui"
	case No:
		return "non"
	case Timeout:
		return "sans réponse"
	}
	return "personne n'est connecté"
}

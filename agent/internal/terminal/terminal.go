// Package terminal gère les particularités de la console (couleurs ANSI, pause, élévation Windows).
package terminal

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// Pause attend que l'utilisateur appuie sur Entrée (fenêtre ouverte par double-clic sous Windows).
func Pause() {
	fmt.Print("\nAppuyez sur Entrée pour fermer cette fenêtre…")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
}

// Confirm pose une question oui/non ; oui par défaut.
func Confirm(question string) bool {
	fmt.Printf("%s [O/n] ", question)
	answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	switch answer {
	case "n\n", "N\n", "non\n", "n\r\n", "N\r\n", "non\r\n":
		return false
	}
	return true
}

// ReadPassword demande un mot de passe sans l'afficher (saisie ordinaire si l'entrée n'est pas une
// console : redirection, tests).
func ReadPassword(prompt string) (string, error) {
	fmt.Print(prompt)
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		b, err := term.ReadPassword(fd)
		fmt.Println()
		return string(b), err
	}
	line, err := stdin.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// stdin est partagé : deux lectures successives ne perdent pas de ligne dans un tampon.
var stdin = bufio.NewReader(os.Stdin)

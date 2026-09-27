// Package terminal gère les particularités de la console (couleurs ANSI, pause, élévation Windows).
package terminal

import (
	"bufio"
	"fmt"
	"os"
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

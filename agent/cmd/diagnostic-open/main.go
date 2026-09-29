// Commande diagnostic-open : ouvre un rapport de diagnostic Patronus (docs/DIAGNOSTIC.md).
//
//	cd agent && go run ./cmd/diagnostic-open rapport.diag [texte.txt]
//
// Le mot de passe est lu dans la variable PATRONUS_DIAG_PASSWORD, sinon sur l'entrée standard.
// Sans fichier de sortie, le texte est affiché.
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/slaynaw/wakeonlan/agent/diagnostic"
)

func main() {
	if len(os.Args) < 2 || len(os.Args) > 3 {
		fmt.Fprintln(os.Stderr, "usage : diagnostic-open <rapport> [fichier-texte]")
		os.Exit(2)
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fail(err)
	}
	password := os.Getenv("PATRONUS_DIAG_PASSWORD")
	if password == "" {
		fmt.Fprint(os.Stderr, "Mot de passe du rapport : ")
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		password = strings.TrimRight(line, "\r\n")
	}
	env, text, err := diagnostic.Open(data, password)
	if err != nil {
		fail(err)
	}
	fmt.Fprintf(os.Stderr, "%s — %s\n", env.App, env.CreatedAt)
	if len(os.Args) == 3 {
		if err := os.WriteFile(os.Args[2], []byte(text), 0o600); err != nil {
			fail(err)
		}
		return
	}
	fmt.Print(text)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "Erreur :", err)
	os.Exit(1)
}

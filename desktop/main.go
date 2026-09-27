// Wake On LAN pour Windows : même interface et mêmes fonctions que l'application Android.
//
// Le moteur (réveil, état en temps réel, agent, sauvegardes) est en Go ; l'interface est une page
// HTML affichée par Microsoft Edge WebView2 (intégré à Windows 10/11). Un seul fichier .exe.
package main

import (
	"embed"
	"os"
	"path/filepath"
	"strings"
)

// version est fixée à la compilation : -ldflags "-X main.version=1.1.0".
var version = "dev"

//go:embed ui
var uiFiles embed.FS

// inlinedPage renvoie la page de l'interface avec sa feuille de style et son script intégrés
// (une seule chaîne, affichée sans serveur ni fichier temporaire).
func inlinedPage() (string, error) {
	read := func(name string) (string, error) {
		b, err := uiFiles.ReadFile("ui/" + name)
		return string(b), err
	}
	page, err := read("index.html")
	if err != nil {
		return "", err
	}
	css, err := read("app.css")
	if err != nil {
		return "", err
	}
	js, err := read("app.js")
	if err != nil {
		return "", err
	}
	page = strings.Replace(page, `<link rel="stylesheet" href="app.css">`, "<style>\n"+css+"</style>", 1)
	// Un « </script> » dans le script fermerait la balise : il n'y en a pas, on le vérifie.
	if strings.Contains(js, "</script") {
		return "", os.ErrInvalid
	}
	page = strings.Replace(page, `<script src="app.js"></script>`, "<script>\n"+js+"</script>", 1)
	return page, nil
}

// defaultDataDir renvoie le dossier de configuration (Windows : %APPDATA%\WakeOnLan), ou celui
// indiqué par la variable d'environnement WOL_DATA_DIR (tests, utilisation « portable »).
func defaultDataDir() string {
	if dir := os.Getenv("WOL_DATA_DIR"); dir != "" {
		return dir
	}
	base, err := os.UserConfigDir()
	if err != nil {
		base = "."
	}
	return filepath.Join(base, "WakeOnLan")
}

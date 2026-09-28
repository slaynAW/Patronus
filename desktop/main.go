// Wake On LAN pour Windows : même interface et mêmes fonctions que l'application Android.
//
// Le moteur (réveil, état en temps réel, agent, sauvegardes) est en Go ; l'interface est une page
// HTML affichée par Microsoft Edge WebView2 (intégré à Windows 10/11). Un seul fichier .exe.
package main

import (
	"embed"
	"encoding/base64"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// version et buildCode (numéro croissant de la CI, comparé pour les mises à jour) sont fixés à la
// compilation : -ldflags "-X main.version=1.2.0 -X main.buildCode=42".
var (
	version   = "dev"
	buildCode = "0"
)

//go:embed ui
var uiFiles embed.FS

// fontURL repère les polices de la feuille de style (dossier ui/fonts).
var fontURL = regexp.MustCompile(`url\(fonts/([a-z0-9-]+\.woff2)\)`)

// inlinedPage renvoie la page de l'interface avec sa feuille de style, ses polices et son script
// intégrés (une seule chaîne, affichée sans serveur ni fichier temporaire).
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
	if css, err = inlineFonts(css); err != nil {
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

// inlineFonts remplace les références aux polices par leur contenu (URL « data: »).
func inlineFonts(css string) (string, error) {
	var failed error
	css = fontURL.ReplaceAllStringFunc(css, func(match string) string {
		b, err := uiFiles.ReadFile("ui/fonts/" + fontURL.FindStringSubmatch(match)[1])
		if err != nil {
			failed = err
			return match
		}
		return "url(data:font/woff2;base64," + base64.StdEncoding.EncodeToString(b) + ")"
	})
	return css, failed
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

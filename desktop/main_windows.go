package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"time"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"

	"github.com/slaynaw/wakeonlan/agent/update"
	"github.com/slaynaw/wakeonlan/desktop/internal/app"
	"github.com/slaynaw/wakeonlan/desktop/internal/config"
	"github.com/slaynaw/wakeonlan/desktop/internal/history"
)

const (
	windowTitle = "Patronus"
	// iconResourceID est l'identifiant de l'icône dans les ressources (winres/winres.json : « #1 »).
	iconResourceID = 1
	// webView2DownloadURL : programme d'installation officiel du composant WebView2 (Microsoft).
	webView2DownloadURL = "https://go.microsoft.com/fwlink/p/?LinkId=2124703"
)

// afterUpdateFlag est passé à la nouvelle version lancée à la fin d'une mise à jour.
const afterUpdateFlag = "--after-update"

// currentExecutable renvoie le chemin du programme en cours (vide s'il est inconnu).
func currentExecutable() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe
}

// updateOptions active les mises à jour intégrées pour les versions publiées par la CI (numéro de
// build connu, certificat de signature présent) ; jamais pendant l'autotest de démarrage.
func updateOptions(exe string) *app.UpdateOptions {
	code, _ := strconv.ParseInt(buildCode, 10, 64)
	if version == "dev" || code <= 0 || exe == "" || selfTestPath != "" {
		return nil
	}
	src, err := update.Official()
	if err != nil {
		log.Printf("mises à jour désactivées : %v", err)
		return nil
	}
	src.UserAgent = "Patronus-Windows"
	return &app.UpdateOptions{Source: src, Code: code, Platform: "windows-" + runtime.GOARCH, Exe: exe}
}

// agentOptions active le téléchargement de l'agent depuis l'application : dernière version publiée,
// vérifiée avec le manifeste signé (certificat intégré). Fichiers de travail dans
// %LOCALAPPDATA%\WakeOnLan\Agent, dossier propre à l'utilisateur, vidé au démarrage.
func agentOptions() *app.AgentOptions {
	if selfTestPath != "" {
		return nil
	}
	src, err := update.Official()
	if err != nil {
		log.Printf("téléchargement de l'agent désactivé : %v", err)
		return nil
	}
	local, err := os.UserCacheDir()
	if err != nil {
		return nil
	}
	src.UserAgent = "Patronus-Windows"
	return &app.AgentOptions{Source: src, Platform: "windows-" + runtime.GOARCH, Dir: filepath.Join(local, "WakeOnLan", "Agent")}
}

// selfTestPath (variable WOL_SELFTEST) active l'autotest de démarrage utilisé par la CI : la fenêtre
// s'ouvre, l'interface se charge, puis le résultat est écrit dans ce fichier et l'application se ferme.
var selfTestPath = os.Getenv("WOL_SELFTEST")

func main() {
	setDPIAware()
	if selfTestPath != "" {
		go func() {
			time.Sleep(90 * time.Second)
			selfTestDone(false, "l'interface ne s'est pas affichée en 90 s")
		}()
	}
	// Une seule fenêtre : un second lancement ramène simplement la fenêtre existante au premier plan.
	// Après une mise à jour, la nouvelle version attend que l'ancienne se soit fermée.
	afterUpdate := slices.Contains(os.Args[1:], afterUpdateFlag)
	acquired := acquireSingleInstance()
	for tries := 0; !acquired && afterUpdate && tries < 60; tries++ {
		time.Sleep(250 * time.Millisecond)
		acquired = acquireSingleInstance()
	}
	if !acquired {
		focusExistingWindow(windowTitle)
		return
	}
	exe := currentExecutable()
	if exe != "" {
		go update.CleanupOld(exe) // version précédente laissée par une mise à jour
	}

	dataDir := defaultDataDir()
	store, err := config.NewStore(dataDir)
	if err != nil {
		fatal("Impossible de créer le dossier de configuration :\n" + err.Error())
	}
	setupLog(dataDir)
	page, err := inlinedPage()
	if err != nil {
		fatal("Interface introuvable : " + err.Error())
	}

	webData := filepath.Join(dataDir, "WebView2")
	if local, err := os.UserCacheDir(); err == nil { // %LOCALAPPDATA%
		webData = filepath.Join(local, "WakeOnLan", "WebView2")
	}
	width, height := initialWindowSize()
	// Fond sombre dès la création de la vue (pas d'éclair blanc avant l'affichage de la page).
	_ = os.Setenv("WEBVIEW2_DEFAULT_BACKGROUND_COLOR", "FF0C0E11")
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     os.Getenv("WOL_DEBUG") == "1",
		DataPath:  webData,
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title:  windowTitle,
			Width:  uint(width),
			Height: uint(height),
			IconId: iconResourceID,
			Center: true,
		},
	})
	if w == nil {
		askInstallWebView2()
		return
	}
	defer w.Destroy()
	hwnd := windows.HWND(uintptr(w.Window()))
	minW, minH := scaled(720), scaled(520)
	w.SetSize(minW, minH, webview2.HintMin)

	platform := &winPlatform{w: w, hwnd: hwnd, exe: exe}
	histStore, err := history.NewStore(dataDir)
	if err != nil {
		fatal("Impossible de créer le dossier de l'historique :\n" + err.Error())
	}
	svc := app.New(app.Options{
		Version: version, Store: store, Platform: platform, History: histStore,
		Updates: updateOptions(exe), Share: shareOptions(dataDir), Agent: agentOptions(),
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go svc.Run(ctx)

	// État poussé vers l'interface à chaque changement (regroupé toutes les 100 ms au plus).
	changed := make(chan struct{}, 1)
	svc.OnChange(func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	})
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-changed:
			}
			data, err := json.Marshal(svc.State())
			if err == nil {
				js := "window.__goState && window.__goState(" + string(data) + ")"
				w.Dispatch(func() { w.Eval(js) })
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()

	// Appels de l'interface : exécutés hors du fil de la fenêtre (réseau, chiffrement), puis la
	// réponse est renvoyée à la page via __goResolve.
	err = w.Bind("goInvoke", func(id int, method string, params string) {
		if method == "uiReady" && selfTestPath != "" {
			go selfTestDone(true, json.RawMessage(params))
		}
		go func() {
			result, err := svc.Call(method, json.RawMessage(params))
			ok := err == nil
			var payload []byte
			if ok {
				if payload, err = json.Marshal(result); err != nil {
					ok = false
				}
			}
			if !ok {
				payload, _ = json.Marshal(err.Error())
			}
			js := fmt.Sprintf("window.__goResolve(%d, %t, %s)", id, ok, payload)
			w.Dispatch(func() { w.Eval(js) })
		}()
	})
	if err != nil {
		fatal(err.Error())
	}

	// Barre de titre sombre, assortie à l'interface ; surveillance en pause quand la fenêtre est
	// réduite (comme l'application Android en arrière-plan).
	setDarkTitleBar(hwnd)
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			svc.SetWindowActive(!isIconic(hwnd))
		}
	}()

	w.SetHtml(page)
	w.Run()
}

// initialWindowSize renvoie la taille de la fenêtre au lancement (plan du réseau et panneau de
// détail côte à côte), bornée par l'écran.
func initialWindowSize() (int, int) {
	width, height := scaled(1200), scaled(800)
	if sw, sh := workArea(); sw > 0 && sh > 0 {
		width = min(width, sw*9/10)
		height = min(height, sh*9/10)
	}
	return width, height
}

func setupLog(dir string) {
	path := filepath.Join(dir, "wakeonlan.log")
	if info, err := os.Stat(path); err == nil && info.Size() > 1<<20 {
		_ = os.Remove(path)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		log.SetOutput(io.Discard)
		return
	}
	log.SetOutput(f)
	log.Printf("Patronus %s démarré", version)
}

func askInstallWebView2() {
	if selfTestPath != "" {
		selfTestDone(false, "WebView2 indisponible")
	}
	text := "Le composant Microsoft Edge WebView2, nécessaire à l'affichage, est introuvable.\n\n" +
		"Il est inclus dans Windows 11 et installé par les mises à jour de Windows 10.\n\n" +
		"Ouvrir la page de téléchargement de Microsoft ?"
	if messageBox(text, windows.MB_YESNO|windows.MB_ICONWARNING) == idYes {
		_ = shellOpen(0, webView2DownloadURL)
	}
}

func fatal(message string) {
	log.Print(message)
	if selfTestPath != "" {
		selfTestDone(false, message)
	}
	messageBox(message, windows.MB_OK|windows.MB_ICONERROR)
	os.Exit(1)
}

// selfTestDone écrit le résultat de l'autotest et quitte.
func selfTestDone(ok bool, detail any) {
	data, _ := json.Marshal(map[string]any{"ok": ok, "version": version, "detail": detail})
	_ = os.WriteFile(selfTestPath, data, 0o600)
	log.Printf("autotest : %s", data)
	if ok {
		os.Exit(0)
	}
	os.Exit(1)
}

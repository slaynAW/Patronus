// Package protocol implémente le protocole « wolagent/1 » (voir docs/PROTOCOLE.md).
//
// Chaque échange TCP se déroule ainsi (une ligne JSON par message) :
//
//	agent → app : {"proto":"wolagent/1","nonce":"<32 octets aléatoires>"}
//	app → agent : {"cnonce":"<16 octets>","body":"<JSON>","mac":"<HMAC>"}
//	agent → app : {"body":"<JSON>","mac":"<HMAC>"}   ou   {"error":"unauthorized"}
//
// Les signatures HMAC-SHA256 couvrent les deux nonces : un message capturé ne peut être ni rejoué
// ni modifié, et la clé partagée ne circule jamais sur le réseau.
package protocol

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
)

const (
	// Proto identifie la version du protocole.
	Proto = "wolagent/1"
	// ServerNonceBytes est la taille du défi envoyé par l'agent.
	ServerNonceBytes = 32
	// ClientNonceBytes est la taille du nonce choisi par l'application.
	ClientNonceBytes = 16
	// KeyBytes est la taille de la clé partagée.
	KeyBytes = 32
	// MaxLineBytes borne la taille d'un message.
	MaxLineBytes = 8 * 1024
	// MaxHistoryBytes borne la taille de la réponse à la commande « history » (journal de 30 jours).
	MaxHistoryBytes = 512 * 1024
	// MaxMetricsBytes borne la taille de la réponse à la commande « metrics » (une journée de mesures).
	MaxMetricsBytes = 512 * 1024
	// MetricsDays : nombre de jours de mesures gardés par l'agent.
	MetricsDays = 90
	// MaxWakes borne le nombre de démarrages signalés en une fois (commande « wakes »).
	MaxWakes = 50
	// MaxByLength borne le nom de l'appareil indiqué par l'application (en caractères).
	MaxByLength = 40
)

// Commandes du journal, autorisées avec « status » (agent 1.4.0 ou plus pour « wakes »).
const (
	CmdStatus  = "status"
	CmdHistory = "history"
	// CmdWakes : l'application signale les démarrages qu'elle a demandés (paquet magique, que l'agent
	// ne peut pas voir), une fois le PC joignable. La réponse contient le journal à jour.
	CmdWakes = "wakes"
	// CmdMetrics : mesures enregistrées en continu, une ligne par minute (agent 1.8.0) ; RequestBody.Day
	// choisit le jour (UTC), la réponse donne aussi la liste des jours disponibles.
	CmdMetrics = "metrics"
)

// Types d'évènements du journal de l'agent (commande « history »).
const (
	// HistoryBoot : démarrage du PC (heure réelle, calculée depuis l'uptime).
	HistoryBoot = "boot"
	// HistoryShutdown : arrêt propre du PC (service arrêté par le système).
	HistoryShutdown = "shutdown"
	// HistoryLost : arrêt non enregistré (coupure de courant, arrêt forcé) ; heure du dernier signe de vie.
	HistoryLost = "lost"
	// HistorySleep : mise en veille (ou arrêt avec « démarrage rapide » sous Windows).
	HistorySleep = "sleep"
	// HistoryResume : sortie de veille.
	HistoryResume = "resume"
	// HistoryCommand : commande d'alimentation reçue (A = action, C = adresse du client, B = son nom).
	HistoryCommand = "cmd"
	// HistoryWake : démarrage demandé par une application (C = adresse, B = nom de l'appareil).
	HistoryWake = "wake"
)

// HistoryEvent est un évènement du journal de l'agent.
type HistoryEvent struct {
	// T est l'heure de l'évènement (secondes Unix).
	T int64  `json:"t"`
	K string `json:"k"`
	// A est l'action d'une commande (shutdown, reboot, sleep).
	A string `json:"a,omitempty"`
	// C est l'adresse IP du client à l'origine d'une commande.
	C string `json:"c,omitempty"`
	// B est le nom de l'appareil à l'origine d'une commande, indiqué par l'application (« Pixel 8 »).
	B string `json:"b,omitempty"`
	// R est la cause d'un arrêt non enregistré (HistoryLost), lue dans le journal d'événements de
	// Windows : LostBSOD, LostButton, LostPower ou LostHardware (vide : inconnue ; agent 1.9.0 ou plus).
	R string `json:"r,omitempty"`
	// D précise la cause : code et nom de l'écran bleu (« 0x7E SYSTEM_THREAD_EXCEPTION_NOT_HANDLED »),
	// composant en panne…
	D string `json:"d,omitempty"`
}

// Causes d'un arrêt non enregistré (HistoryEvent.R).
const (
	// LostBSOD : plantage du système (écran bleu), D donne le code d'arrêt.
	LostBSOD = "bsod"
	// LostButton : arrêt forcé avec le bouton d'alimentation.
	LostButton = "button"
	// LostPower : coupure de courant ou blocage complet (redémarrage sans arrêt propre).
	LostPower = "power"
	// LostHardware : erreur matérielle fatale (processeur, mémoire, bus), D donne le composant.
	LostHardware = "hardware"
)

// History est le journal renvoyé par la commande « history ».
type History struct {
	// From est le début de la période couverte par le journal (secondes Unix).
	From   int64          `json:"from"`
	Events []HistoryEvent `json:"events"`
}

// Erreurs renvoyées (non signées) quand l'authentification est impossible.
const (
	ErrUnauthorized = "unauthorized"
	ErrRateLimited  = "rate_limited"
	ErrBadRequest   = "bad_request"
)

// Hello est le premier message, envoyé par l'agent.
type Hello struct {
	Proto string `json:"proto"`
	Nonce string `json:"nonce"`
}

// Request est la requête signée de l'application.
type Request struct {
	CNonce string `json:"cnonce"`
	Body   string `json:"body"`
	Mac    string `json:"mac"`
}

// RequestBody est le contenu (signé) d'une requête.
type RequestBody struct {
	Cmd   string `json:"cmd"`
	Delay *int   `json:"delay,omitempty"`
	Force *bool  `json:"force,omitempty"`
	// By est le nom de l'appareil qui envoie la requête (noté dans le journal ; ignoré avant 1.4.0).
	By string `json:"by,omitempty"`
	// Wakes : heures (secondes Unix) des démarrages demandés, pour la commande « wakes ».
	Wakes []int64 `json:"wakes,omitempty"`
	// Day : jour des mesures demandées (« 2026-10-03 », UTC), pour la commande « metrics » ; vide pour
	// la seule liste des jours.
	Day string `json:"day,omitempty"`
}

// Response est la réponse de l'agent : signée (Body + Mac) ou erreur d'authentification (Error).
type Response struct {
	Body  string `json:"body,omitempty"`
	Mac   string `json:"mac,omitempty"`
	Error string `json:"error,omitempty"`
}

// ResponseBody est le contenu (signé) d'une réponse.
type ResponseBody struct {
	OK       bool   `json:"ok"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Version  string `json:"version"`
	Uptime   int64  `json:"uptime"`
	// History n'est renseigné que pour la commande « history ».
	History *History `json:"history,omitempty"`
	// Metrics n'est renseigné que pour la commande « metrics » (agent 1.8.0 ou plus).
	Metrics *Metrics `json:"metrics,omitempty"`
	// Temperatures n'est renseigné que pour la commande « status » (agent 1.5.0 ou plus) ; il
	// contient aussi l'utilisation du processeur et de la carte graphique (agent 1.7.0 ou plus).
	Temperatures *Temperatures `json:"temperatures,omitempty"`
	// Disks n'est renseigné que pour la commande « status » (agent 1.9.0 ou plus).
	Disks *Disks `json:"disks,omitempty"`
}

// Disks décrit les disques du PC : espace des lecteurs, santé des disques physiques et erreurs
// d'accès signalées par le système. Une valeur illisible est absente.
type Disks struct {
	// Volumes : lecteurs locaux (« C: », « D: » ; points de montage sous Linux).
	Volumes []Volume `json:"volumes,omitempty"`
	// Drives : disques physiques (SSD, disques durs).
	Drives []Drive `json:"drives,omitempty"`
	// Errors : erreurs d'accès aux disques signalées par Windows sur les DiskErrorDays derniers
	// jours (secteurs illisibles, contrôleur, système de fichiers) ; LastError : la plus récente
	// (secondes Unix).
	Errors    int   `json:"errors,omitempty"`
	LastError int64 `json:"lastError,omitempty"`
}

// DiskErrorDays : période des erreurs d'accès comptées (Disks.Errors).
const DiskErrorDays = 30

// Volume est un lecteur et son espace (octets).
type Volume struct {
	Mount string `json:"mount"`
	Label string `json:"label,omitempty"`
	FS    string `json:"fs,omitempty"`
	Total uint64 `json:"total"`
	Free  uint64 `json:"free"`
}

// Drive est un disque physique et sa santé.
type Drive struct {
	Name string `json:"name"`
	// Media : DriveSSD ou DriveHDD (vide : inconnu) ; Bus : « NVMe », « SATA », « USB »…
	Media string `json:"media,omitempty"`
	Bus   string `json:"bus,omitempty"`
	Size  uint64 `json:"size,omitempty"`
	// Health : état donné par le système (DriveHealthy, DriveWarning, DriveUnhealthy ; vide : inconnu).
	Health string `json:"health,omitempty"`
	// Temp / TempMax : température actuelle et maximale atteinte (°C).
	Temp    *float64 `json:"temp,omitempty"`
	TempMax *float64 `json:"tempMax,omitempty"`
	// Wear : usure en % de la durée de vie prévue (SSD).
	Wear *int `json:"wear,omitempty"`
	// Hours : heures de fonctionnement.
	Hours *int64 `json:"hours,omitempty"`
	// ReadErrors / WriteErrors : erreurs de lecture / d'écriture non corrigées.
	ReadErrors  *int64 `json:"readErrors,omitempty"`
	WriteErrors *int64 `json:"writeErrors,omitempty"`
}

// Valeurs de Drive.Media et Drive.Health.
const (
	DriveSSD       = "ssd"
	DriveHDD       = "hdd"
	DriveHealthy   = "ok"
	DriveWarning   = "warning"
	DriveUnhealthy = "bad"
)

// Temperatures donne les températures du PC en °C et l'utilisation du processeur et de la carte
// graphique en % ; un capteur illisible est absent.
type Temperatures struct {
	CPU *float64 `json:"cpu,omitempty"`
	GPU *float64 `json:"gpu,omitempty"`
	// CPULoad : utilisation du processeur en % (0 à 100), mesurée sur une seconde comme le
	// Gestionnaire des tâches (agent 1.7.0 ou plus).
	CPULoad *float64 `json:"cpuLoad,omitempty"`
	// GPULoad : utilisation de la carte graphique GPUName en % (0 à 100) : moteur le plus occupé,
	// comme le Gestionnaire des tâches (agent 1.7.0 ou plus).
	GPULoad *float64 `json:"gpuLoad,omitempty"`
	// GPUName est le nom de la carte graphique (« NVIDIA GeForce RTX 4070 »).
	GPUName string `json:"gpuName,omitempty"`
	// GPUShared : la carte graphique est intégrée au processeur et n'a pas de sonde lisible à
	// part (Intel UHD Graphics…) ; GPU vaut alors la température de la puce, celle du processeur
	// (agent 1.6.1 ou plus).
	GPUShared bool `json:"gpuShared,omitempty"`
	// CPUHint explique l'absence de la température du processeur : CPUHintLHM sous Windows.
	CPUHint string `json:"cpuHint,omitempty"`
	// LHM précise ce qui empêche de lire LibreHardwareMonitor quand CPUHint vaut CPUHintLHM
	// (agent 1.6.0 ou plus ; les applications plus anciennes s'en tiennent à CPUHint).
	LHM string `json:"lhm,omitempty"`
}

// Metrics : mesures d'un jour, renvoyées par la commande « metrics ».
type Metrics struct {
	// Day : jour demandé (« 2026-10-03 », UTC), vide si seule la liste des jours est demandée.
	Day string `json:"day"`
	// Days : jours disponibles sur le PC (MetricsDays au plus), du plus ancien au plus récent.
	Days []string `json:"days"`
	// Rows : une ligne par minute, dans l'ordre.
	Rows []MetricsRow `json:"rows"`
}

// MetricsRow : mesures d'une minute (relevés toutes les 5 à 10 s) ; une valeur absente n'a pas pu
// être lue pendant cette minute. Températures en °C, utilisation en %.
type MetricsRow struct {
	// T : début de la minute (secondes Unix).
	T int64 `json:"t"`
	// N : nombre de relevés de la minute.
	N int `json:"n"`
	// CPUTemp / CPUTempMax : température moyenne et maximale du processeur.
	CPUTemp    *float64 `json:"ct,omitempty"`
	CPUTempMax *float64 `json:"ctx,omitempty"`
	// GPUTemp / GPUTempMax : température moyenne et maximale de la carte graphique.
	GPUTemp    *float64 `json:"gt,omitempty"`
	GPUTempMax *float64 `json:"gtx,omitempty"`
	// CPULoad / CPULoadMax : utilisation moyenne et maximale du processeur.
	CPULoad    *float64 `json:"cl,omitempty"`
	CPULoadMax *float64 `json:"clx,omitempty"`
	// GPULoad / GPULoadMax : utilisation moyenne et maximale de la carte graphique.
	GPULoad    *float64 `json:"gl,omitempty"`
	GPULoadMax *float64 `json:"glx,omitempty"`
}

// CPUHintLHM : sous Windows, la température du processeur est lue dans LibreHardwareMonitor, qui ne
// la fournit pas (voir Temperatures.LHM).
const CPUHintLHM = "lhm"

// États de LibreHardwareMonitor (Temperatures.LHM).
const (
	// LHMNotRunning : LibreHardwareMonitor ne tourne pas sur ce PC.
	LHMNotRunning = "not-running"
	// LHMWebOff : il tourne, mais son serveur web (Options → Remote Web Server → Run) ne répond
	// pas ; c'est le seul moyen de le lire depuis sa version 0.9.5 (WMI retiré).
	LHMWebOff = "web-off"
	// LHMAuth : son serveur web demande un mot de passe (Options → Remote Web Server → Authentication).
	LHMAuth = "auth"
	// LHMNoSensor : il répond, mais sans température du processeur (pilote PawnIO absent, processeur
	// non reconnu…).
	LHMNoSensor = "no-sensor"
)

var b64 = base64.RawURLEncoding

// RequestMAC calcule la signature d'une requête.
func RequestMAC(key []byte, nonce, cnonce, body string) string {
	return sign(key, Proto+"\nrequest\n"+nonce+"\n"+cnonce+"\n"+body)
}

// ResponseMAC calcule la signature d'une réponse.
func ResponseMAC(key []byte, nonce, cnonce, body string) string {
	return sign(key, Proto+"\nresponse\n"+nonce+"\n"+cnonce+"\n"+body)
}

// MACEqual compare deux signatures en temps constant.
func MACEqual(expected, received string) bool {
	a, err1 := b64.DecodeString(expected)
	b, err2 := b64.DecodeString(received)
	if err1 != nil || err2 != nil {
		return false
	}
	return hmac.Equal(a, b)
}

// NewNonce renvoie n octets aléatoires encodés en Base64 URL.
func NewNonce(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return b64.EncodeToString(buf), nil
}

// DecodedLen renvoie la taille décodée d'une valeur Base64 URL, ou -1 si elle est invalide.
func DecodedLen(value string) int {
	raw, err := b64.DecodeString(value)
	if err != nil {
		return -1
	}
	return len(raw)
}

// NewKey génère une clé partagée aléatoire de 256 bits.
func NewKey() (string, error) { return NewNonce(KeyBytes) }

// DecodeKey décode et vérifie une clé partagée.
func DecodeKey(value string) ([]byte, error) {
	raw, err := b64.DecodeString(value)
	if err != nil || len(raw) != KeyBytes {
		return nil, errors.New("clé invalide : 32 octets en Base64 URL attendus")
	}
	return raw, nil
}

func sign(key []byte, message string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(message))
	return b64.EncodeToString(mac.Sum(nil))
}

// Package app assemble les composants et expose les actions de l'interface (équivalent des
// ViewModels Android) sous forme d'appels JSON : l'interface HTML appelle Call(méthode, paramètres)
// et reçoit l'état complet à chaque changement.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/slaynaw/wakeonlan/desktop/internal/agentclient"
	"github.com/slaynaw/wakeonlan/desktop/internal/config"
	"github.com/slaynaw/wakeonlan/desktop/internal/history"
	"github.com/slaynaw/wakeonlan/desktop/internal/model"
	"github.com/slaynaw/wakeonlan/desktop/internal/netstate"
	"github.com/slaynaw/wakeonlan/desktop/internal/pairing"
	"github.com/slaynaw/wakeonlan/desktop/internal/status"
	"github.com/slaynaw/wakeonlan/desktop/internal/wol"
)

// URLs ouvrables depuis l'interface (liste blanche : l'interface ne peut pas ouvrir n'importe quoi).
const (
	RepoURL     = "https://github.com/slaynAW/WakeOnLan"
	ReleasesURL = RepoURL + "/releases"
)

// ErrCancelled signale que l'utilisateur a annulé une boîte de dialogue.
var ErrCancelled = errors.New("annulé")

// Platform regroupe les fonctions propres au système (boîtes de dialogue, navigateur, presse-papiers).
type Platform interface {
	// SaveFile propose d'enregistrer un fichier ; renvoie le chemin choisi ou ErrCancelled.
	// Un chemin vide sans erreur signifie que l'interface doit proposer le téléchargement elle-même.
	SaveFile(suggestedName string, content []byte) (string, error)
	OpenURL(url string) error
	ReadClipboard() (string, error)
}

// Options configure le service.
type Options struct {
	Version  string
	Store    *config.Store
	Platform Platform
	// Prober et NetState sont remplaçables pour les tests.
	Prober   status.Prober
	NetState func() (netstate.State, error)
	// NetPoll est l'intervalle de vérification du réseau local.
	NetPoll time.Duration
	// History enregistre l'historique (nil : en mémoire seulement, pour les tests).
	History *history.Store
	// Now est l'horloge (remplaçable pour les tests).
	Now func() time.Time
}

// Service est le cœur de l'application.
type Service struct {
	version  string
	store    *config.Store
	platform Platform
	netState func() (netstate.State, error)
	netPoll  time.Duration
	agent    *agentclient.Client
	monitor  *status.Monitor

	mu             sync.Mutex
	cfg            model.AppConfig
	network        netstate.State
	windowActive   bool
	pageVisible    bool
	startupMessage string
	pendingImport  *importState
	listener       func()

	now func() time.Time
	// Historique (verrou distinct : jamais pris en même temps que mu, toujours après statusMu).
	histMu      sync.Mutex
	histStore   *history.Store
	hist        history.Data
	histVersion int
	agentFetch  map[string]*agentFetchState
	// Dernier relevé d'états, pour détecter les allumages / extinctions.
	statusMu     sync.Mutex
	lastStatuses map[string]status.DeviceStatus
}

type importState struct {
	text   []byte
	config *model.AppConfig
}

// New crée le service et charge la configuration.
func New(opts Options) *Service {
	s := &Service{
		version:      opts.Version,
		store:        opts.Store,
		platform:     opts.Platform,
		netState:     opts.NetState,
		netPoll:      opts.NetPoll,
		agent:        agentclient.New(),
		windowActive: true,
		pageVisible:  true,
		listener:     func() {},
		now:          opts.Now,
		histStore:    opts.History,
		hist:         history.New(),
		agentFetch:   map[string]*agentFetchState{},
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.histStore != nil {
		s.hist = s.histStore.Load()
	}
	if s.netState == nil {
		s.netState = netstate.Current
	}
	if s.netPoll <= 0 {
		s.netPoll = 2 * time.Second
	}
	prober := opts.Prober
	if prober == nil {
		prober = status.NewHostProber()
	}
	s.monitor = status.NewMonitor(prober, func() int64 { return s.now().UnixMilli() }, s.onStatusChange)
	cfg, err := s.store.Load()
	if err != nil {
		s.startupMessage = "Erreur : " + err.Error()
	}
	s.cfg = cfg
	s.network, _ = s.netState()
	return s
}

// OnChange enregistre la fonction appelée à chaque changement d'état (à relire avec State).
func (s *Service) OnChange(f func()) {
	s.mu.Lock()
	s.listener = f
	s.mu.Unlock()
}

func (s *Service) notify() {
	s.mu.Lock()
	f := s.listener
	s.mu.Unlock()
	f()
}

// Run lance la surveillance des PC et du réseau jusqu'à l'annulation de ctx.
func (s *Service) Run(ctx context.Context) {
	go s.monitor.Run(ctx)
	s.updateMonitor()
	ticker := time.NewTicker(s.netPoll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			state, err := s.netState()
			if err != nil {
				continue
			}
			s.mu.Lock()
			changed := state.Key() != s.network.Key()
			s.network = state
			s.mu.Unlock()
			if changed {
				s.updateMonitor()
				s.monitor.Refresh("")
				s.notify()
			}
		}
	}
}

// SetWindowActive met la surveillance en pause quand la fenêtre est réduite (comme Android en arrière-plan).
func (s *Service) SetWindowActive(active bool) {
	s.mu.Lock()
	changed := s.windowActive != active
	s.windowActive = active
	s.mu.Unlock()
	if changed {
		s.updateMonitor()
	}
}

func (s *Service) updateMonitor() {
	s.mu.Lock()
	cfg := s.cfg
	avail := status.Availability{CanProbe: true}
	if !s.network.Connected() && !s.network.VPNActive {
		avail = status.Availability{CanProbe: false, Reason: status.NoNetwork}
	}
	active := s.windowActive && s.pageVisible
	s.mu.Unlock()
	s.monitor.Update(cfg, avail, active)
}

// --- État envoyé à l'interface ---

// UIState est l'état complet affiché par l'interface.
type UIState struct {
	Version        string            `json:"version"`
	Network        NetworkView       `json:"network"`
	Settings       model.AppSettings `json:"settings"`
	Devices        []DeviceView      `json:"devices"`
	HasSecrets     bool              `json:"hasSecrets"`
	StartupMessage string            `json:"startupMessage,omitempty"`
	// HistoryVersion change à chaque modification de l'historique (l'interface le relit alors).
	HistoryVersion int `json:"historyVersion"`
}

// NetworkView décrit le réseau local.
type NetworkView struct {
	Connected bool   `json:"connected"`
	Transport string `json:"transport"`
	Address   string `json:"address"`
	VPN       bool   `json:"vpn"`
}

// DeviceView est un PC tel qu'affiché dans la liste (sans secret).
type DeviceView struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	MAC         string              `json:"mac"`
	Host        string              `json:"host"`
	HasAgent    bool                `json:"hasAgent"`
	CanShutdown bool                `json:"canShutdown"`
	Status      status.DeviceStatus `json:"status"`
	// Latency contient les mesures récentes (latencyView), pour le tracé en direct.
	Latency []status.LatencySample `json:"latency,omitempty"`
}

// latencyView est la durée des mesures de latence envoyées à l'interface : la minute tracée,
// plus une marge pour le défilement.
const latencyView = 75 * time.Second

// State renvoie l'état courant.
func (s *Service) State() UIState {
	statuses := s.monitor.Statuses()
	s.histMu.Lock()
	historyVersion := s.histVersion
	s.histMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	st := UIState{
		HistoryVersion: historyVersion,
		Version:        s.version,
		Settings:       s.cfg.Settings,
		Devices:        []DeviceView{},
		HasSecrets:     s.cfg.HasSecrets(),
		StartupMessage: s.startupMessage,
		Network:        NetworkView{Connected: s.network.Connected(), Address: s.network.PrimaryAddress(), VPN: s.network.VPNActive},
	}
	if p, ok := s.network.Primary(); ok {
		st.Network.Transport = string(p.Transport)
	}
	since := s.now().Add(-latencyView).UnixMilli()
	for _, d := range s.cfg.Devices {
		ds, ok := statuses[d.ID]
		if !ok {
			ds = status.DeviceStatus{State: status.Unknown}
		}
		st.Devices = append(st.Devices, DeviceView{
			ID: d.ID, Name: d.Name, MAC: d.MAC.String(), Host: d.Host,
			HasAgent: d.Agent != nil, CanShutdown: d.CanShutdown(), Status: ds,
			Latency: s.monitor.Latency(d.ID, since),
		})
	}
	return st
}

// --- Appels depuis l'interface ---

// Call exécute une méthode de l'interface ; params est un objet JSON.
func (s *Service) Call(method string, params json.RawMessage) (any, error) {
	if len(params) == 0 || string(params) == "null" {
		params = json.RawMessage("{}")
	}
	var p struct {
		ID         string           `json:"id"`
		Offset     int              `json:"offset"`
		Action     string           `json:"action"`
		Force      bool             `json:"force"`
		Form       model.DeviceForm `json:"form"`
		Host       string           `json:"host"`
		Port       string           `json:"port"`
		Key        string           `json:"key"`
		Text       string           `json:"text"`
		Password   string           `json:"password"`
		Replace    bool             `json:"replace"`
		URL        string           `json:"url"`
		Visible    bool             `json:"visible"`
		Refresh    bool             `json:"refresh"`
		Poll       *int             `json:"pollIntervalSeconds"`
		WakeTO     *int             `json:"wakeTimeoutSeconds"`
		Confirm    *bool            `json:"confirmPowerActions"`
		WithSecret bool             `json:"withSecrets"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("paramètres invalides : %w", err)
	}
	switch method {
	case "getState":
		return s.State(), nil
	case "uiReady":
		// Signal de l'interface une fois affichée (utilisé par l'autotest de démarrage).
		return nil, nil
	case "dismissStartupMessage":
		s.mu.Lock()
		s.startupMessage = ""
		s.mu.Unlock()
		return nil, nil
	case "setVisible":
		s.mu.Lock()
		s.pageVisible = p.Visible
		s.mu.Unlock()
		s.updateMonitor()
		return nil, nil
	case "refresh":
		s.monitor.Refresh(p.ID)
		return nil, nil
	case "setLive":
		// PC affiché en détail (vide : aucun) : sondé chaque seconde pour le tracé de latence.
		s.monitor.SetLive(p.ID)
		return nil, nil
	case "getHistory":
		if p.Refresh {
			s.refreshAgentHistories(p.ID, true)
		}
		return s.historyView(p.ID), nil
	case "clearHistory":
		s.clearHistory()
		return nil, nil
	case "clearNotice":
		s.monitor.ClearNotice(p.ID)
		return nil, nil
	case "getDevice":
		return s.getDevice(p.ID), nil
	case "saveDevice":
		return s.saveDevice(p.ID, p.Form)
	case "deleteDevice":
		return nil, s.mutate(func(c model.AppConfig) model.AppConfig { return c.Remove(p.ID) })
	case "moveDevice":
		return nil, s.mutate(func(c model.AppConfig) model.AppConfig { return c.Move(p.ID, p.Offset) })
	case "wake":
		return s.wake(p.ID)
	case "power":
		return s.power(p.ID, agentclient.Action(p.Action), p.Force)
	case "testAgent":
		return s.testAgent(p.Host, p.Port, p.Key), nil
	case "parsePairing":
		info, err := pairing.Parse(p.Text)
		if err != nil {
			return map[string]any{"ok": false, "error": err.Error()}, nil
		}
		return map[string]any{"ok": true, "info": info}, nil
	case "readClipboard":
		text, err := s.platform.ReadClipboard()
		if err != nil {
			return map[string]any{"text": ""}, nil
		}
		return map[string]any{"text": text}, nil
	case "updateSettings":
		return nil, s.mutate(func(c model.AppConfig) model.AppConfig {
			if p.Poll != nil {
				c.Settings.PollIntervalSeconds = *p.Poll
			}
			if p.WakeTO != nil {
				c.Settings.WakeTimeoutSeconds = *p.WakeTO
			}
			if p.Confirm != nil {
				c.Settings.ConfirmPowerActions = *p.Confirm
			}
			c.Settings = c.Settings.Clamped()
			return c
		})
	case "exportConfig":
		return s.exportConfig(p.WithSecret, p.Password)
	case "importFile":
		return s.importFile([]byte(p.Text))
	case "importPassword":
		return s.importPassword(p.Password)
	case "importConfirm":
		return s.importConfirm(p.Replace)
	case "importCancel":
		s.mu.Lock()
		s.pendingImport = nil
		s.mu.Unlock()
		return nil, nil
	case "openUrl":
		if p.URL != RepoURL && !strings.HasPrefix(p.URL, RepoURL+"/") {
			return nil, errors.New("adresse non autorisée")
		}
		return nil, s.platform.OpenURL(p.URL)
	}
	return nil, fmt.Errorf("méthode inconnue : %s", method)
}

// mutate applique une modification, la valide, l'enregistre puis relance la surveillance.
func (s *Service) mutate(f func(model.AppConfig) model.AppConfig) error {
	s.mu.Lock()
	next, err := config.Sanitize(f(s.cfg.Clone()))
	if err == nil {
		err = s.store.Save(next)
	}
	if err == nil {
		s.cfg = next
	}
	s.mu.Unlock()
	if err != nil {
		return err
	}
	ids := map[string]bool{}
	for _, d := range next.Devices {
		ids[d.ID] = true
	}
	s.keepHistory(ids)
	s.updateMonitor()
	s.notify()
	return nil
}

func (s *Service) device(id string) (model.Device, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg.Device(id)
}

func (s *Service) getDevice(id string) map[string]any {
	if d, ok := s.device(id); ok {
		return map[string]any{"isNew": false, "form": model.FormFrom(d)}
	}
	return map[string]any{"isNew": true, "form": model.NewForm()}
}

func (s *Service) saveDevice(id string, form model.DeviceForm) (any, error) {
	if _, ok := s.device(id); !ok {
		id = ""
	}
	d, errs := form.Build(id)
	if errs != nil {
		return map[string]any{"ok": false, "errors": errs}, nil
	}
	if err := s.mutate(func(c model.AppConfig) model.AppConfig { return c.Upsert(d) }); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "id": d.ID}, nil
}

func (s *Service) wake(id string) (any, error) {
	d, ok := s.device(id)
	if !ok {
		return nil, errors.New("PC introuvable")
	}
	var password []byte
	if d.SecureOnPassword != nil {
		var err error
		if password, err = model.SecureOnPasswordBytes(*d.SecureOnPassword); err != nil {
			return map[string]any{"ok": false, "error": err.Error()}, nil
		}
	}
	packet, err := wol.BuildPacket(d.MAC, password)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	req := wol.Request{Packet: packet, Port: d.WolPort}
	if d.BroadcastAddress != nil {
		req.Override, _ = netip.ParseAddr(*d.BroadcastAddress)
	}
	s.mu.Lock()
	for _, prefix := range s.network.Prefixes() {
		req.Routes = append(req.Routes, wol.Route{Source: prefix})
	}
	s.mu.Unlock()
	result := wol.Send(context.Background(), req)
	if !result.Success() {
		msg := "aucun paquet envoyé"
		if len(result.Errors) > 0 {
			msg = result.Errors[0]
		}
		return map[string]any{"ok": false, "error": msg}, nil
	}
	// Demande enregistrée avant le changement d'état : l'allumage constaté ne peut pas la précéder.
	s.record(history.Event{Device: id, Kind: history.WakeSent, Source: history.App})
	s.monitor.OnWakeSent(id)
	return map[string]any{"ok": true, "result": result}, nil
}

func (s *Service) power(id string, action agentclient.Action, force bool) (any, error) {
	d, ok := s.device(id)
	if !ok {
		return nil, errors.New("PC introuvable")
	}
	if !action.Valid() {
		return nil, errors.New("action inconnue")
	}
	if d.Agent == nil || !d.Agent.HasKey() || !d.HasHost() {
		return map[string]any{"ok": false, "code": agentclient.NoKey}, nil
	}
	if _, err := s.agent.Power(context.Background(), d.Host, *d.Agent, action, 0, force); err != nil {
		return map[string]any{"ok": false, "code": agentclient.CodeOf(err)}, nil
	}
	s.record(history.Event{Device: id, Kind: requestKind(action), Source: history.App})
	s.monitor.OnPowerActionSent(id, action)
	return map[string]any{"ok": true}, nil
}

func (s *Service) testAgent(host, portText, key string) any {
	host = strings.TrimSpace(host)
	var port int
	_, err := fmt.Sscan(strings.TrimSpace(portText), &port)
	if !model.IsValidHost(host) || err != nil || !model.IsValidPort(port) {
		return map[string]any{"ok": false, "invalid": "Renseignez une adresse et un port valides"}
	}
	st, err := s.agent.Status(context.Background(), host, model.AgentSettings{Port: port, Key: strings.TrimSpace(key)})
	if err != nil {
		return map[string]any{"ok": false, "code": agentclient.CodeOf(err)}
	}
	return map[string]any{"ok": true, "status": st}
}

// --- Sauvegarde ---

func (s *Service) exportConfig(withSecrets bool, password string) (any, error) {
	s.mu.Lock()
	cfg := s.cfg.Clone()
	s.mu.Unlock()
	if !withSecrets {
		password = ""
	} else if model.UTF16Len(password) < config.MinPasswordLength {
		return nil, errors.New("Mot de passe trop court")
	}
	now := time.Now()
	text, err := config.Export(cfg, config.ExportOptions{
		Password:   password,
		ExportedAt: now.UTC().Format("2006-01-02T15:04:05.000Z"),
		App:        "WakeOnLan Windows " + s.version,
	})
	if err != nil {
		return nil, err
	}
	name := "wakeonlan-" + now.Format("2006-01-02") + ".json"
	path, err := s.platform.SaveFile(name, text)
	if errors.Is(err, ErrCancelled) {
		return map[string]any{"cancelled": true}, nil
	}
	if err != nil {
		return nil, err
	}
	result := map[string]any{"ok": true, "count": len(cfg.Devices), "path": path}
	if path == "" {
		result["download"] = map[string]any{"name": name, "content": string(text)}
	}
	return result, nil
}

func (s *Service) importFile(text []byte) (any, error) {
	if len(text) > config.MaxImportBytes {
		return nil, errors.New("Fichier trop volumineux")
	}
	encrypted, err := config.IsEncrypted(text)
	if err != nil {
		return nil, err
	}
	if encrypted {
		s.mu.Lock()
		s.pendingImport = &importState{text: text}
		s.mu.Unlock()
		return map[string]any{"step": "password"}, nil
	}
	cfg, err := config.Import(text, "")
	if err != nil {
		return nil, err
	}
	return s.confirmStep(cfg), nil
}

func (s *Service) importPassword(password string) (any, error) {
	s.mu.Lock()
	pending := s.pendingImport
	s.mu.Unlock()
	if pending == nil {
		return nil, errors.New("aucun import en cours")
	}
	cfg, err := config.Import(pending.text, password)
	var cfgErr *config.Error
	if errors.As(err, &cfgErr) && cfgErr.Reason == config.WrongPassword {
		return map[string]any{"step": "password", "wrongPassword": true}, nil
	}
	if err != nil {
		s.mu.Lock()
		s.pendingImport = nil
		s.mu.Unlock()
		return nil, err
	}
	return s.confirmStep(cfg), nil
}

func (s *Service) confirmStep(cfg model.AppConfig) map[string]any {
	missingKeys := false
	for _, d := range cfg.Devices {
		missingKeys = missingKeys || (d.Agent != nil && !d.Agent.HasKey())
	}
	s.mu.Lock()
	s.pendingImport = &importState{config: &cfg}
	s.mu.Unlock()
	return map[string]any{"step": "confirm", "count": len(cfg.Devices), "missingKeys": missingKeys}
}

func (s *Service) importConfirm(replace bool) (any, error) {
	s.mu.Lock()
	pending := s.pendingImport
	s.pendingImport = nil
	s.mu.Unlock()
	if pending == nil || pending.config == nil {
		return nil, errors.New("aucun import en cours")
	}
	imported := *pending.config
	err := s.mutate(func(c model.AppConfig) model.AppConfig {
		if replace {
			// Remplacer : les PC de la sauvegarde, en conservant les réglages de la sauvegarde.
			return imported
		}
		return c.MergeDevicesFrom(imported)
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "count": len(imported.Devices)}, nil
}

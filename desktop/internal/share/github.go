package share

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Stockage des fichiers d'accès : un « Gist » secret du compte GitHub de la personne qui partage.
//
//   - Connexion : flux « appareil » d'OAuth (l'utilisateur tape un code sur github.com), droit « gist »
//     seulement : l'application ne peut ni lire ni modifier les dépôts.
//   - Lecture : API publique, sans compte (le contenu est chiffré), avec ETag pour ménager la limite
//     de requêtes de GitHub.

// GitHub est un client minimal de l'API GitHub.
type GitHub struct {
	// API et Web : https://api.github.com et https://github.com (remplaçables pour les tests).
	API string
	Web string
	// ClientID est l'identifiant (public) de l'application OAuth enregistrée sur GitHub.
	ClientID string
	Client   *http.Client
	// UserAgent identifie l'application auprès de GitHub.
	UserAgent string
	// PollUnit est l'unité des délais d'attente de la connexion (0 : 1 s ; raccourcie dans les tests).
	PollUnit time.Duration
}

// Erreurs de l'API.
var (
	ErrNotFound     = errors.New("espace de partage introuvable")
	ErrUnauthorized = errors.New("connexion GitHub expirée ou révoquée : reconnectez-vous")
	ErrRateLimited  = errors.New("GitHub limite temporairement les requêtes : réessayez dans quelques minutes")
	ErrDenied       = errors.New("connexion refusée sur GitHub")
	ErrExpired      = errors.New("code expiré : recommencez la connexion")
	ErrNoClientID   = errors.New("connexion GitHub non configurée dans cette version")
)

const maxResponse = 1 << 20

// transientError est un échec passager (Internet coupé, GitHub momentanément indisponible, réponse
// tronquée) : l'opération peut être retentée.
type transientError struct{ msg string }

func (e *transientError) Error() string { return e.msg }

// IsTransient indique un échec passager. Les réponses refusées par GitHub (4xx) ne le sont pas.
func IsTransient(err error) bool {
	var t *transientError
	return errors.As(err, &t)
}

// statusError décrit une réponse d'erreur, passagère si GitHub est indisponible (5xx) ou surchargé (429).
func statusError(code int) error {
	msg := fmt.Sprintf("GitHub a répondu %d", code)
	if code >= 500 || code == http.StatusTooManyRequests {
		return &transientError{msg}
	}
	return errors.New(msg)
}

var errUnreadable = &transientError{"réponse de GitHub illisible"}

// retryAttempts borne les tentatives d'une étape de la connexion (voir Retry).
const retryAttempts = 5

func (g *GitHub) unit() time.Duration {
	if g.PollUnit == 0 {
		return time.Second
	}
	return g.PollUnit
}

// NewGitHub renvoie un client pour github.com.
func NewGitHub(clientID, userAgent string) *GitHub {
	return &GitHub{
		API: "https://api.github.com", Web: "https://github.com", ClientID: clientID,
		Client: &http.Client{Timeout: 20 * time.Second}, UserAgent: userAgent,
	}
}

// DeviceCode est un code de connexion à saisir sur github.com.
type DeviceCode struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// StartLogin demande un code de connexion (droit « gist » uniquement).
func (g *GitHub) StartLogin(ctx context.Context) (DeviceCode, error) {
	if g.ClientID == "" {
		return DeviceCode{}, ErrNoClientID
	}
	var dc DeviceCode
	err := g.form(ctx, g.Web+"/login/device/code", url.Values{"client_id": {g.ClientID}, "scope": {"gist"}}, &dc)
	if err != nil {
		return DeviceCode{}, err
	}
	if dc.DeviceCode == "" || dc.UserCode == "" {
		return DeviceCode{}, errors.New("réponse de GitHub inattendue")
	}
	if dc.Interval <= 0 {
		dc.Interval = 5
	}
	if dc.VerificationURI == "" {
		dc.VerificationURI = g.Web + "/login/device"
	}
	return dc, nil
}

// WaitLogin attend que l'utilisateur valide le code sur github.com et renvoie le jeton d'accès.
// Les échecs passagers (Internet coupé pendant la validation…) sont ignorés tant que le code est valable.
func (g *GitHub) WaitLogin(ctx context.Context, dc DeviceCode) (string, error) {
	unit := g.unit()
	interval := time.Duration(dc.Interval) * unit
	deadline := time.Now().Add(time.Duration(max(dc.ExpiresIn, 60)) * unit)
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(interval):
		}
		var r struct {
			AccessToken string `json:"access_token"`
			Scope       string `json:"scope"`
			Error       string `json:"error"`
			Interval    int    `json:"interval"`
		}
		err := g.form(ctx, g.Web+"/login/oauth/access_token", url.Values{
			"client_id":   {g.ClientID},
			"device_code": {dc.DeviceCode},
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		}, &r)
		if err != nil {
			if !IsTransient(err) || time.Now().After(deadline) {
				return "", err
			}
			continue
		}
		switch r.Error {
		case "":
			if r.AccessToken == "" {
				return "", errors.New("réponse de GitHub inattendue")
			}
			return r.AccessToken, nil
		case "authorization_pending":
		case "slow_down":
			interval += 5 * unit
			if r.Interval > 0 {
				interval = time.Duration(r.Interval) * unit
			}
		case "access_denied":
			return "", ErrDenied
		case "expired_token":
			return "", ErrExpired
		default:
			return "", fmt.Errorf("connexion GitHub impossible (%s)", r.Error)
		}
		if time.Now().After(deadline) {
			return "", ErrExpired
		}
	}
}

// Retry exécute une étape de la connexion et la réessaie, à intervalle croissant, tant que l'échec est
// passager (retryAttempts tentatives au plus).
func (g *GitHub) Retry(ctx context.Context, step func() error) error {
	for attempt := 1; ; attempt++ {
		err := step()
		if err == nil || !IsTransient(err) || attempt >= retryAttempts {
			return err
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(time.Duration(attempt) * 5 * g.unit()):
		}
	}
}

// User renvoie l'identifiant GitHub du compte connecté.
func (g *GitHub) User(ctx context.Context, token string) (string, error) {
	var u struct {
		Login string `json:"login"`
	}
	if _, err := g.api(ctx, http.MethodGet, "/user", token, "", nil, &u); err != nil {
		return "", err
	}
	if !loginPattern.MatchString(u.Login) {
		return "", errors.New("réponse de GitHub inattendue")
	}
	return u.Login, nil
}

type gistFile struct {
	Content string `json:"content"`
}

// CreateGist crée un Gist secret et renvoie son identifiant.
func (g *GitHub) CreateGist(ctx context.Context, token, description string, files map[string]string) (string, error) {
	body := struct {
		Description string              `json:"description"`
		Public      bool                `json:"public"`
		Files       map[string]gistFile `json:"files"`
	}{Description: description, Files: map[string]gistFile{}}
	for name, content := range files {
		body.Files[name] = gistFile{Content: content}
	}
	var r struct {
		ID string `json:"id"`
	}
	if _, err := g.api(ctx, http.MethodPost, "/gists", token, "", body, &r); err != nil {
		return "", err
	}
	if !gistPattern.MatchString(r.ID) {
		return "", errors.New("réponse de GitHub inattendue")
	}
	return r.ID, nil
}

// UpdateGist ajoute, remplace (contenu) ou supprime (nil) des fichiers d'un Gist.
func (g *GitHub) UpdateGist(ctx context.Context, token, id string, files map[string]*string) error {
	if !gistPattern.MatchString(id) {
		return ErrNotFound
	}
	body := struct {
		Files map[string]*gistFile `json:"files"`
	}{Files: map[string]*gistFile{}}
	for name, content := range files {
		if content == nil {
			body.Files[name] = nil
		} else {
			body.Files[name] = &gistFile{Content: *content}
		}
	}
	_, err := g.api(ctx, http.MethodPatch, "/gists/"+id, token, "", body, nil)
	return err
}

// DeleteGist supprime un Gist.
func (g *GitHub) DeleteGist(ctx context.Context, token, id string) error {
	if !gistPattern.MatchString(id) {
		return ErrNotFound
	}
	_, err := g.api(ctx, http.MethodDelete, "/gists/"+id, token, "", nil, nil)
	return err
}

// Snapshot est le contenu d'un Gist lu sans compte.
type Snapshot struct {
	// NotModified : rien n'a changé depuis etag (Files est alors vide).
	NotModified bool
	ETag        string
	Files       map[string]string
}

// FetchGist lit les fichiers d'un Gist (sans jeton : le contenu est chiffré).
func (g *GitHub) FetchGist(ctx context.Context, id, etag string) (Snapshot, error) {
	if !gistPattern.MatchString(id) {
		return Snapshot{}, ErrNotFound
	}
	var r struct {
		Files map[string]struct {
			Content   string `json:"content"`
			Truncated bool   `json:"truncated"`
			RawURL    string `json:"raw_url"`
			Size      int    `json:"size"`
		} `json:"files"`
	}
	resp, err := g.api(ctx, http.MethodGet, "/gists/"+id, "", etag, nil, &r)
	if err != nil {
		return Snapshot{}, err
	}
	if resp.StatusCode == http.StatusNotModified {
		return Snapshot{NotModified: true, ETag: etag}, nil
	}
	snap := Snapshot{ETag: resp.Header.Get("ETag"), Files: map[string]string{}}
	for name, f := range r.Files {
		if !strings.HasPrefix(name, "acces-") {
			continue
		}
		content := f.Content
		if f.Truncated {
			if f.Size > MaxFileSize {
				continue
			}
			if content, err = g.raw(ctx, f.RawURL, MaxFileSize); err != nil {
				return Snapshot{}, err
			}
		}
		snap.Files[name] = content
	}
	return snap, nil
}

// raw télécharge un fichier tronqué par l'API (seulement depuis le domaine des Gists), limit octets au plus.
func (g *GitHub) raw(ctx context.Context, raw string, limit int) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "gist.githubusercontent.com" {
		return "", errors.New("adresse de fichier inattendue")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", g.userAgent())
	resp, err := g.Client.Do(req)
	if err != nil {
		return "", networkError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", statusError(resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, int64(limit)+1))
	if err != nil {
		return "", networkError(err)
	}
	return string(data), nil
}

func (g *GitHub) userAgent() string {
	if g.UserAgent == "" {
		return "Patronus"
	}
	return g.UserAgent
}

// form envoie un formulaire (connexion OAuth) et décode la réponse JSON.
func (g *GitHub) form(ctx context.Context, endpoint string, values url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", g.userAgent())
	resp, err := g.Client.Do(req)
	if err != nil {
		return networkError(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse))
	if err != nil {
		return networkError(err)
	}
	if resp.StatusCode != http.StatusOK {
		return statusError(resp.StatusCode)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return errUnreadable
	}
	return nil
}

// api appelle l'API REST ; body est encodé en JSON, la réponse décodée dans out (si non nil).
func (g *GitHub) api(ctx context.Context, method, path, token, etag string, body, out any) (*http.Response, error) {
	return g.apiLimit(ctx, method, path, token, etag, body, out, maxResponse)
}

// apiLimit : comme api, avec une réponse de limit octets au plus.
func (g *GitHub) apiLimit(ctx context.Context, method, path, token, etag string, body, out any, limit int) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, g.API+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", g.userAgent())
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	resp, err := g.Client.Do(req)
	if err != nil {
		return nil, networkError(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, int64(limit)))
	if err != nil {
		return nil, networkError(err)
	}
	switch {
	case resp.StatusCode == http.StatusNotModified:
		return resp, nil
	case resp.StatusCode == http.StatusNotFound:
		return nil, ErrNotFound
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, ErrUnauthorized
	case resp.StatusCode == http.StatusTooManyRequests,
		resp.StatusCode == http.StatusForbidden && (token == "" || resp.Header.Get("X-RateLimit-Remaining") == "0"):
		// Sans jeton, un refus ne peut venir que de la limite de requêtes.
		return nil, ErrRateLimited
	case resp.StatusCode == http.StatusForbidden:
		return nil, ErrUnauthorized
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		return nil, statusError(resp.StatusCode)
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return nil, errUnreadable
		}
	}
	return resp, nil
}

// networkError rend lisible une erreur réseau.
func networkError(err error) error {
	if errors.Is(err, context.Canceled) {
		return err
	}
	return &transientError{"GitHub injoignable : vérifiez la connexion Internet"}
}

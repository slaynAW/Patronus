package share

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
)

// Fonctions des Gists utilisées par les sauvegardes automatiques (Gist secret du compte, commun aux
// appareils de l'utilisateur ; contenu chiffré par le mot de passe des sauvegardes).

// maxGistResponse : taille maximale d'une réponse listant un Gist de sauvegardes (plusieurs fichiers).
const maxGistResponse = 32 << 20

// FindGist renvoie l'identifiant du Gist du compte portant cette description ("" s'il n'y en a pas).
func (g *GitHub) FindGist(ctx context.Context, token, description string) (string, error) {
	for page := 1; page <= 5; page++ {
		var gists []struct {
			ID          string `json:"id"`
			Description string `json:"description"`
		}
		path := fmt.Sprintf("/gists?per_page=100&page=%d", page)
		if _, err := g.apiLimit(ctx, http.MethodGet, path, token, "", nil, &gists, 8<<20); err != nil {
			return "", err
		}
		for _, gist := range gists {
			if gist.Description == description && gistPattern.MatchString(gist.ID) {
				return gist.ID, nil
			}
		}
		if len(gists) < 100 {
			break
		}
	}
	return "", nil
}

// GistFile est un fichier d'un Gist.
type GistFile struct {
	Name    string
	Size    int
	Content string
}

// ReadGist lit tous les fichiers d'un Gist avec le jeton (fichiers tronqués par l'API relus en entier,
// limit octets au plus chacun), triés par nom.
func (g *GitHub) ReadGist(ctx context.Context, token, id string, limit int) ([]GistFile, error) {
	if !gistPattern.MatchString(id) {
		return nil, ErrNotFound
	}
	var r struct {
		Files map[string]struct {
			Content   string `json:"content"`
			Truncated bool   `json:"truncated"`
			RawURL    string `json:"raw_url"`
			Size      int    `json:"size"`
		} `json:"files"`
	}
	if _, err := g.apiLimit(ctx, http.MethodGet, "/gists/"+id, token, "", nil, &r, maxGistResponse); err != nil {
		return nil, err
	}
	var out []GistFile
	for name, f := range r.Files {
		if f.Size > limit {
			continue
		}
		content := f.Content
		if f.Truncated {
			var err error
			if content, err = g.raw(ctx, f.RawURL, limit); err != nil {
				return nil, err
			}
		}
		out = append(out, GistFile{Name: name, Size: f.Size, Content: content})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// ErrNoGist signale qu'aucun Gist de sauvegardes n'existe encore sur ce compte.
var ErrNoGist = errors.New("aucune sauvegarde sur ce compte GitHub")

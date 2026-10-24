// Package fleet tient la liste des PC confiés au hub et des appareils sans agent (docs/HUB.md, §5.3 et
// §5.4), dans hub.db, fichier entier du stockage chiffré. Comme le registre des appareils, une liste
// illisible est une erreur, jamais remplacée par une liste vide.
package fleet

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/slaynaw/wakeonlan/agent/hubprotocol"
	"github.com/slaynaw/wakeonlan/agent/protocol2"
)

// FileName est la liste, dans le stockage chiffré du hub.
const FileName = "hub.db"

var (
	ErrUnknownPC = errors.New("PC inconnu du hub")
	ErrTooMany   = fmt.Errorf("%d PC et appareils au plus", hubprotocol.MaxPCs)
	ErrDuplicate = errors.New("ce PC est déjà confié au hub")
)

// Storage garde la liste (store.File en service).
type Storage interface {
	Load() ([]byte, error)
	Save(data []byte) error
}

// PC est un PC confié, ou un appareil sans agent (Agent nul, Probe renseignée).
type PC struct {
	ID    string             `json:"id"`
	Name  string             `json:"name"`
	Host  string             `json:"host"`
	Port  int                `json:"port,omitempty"`
	MAC   string             `json:"mac,omitempty"`
	Agent *protocol2.Pin     `json:"agent,omitempty"`
	Role  protocol2.Role     `json:"role,omitempty"` // rôle du hub sur l'agent
	Probe *hubprotocol.Probe `json:"probe,omitempty"`
	Added int64              `json:"added"`
}

// Info décrit le PC dans les réponses du hub.
func (p PC) Info() hubprotocol.PCInfo {
	info := hubprotocol.PCInfo{ID: p.ID, Name: p.Name, Host: p.Host, Port: p.Port, MAC: p.MAC, Probe: p.Probe, Added: p.Added}
	if p.Agent != nil {
		info.Agent = p.Agent.String()
		info.Role = p.Role.String()
	}
	return info
}

type state struct {
	PCs []PC `json:"pcs"`
}

// Fleet est la liste, sûre pour un usage concurrent.
type Fleet struct {
	mu    sync.Mutex
	store Storage
	now   func() time.Time
	pcs   []PC
}

// Open lit la liste (vide si elle n'existe pas encore).
func Open(store Storage, now func() time.Time) (*Fleet, error) {
	f := &Fleet{store: store, now: now}
	raw, err := store.Load()
	if errors.Is(err, os.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return nil, err
	}
	var st state
	if err := json.Unmarshal(raw, &st); err != nil {
		return nil, fmt.Errorf("liste des PC du hub illisible : %w", err)
	}
	for _, p := range st.PCs {
		if err := validate(p); err != nil || p.ID == "" {
			return nil, fmt.Errorf("liste des PC du hub incohérente (%q) : %v", p.ID, err)
		}
	}
	f.pcs = st.PCs
	return f, nil
}

// List renvoie une copie de la liste, dans l'ordre d'ajout.
func (f *Fleet) List() []PC {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.pcs)
}

// Get cherche un PC par son identifiant.
func (f *Fleet) Get(id string) (PC, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if i := f.index(id); i >= 0 {
		return f.pcs[i], true
	}
	return PC{}, false
}

// Add ajoute un PC (son identifiant et sa date d'ajout sont attribués ici).
func (f *Fleet) Add(p PC) (PC, error) {
	if err := validate(p); err != nil {
		return PC{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.pcs) >= hubprotocol.MaxPCs {
		return PC{}, ErrTooMany
	}
	for _, other := range f.pcs {
		if p.Agent != nil && other.Agent != nil && *other.Agent == *p.Agent {
			return PC{}, ErrDuplicate
		}
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return PC{}, err
	}
	p.ID, p.Added = base64.RawURLEncoding.EncodeToString(id), f.now().Unix()
	next := append(slices.Clone(f.pcs), p)
	return p, f.commit(next)
}

// Update modifie un PC ; change ne peut toucher ni à l'identifiant, ni à l'agent, ni à la date d'ajout.
func (f *Fleet) Update(id string, change func(*PC)) (PC, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := f.index(id)
	if i < 0 {
		return PC{}, ErrUnknownPC
	}
	p := f.pcs[i]
	change(&p)
	p.ID, p.Agent, p.Added = f.pcs[i].ID, f.pcs[i].Agent, f.pcs[i].Added
	if err := validate(p); err != nil {
		return PC{}, err
	}
	next := slices.Clone(f.pcs)
	next[i] = p
	return p, f.commit(next)
}

// Remove retire un PC.
func (f *Fleet) Remove(id string) (PC, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := f.index(id)
	if i < 0 {
		return PC{}, ErrUnknownPC
	}
	p := f.pcs[i]
	return p, f.commit(slices.Delete(slices.Clone(f.pcs), i, i+1))
}

func (f *Fleet) index(id string) int {
	return slices.IndexFunc(f.pcs, func(p PC) bool { return p.ID == id })
}

// commit enregistre next puis l'adopte : en cas d'échec, rien ne change.
func (f *Fleet) commit(next []PC) error {
	if next == nil {
		next = []PC{}
	}
	raw, err := json.Marshal(state{PCs: next})
	if err != nil {
		return err
	}
	if err := f.store.Save(raw); err != nil {
		return err
	}
	f.pcs = next
	return nil
}

// validate vérifie un PC : nom, adresse, port, MAC, et soit un agent (rôle du hub : view, wake ou
// control, jamais admin), soit une sonde.
func validate(p PC) error {
	switch {
	case p.Name == "" || p.Name != protocol2.CleanName(p.Name, protocol2.MaxPCNameLength):
		return errors.New("nom invalide")
	case !protocol2.ValidHost(p.Host):
		return errors.New("adresse invalide")
	case p.Port < 0 || p.Port > 65535:
		return errors.New("port invalide")
	}
	if p.MAC != "" {
		if mac, ok := protocol2.NormalizeMAC(p.MAC); !ok || mac != p.MAC {
			return errors.New("adresse MAC invalide")
		}
	}
	if p.Agent != nil {
		if p.Probe != nil || p.Port == 0 {
			return errors.New("un PC avec agent a un port et pas de sonde")
		}
		if p.Role != protocol2.RoleView && p.Role != protocol2.RoleWake && p.Role != protocol2.RoleControl {
			return errors.New("rôle du hub sur l'agent invalide")
		}
		return nil
	}
	if p.Role != 0 || p.Port != 0 {
		return errors.New("un appareil sans agent n'a ni rôle ni port d'agent")
	}
	return ValidProbe(p.Probe)
}

// ValidProbe vérifie la sonde d'un appareil sans agent.
func ValidProbe(probe *hubprotocol.Probe) error {
	if probe == nil {
		return errors.New("sonde absente")
	}
	switch probe.Kind {
	case hubprotocol.ProbeTCP:
		if probe.Port < 1 || probe.Port > 65535 {
			return errors.New("port de la sonde invalide")
		}
	case hubprotocol.ProbeHTTP, hubprotocol.ProbeHTTPS:
		if probe.Port < 0 || probe.Port > 65535 {
			return errors.New("port de la sonde invalide")
		}
	default:
		return errors.New("sonde inconnue : tcp, http ou https")
	}
	return nil
}

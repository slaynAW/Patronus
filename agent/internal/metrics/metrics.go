// Package metrics enregistre en continu les températures et l'utilisation du PC : un relevé toutes
// les 10 s (5 s quand une application regarde le PC), résumé par minute (moyenne et maximum) et
// gardé protocol.MetricsDays jours, un fichier par jour (UTC) à côté de la configuration. Les
// applications le lisent (commande « metrics ») pour l'afficher et l'archiver ; il reste complet
// même quand elles sont fermées.
package metrics

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sync"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol"
)

const (
	// Interval : écart entre deux relevés.
	Interval = 10 * time.Second
	// ActiveInterval : écart entre deux relevés quand une application regarde le PC (état demandé
	// depuis moins de activeFor), pour un affichage plus vivant.
	ActiveInterval = 5 * time.Second
	activeFor      = 30 * time.Second
	// latestMaxAge : au-delà, le dernier relevé n'est plus joint à la réponse à « status ».
	latestMaxAge = 30 * time.Second
	// maxRows borne les lignes d'un jour (1 440 minutes, plus d'éventuelles corrections d'horloge).
	maxRows   = 2000
	dayLayout = "2006-01-02"
)

var dayFile = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})\.jsonl(\.gz)?$`)

// ValidDay indique si day est un jour au format « 2026-10-03 ».
func ValidDay(day string) bool {
	_, err := time.Parse(dayLayout, day)
	return err == nil && len(day) == len(dayLayout)
}

// Recorder relève et enregistre les mesures.
type Recorder struct {
	dir  string
	read func() protocol.Temperatures
	now  func() time.Time
	// OnError reçoit les erreurs d'écriture (journal de l'agent).
	OnError func(error)

	mu       sync.Mutex
	latest   protocol.Temperatures
	latestAt time.Time
	demand   time.Time
	current  bucket
	day      string // jour du fichier en cours d'écriture
}

// New prépare un enregistreur qui écrit dans dir ; read fait un relevé (une seconde environ).
func New(dir string, read func() protocol.Temperatures, now func() time.Time) *Recorder {
	return &Recorder{dir: dir, read: read, now: now}
}

// Run relève jusqu'à l'annulation de ctx, puis enregistre la minute en cours.
func (r *Recorder) Run(ctx context.Context) {
	r.tidy(r.now())
	for {
		start := time.Now()
		r.Add(r.now(), r.read())
		wait := r.interval() - time.Since(start)
		select {
		case <-ctx.Done():
			r.Flush()
			return
		case <-time.After(max(wait, time.Second)):
		}
	}
}

func (r *Recorder) interval() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.demand.IsZero() && r.now().Sub(r.demand) < activeFor {
		return ActiveInterval
	}
	return Interval
}

// Latest renvoie le dernier relevé, joint à la réponse à « status » (nil s'il est trop ancien ou
// vide) ; une demande rend les relevés plus fréquents pendant quelques dizaines de secondes.
func (r *Recorder) Latest() *protocol.Temperatures {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	r.demand = now
	if r.latestAt.IsZero() || now.Sub(r.latestAt) > latestMaxAge || r.latest == (protocol.Temperatures{}) {
		return nil
	}
	t := r.latest
	return &t
}

// Add ajoute un relevé fait à l'instant at ; la minute précédente est enregistrée dès qu'une
// nouvelle commence.
func (r *Recorder) Add(at time.Time, t protocol.Temperatures) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.latest, r.latestAt = t, at
	minute := at.Unix() - mod(at.Unix(), 60)
	if r.current.n > 0 && r.current.minute != minute {
		r.flushLocked()
	}
	if r.current.n == 0 {
		r.current = bucket{minute: minute}
	}
	r.current.add(t)
}

// Flush enregistre la minute en cours (arrêt de l'agent).
func (r *Recorder) Flush() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.flushLocked()
}

func (r *Recorder) flushLocked() {
	row, ok := r.current.row()
	r.current = bucket{}
	if !ok {
		return
	}
	day := time.Unix(row.T, 0).UTC().Format(dayLayout)
	if day != r.day {
		r.day = day
		r.tidy(time.Unix(row.T, 0))
	}
	if err := r.append(day, row); err != nil && r.OnError != nil {
		r.OnError(err)
	}
}

func (r *Recorder) append(day string, row protocol.MetricsRow) error {
	if err := os.MkdirAll(r.dir, 0o700); err != nil {
		return err
	}
	raw, err := json.Marshal(row)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(r.dir, day+".jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(append(raw, '\n'))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// tidy compresse les jours terminés et supprime ceux de plus de protocol.MetricsDays jours.
func (r *Recorder) tidy(now time.Time) {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return
	}
	today := now.UTC().Format(dayLayout)
	oldest := now.UTC().AddDate(0, 0, -(protocol.MetricsDays - 1)).Format(dayLayout)
	for _, e := range entries {
		m := dayFile.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		path := filepath.Join(r.dir, e.Name())
		switch {
		case m[1] < oldest:
			_ = os.Remove(path)
		case m[2] == "" && m[1] < today:
			if err := compress(path); err != nil && r.OnError != nil {
				r.OnError(err)
			}
		}
	}
}

// compress remplace un fichier « .jsonl » par sa version « .jsonl.gz ».
func compress(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(raw); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	tmp := path + ".gz.tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path+".gz"); err != nil {
		return err
	}
	return os.Remove(path)
}

// Days renvoie les jours enregistrés, du plus ancien au plus récent.
func (r *Recorder) Days() []string {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return []string{}
	}
	var days []string
	for _, e := range entries {
		if m := dayFile.FindStringSubmatch(e.Name()); m != nil && !slices.Contains(days, m[1]) {
			days = append(days, m[1])
		}
	}
	slices.Sort(days)
	if len(days) > protocol.MetricsDays {
		days = days[len(days)-protocol.MetricsDays:]
	}
	if days == nil {
		days = []string{}
	}
	return days
}

// Day renvoie les mesures d'un jour, minute par minute (vide si le jour n'est pas enregistré).
func (r *Recorder) Day(day string) ([]protocol.MetricsRow, error) {
	if !ValidDay(day) {
		return nil, errors.New("jour invalide")
	}
	var in io.Reader
	path := filepath.Join(r.dir, day+".jsonl")
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		f, err = os.Open(path + ".gz")
		if errors.Is(err, fs.ErrNotExist) {
			return []protocol.MetricsRow{}, nil
		}
		if err != nil {
			return nil, err
		}
		defer f.Close()
		zr, err := gzip.NewReader(f)
		if err != nil {
			return nil, err
		}
		in = zr
	} else if err != nil {
		return nil, err
	} else {
		defer f.Close()
		in = f
	}
	rows := map[int64]protocol.MetricsRow{}
	scanner := bufio.NewScanner(io.LimitReader(in, 16<<20))
	for scanner.Scan() {
		var row protocol.MetricsRow
		// Ligne abîmée (arrêt brutal pendant l'écriture) : ignorée.
		if json.Unmarshal(scanner.Bytes(), &row) == nil && row.T > 0 && row.N > 0 {
			rows[row.T] = row
		}
	}
	out := make([]protocol.MetricsRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, row)
	}
	slices.SortFunc(out, func(a, b protocol.MetricsRow) int { return int(a.T - b.T) })
	if len(out) > maxRows {
		out = out[len(out)-maxRows:]
	}
	return out, nil
}

// --- Résumé d'une minute ---

type acc struct {
	sum, max float64
	n        int
}

func (a *acc) add(v *float64) {
	if v == nil || math.IsNaN(*v) {
		return
	}
	a.sum += *v
	if a.n == 0 || *v > a.max {
		a.max = *v
	}
	a.n++
}

func (a acc) values() (avg, maxValue *float64) {
	if a.n == 0 {
		return nil, nil
	}
	return round1(a.sum / float64(a.n)), round1(a.max)
}

type bucket struct {
	minute                 int64
	n                      int
	cpuT, gpuT, cpuL, gpuL acc
}

func (b *bucket) add(t protocol.Temperatures) {
	b.n++
	b.cpuT.add(t.CPU)
	b.gpuT.add(t.GPU)
	b.cpuL.add(t.CPULoad)
	b.gpuL.add(t.GPULoad)
}

// row résume la minute ; ok est faux si elle ne contient aucune mesure.
func (b bucket) row() (protocol.MetricsRow, bool) {
	row := protocol.MetricsRow{T: b.minute, N: b.n}
	row.CPUTemp, row.CPUTempMax = b.cpuT.values()
	row.GPUTemp, row.GPUTempMax = b.gpuT.values()
	row.CPULoad, row.CPULoadMax = b.cpuL.values()
	row.GPULoad, row.GPULoadMax = b.gpuL.values()
	ok := b.n > 0 && (row.CPUTemp != nil || row.GPUTemp != nil || row.CPULoad != nil || row.GPULoad != nil)
	return row, ok
}

func round1(v float64) *float64 {
	r := math.Round(v*10) / 10
	return &r
}

func mod(a, b int64) int64 { return (a%b + b) % b }

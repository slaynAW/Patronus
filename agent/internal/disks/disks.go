// Package disks lit les disques du PC : espace des lecteurs, santé des disques physiques (SSD et
// disques durs : état, température, usure, heures de fonctionnement, erreurs non corrigées) et
// erreurs d'accès signalées par Windows. Les relevés se font en arrière-plan (Run) : l'état du PC
// répond toujours tout de suite, même si le système tarde à répondre.
package disks

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol"
)

const (
	volumesEvery = 30 * time.Second
	drivesEvery  = 10 * time.Minute
	errorsEvery  = 30 * time.Minute
	tick         = 5 * time.Second
)

// Reader garde le dernier relevé des disques.
type Reader struct {
	now         func() time.Time
	readVolumes func() ([]protocol.Volume, error)
	readDrives  func() ([]protocol.Drive, error)
	readErrors  func(since time.Time) (int, time.Time, error)
	// OnError reçoit les erreurs de lecture (une fois par type d'erreur et par minute au plus).
	OnError func(what string, err error)

	mu       sync.Mutex
	volumes  []protocol.Volume
	drives   []protocol.Drive
	errors   int
	lastErr  time.Time
	read     bool
	volAt    time.Time
	drivesAt time.Time
	errsAt   time.Time
}

// New crée un lecteur des disques de ce PC.
func New() *Reader {
	return &Reader{now: time.Now, readVolumes: volumes, readDrives: drives, readErrors: diskErrors}
}

// Run relève les disques jusqu'à l'annulation de ctx.
func (r *Reader) Run(ctx context.Context) {
	for {
		r.Refresh()
		select {
		case <-ctx.Done():
			return
		case <-time.After(tick):
		}
	}
}

// Refresh fait les relevés dus.
func (r *Reader) Refresh() {
	now := r.now()
	r.mu.Lock()
	dueVol := r.volAt.IsZero() || now.Sub(r.volAt) >= volumesEvery
	dueDrives := r.drivesAt.IsZero() || now.Sub(r.drivesAt) >= drivesEvery
	dueErrs := r.errsAt.IsZero() || now.Sub(r.errsAt) >= errorsEvery
	r.mu.Unlock()
	if dueVol {
		v, err := r.readVolumes()
		r.mu.Lock()
		r.volAt = now
		if err == nil {
			r.volumes, r.read = v, true
		}
		r.mu.Unlock()
		r.report("lecteurs", err)
	}
	if dueDrives {
		d, err := r.readDrives()
		r.mu.Lock()
		r.drivesAt = now
		if err == nil {
			r.drives, r.read = d, true
		}
		r.mu.Unlock()
		r.report("santé des disques", err)
	}
	if dueErrs {
		n, last, err := r.readErrors(now.Add(-protocol.DiskErrorDays * 24 * time.Hour))
		r.mu.Lock()
		r.errsAt = now
		if err == nil {
			r.errors, r.lastErr = n, last
		}
		r.mu.Unlock()
		if err != nil && err != errUnsupported {
			r.report("erreurs de disque", err)
		}
	}
}

func (r *Reader) report(what string, err error) {
	if err != nil && err != errUnsupported && r.OnError != nil {
		r.OnError(what, err)
	}
}

// Read renvoie le dernier relevé (nil tant qu'aucun n'a réussi).
func (r *Reader) Read() *protocol.Disks {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.read {
		return nil
	}
	d := &protocol.Disks{Volumes: slices.Clone(r.volumes), Drives: slices.Clone(r.drives), Errors: r.errors}
	if r.errors > 0 && !r.lastErr.IsZero() {
		d.LastError = r.lastErr.Unix()
	}
	return d
}

// ReadNow relève tout de suite (commande « wol-agent status », rapport de diagnostic).
func ReadNow() (*protocol.Disks, []string) {
	r := New()
	var problems []string
	r.OnError = func(what string, err error) { problems = append(problems, what+" : "+err.Error()) }
	r.Refresh()
	return r.Read(), problems
}

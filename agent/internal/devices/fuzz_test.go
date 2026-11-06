package devices

import (
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol2"
)

// Fuzzing : go test ./internal/devices -run '^$' -fuzz FuzzOpenStorage. Sans -fuzz, seuls les exemples
// de départ sont rejoués.

// FuzzOpenStorage : un registre quelconque ne fait jamais paniquer ; un registre accepté n'a que des
// appareils distincts aux rôles valides, et se réécrit puis se relit à l'identique.
func FuzzOpenStorage(f *testing.F) {
	now := func() time.Time { return time.Date(2026, 11, 6, 3, 0, 0, 0, time.UTC) }
	st := &memStorage{}
	r, err := OpenStorage(st, "fuzz", now)
	if err != nil {
		f.Fatal(err)
	}
	_, code, err := r.CreateInvite(protocol2.RoleAdmin, "", time.Hour, nil, ByLocal)
	if err != nil {
		f.Fatal(err)
	}
	if _, err := r.Redeem(code, protocol2.PinOf([]byte("téléphone")), "Téléphone"); err != nil {
		f.Fatal(err)
	}
	if _, _, err := r.CreateInvite(protocol2.RoleView, "Invité", time.Hour, nil, ByLocal); err != nil {
		f.Fatal(err)
	}
	for _, seed := range [][]byte{st.data, []byte(`{"devices":[],"invites":[]}`), []byte(`{"devices":[{"device":"x"}]}`), []byte(`null`)} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		r, err := OpenStorage(&memStorage{data: raw}, "fuzz", now)
		if err != nil {
			return
		}
		seen := map[protocol2.Pin]bool{}
		for _, d := range r.Devices() {
			if seen[d.Pin] || d.Role.String() == "" {
				t.Fatalf("registre accepté : appareil %s en double ou sans rôle", d.Pin.Short())
			}
			seen[d.Pin] = true
		}
		// Réécrit (en retirant un appareil absent : rien ne change), puis relu : le même registre.
		out := &memStorage{data: raw}
		again, err := OpenStorage(out, "fuzz", now)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := again.Remove(protocol2.PinOf([]byte("absent")), false); err == nil {
			t.Fatal("appareil absent retiré")
		}
		if len(again.Devices()) != len(r.Devices()) {
			t.Fatalf("relecture : %d appareils au lieu de %d", len(again.Devices()), len(r.Devices()))
		}
	})
}

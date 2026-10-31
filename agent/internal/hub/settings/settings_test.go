package settings

import (
	"errors"
	"os"
	"testing"
)

type memStorage struct {
	data []byte
	err  error
}

func (m *memStorage) Load() ([]byte, error) {
	if m.data == nil {
		return nil, os.ErrNotExist
	}
	return m.data, nil
}

func (m *memStorage) Save(data []byte) error {
	if m.err != nil {
		return m.err
	}
	m.data = append([]byte(nil), data...)
	return nil
}

func TestSettings(t *testing.T) {
	st := &memStorage{}
	s, err := Open(st)
	if err != nil || s.Get() != Defaults || Defaults.RetentionDays != 365 {
		t.Fatalf("réglages par défaut : %+v, %v", s.Get(), err)
	}
	if err := s.Set(Values{RetentionDays: 89}); err == nil {
		t.Fatal("89 jours acceptés")
	}
	if err := s.Set(Values{RetentionDays: 180, Timezone: DefaultTimezone}); err != nil {
		t.Fatal(err)
	}
	again, err := Open(st)
	if err != nil || again.Get().RetentionDays != 180 {
		t.Fatalf("relecture : %+v, %v", again.Get(), err)
	}
	st.err = errors.New("disque plein")
	if err := s.Set(Values{RetentionDays: 400, Timezone: DefaultTimezone}); err == nil || s.Get().RetentionDays != 180 {
		t.Fatalf("échec d'enregistrement : %v, %+v", err, s.Get())
	}
	st.err = nil
	// Fuseau horaire : Europe/Paris par défaut, y compris pour des réglages d'avant les programmations.
	if old, err := Open(&memStorage{data: []byte(`{"retention":200}`)}); err != nil || old.Get().Timezone != DefaultTimezone {
		t.Fatalf("anciens réglages : %+v, %v", old, err)
	}
	if err := s.Set(Values{RetentionDays: 180, Timezone: "America/Montreal"}); err != nil || s.Get().Timezone != "America/Montreal" {
		t.Fatalf("fuseau horaire : %+v, %v", s.Get(), err)
	}
	for _, tz := range []string{"", "Local", "Europe/Atlantide", "../../etc/passwd"} {
		if err := s.Set(Values{RetentionDays: 180, Timezone: tz}); err == nil {
			t.Errorf("fuseau %q accepté", tz)
		}
	}
	if loc, err := Location("Europe/Paris"); err != nil || loc.String() != "Europe/Paris" {
		t.Errorf("Europe/Paris : %v, %v", loc, err)
	}
	for _, raw := range []string{"{", `{"retention":5000}`, `{"retention":200,"timezone":"Mars/Olympus"}`} {
		if _, err := Open(&memStorage{data: []byte(raw)}); err == nil {
			t.Errorf("%s accepté", raw)
		}
	}
}

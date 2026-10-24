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
	if err := s.Set(Values{RetentionDays: 180}); err != nil {
		t.Fatal(err)
	}
	again, err := Open(st)
	if err != nil || again.Get().RetentionDays != 180 {
		t.Fatalf("relecture : %+v, %v", again.Get(), err)
	}
	st.err = errors.New("disque plein")
	if err := s.Set(Values{RetentionDays: 400}); err == nil || s.Get().RetentionDays != 180 {
		t.Fatalf("échec d'enregistrement : %v, %+v", err, s.Get())
	}
	for _, raw := range []string{"{", `{"retention":5000}`} {
		if _, err := Open(&memStorage{data: []byte(raw)}); err == nil {
			t.Errorf("%s accepté", raw)
		}
	}
}

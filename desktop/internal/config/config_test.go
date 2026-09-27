package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/slaynaw/wakeonlan/desktop/internal/model"
)

const testKey = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8"

func sample(t *testing.T) model.AppConfig {
	t.Helper()
	c, err := Decode([]byte(`{"schemaVersion":1,"devices":[
		{"id":"pc1","name":"PC Bureau","mac":"AA:BB:CC:DD:EE:01","host":"192.168.1.20","agent":{"key":"` + testKey + `"}},
		{"id":"nas","name":"NAS","mac":"AA:BB:CC:DD:EE:02","secureOnPassword":"11:22:33:44:55:66"}],
		"settings":{"pollIntervalSeconds":5}}`))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func reason(err error) Reason {
	var e *Error
	if errors.As(err, &e) {
		return e.Reason
	}
	return ""
}

func TestCodecValidation(t *testing.T) {
	c := sample(t)
	data, _ := Encode(c)
	back, err := Decode(data)
	if err != nil || !reflect.DeepEqual(c, back) {
		t.Fatalf("aller-retour : %v", err)
	}
	bad := strings.Replace(string(data), "192.168.1.20", "pas une ip !", 1)
	if _, err := Decode([]byte(bad)); reason(err) != InvalidData {
		t.Errorf("configuration invalide acceptée : %v", err)
	}
	dup := c.Clone()
	dup.Devices = append(dup.Devices, dup.Devices[0])
	if _, err := Sanitize(dup); reason(err) != InvalidData || !strings.Contains(err.Error(), "en double") {
		t.Errorf("doublon accepté : %v", err)
	}
	if _, err := Decode([]byte(`{"schemaVersion":99,"devices":[]}`)); reason(err) != NewerVersion {
		t.Errorf("version future : %v", err)
	}
	extreme, err := Decode([]byte(`{"schemaVersion":1,"devices":[],"settings":{"pollIntervalSeconds":0,"wakeTimeoutSeconds":999999}}`))
	if err != nil || extreme.Settings.PollIntervalSeconds != 1 || extreme.Settings.WakeTimeoutSeconds != 900 {
		t.Errorf("réglages non bornés : %+v %v", extreme.Settings, err)
	}
	if _, err := Decode([]byte(`[1,2]`)); reason(err) != InvalidData {
		t.Errorf("JSON non objet accepté : %v", err)
	}
}

func TestPlainExportHasNoSecrets(t *testing.T) {
	c := sample(t)
	text, err := Export(c, ExportOptions{ExportedAt: "2026-09-27T10:00:00Z", App: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(text), testKey) || strings.Contains(string(text), "11:22:33:44:55:66") {
		t.Fatal("secret présent dans l'export en clair")
	}
	if enc, _ := IsEncrypted(text); enc {
		t.Fatal("export en clair marqué chiffré")
	}
	imported, err := Import(text, "")
	if err != nil {
		t.Fatal(err)
	}
	if imported.Devices[0].Agent.Key != "" || imported.Devices[1].SecureOnPassword != nil ||
		imported.Devices[0].MAC != c.Devices[0].MAC {
		t.Errorf("import incorrect : %+v", imported)
	}
}

func TestEncryptedExport(t *testing.T) {
	c := sample(t)
	password := "correct horse battery"
	text, err := Export(c, ExportOptions{Password: password, ExportedAt: "2026-09-27T10:00:00Z", Iterations: MinIterations})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(text), testKey) || strings.Contains(string(text), "PC Bureau") {
		t.Fatal("contenu en clair dans l'export chiffré")
	}
	if enc, _ := IsEncrypted(text); !enc {
		t.Fatal("export chiffré non détecté")
	}
	back, err := Import(text, password)
	if err != nil || !reflect.DeepEqual(c, back) {
		t.Fatalf("import : %v", err)
	}
	if _, err := Import(text, "mauvais mot de passe"); reason(err) != WrongPassword {
		t.Errorf("mauvais mot de passe : %v", err)
	}
	if _, err := Import(text, ""); reason(err) != PasswordRequired {
		t.Errorf("mot de passe absent : %v", err)
	}
	data := regexp.MustCompile(`"data": "([^"]+)"`).FindSubmatch(text)[1]
	flipped := []byte(string(data))
	if flipped[10] == 'A' {
		flipped[10] = 'B'
	} else {
		flipped[10] = 'A'
	}
	tampered := strings.Replace(string(text), string(data), string(flipped), 1)
	if _, err := Import([]byte(tampered), password); err == nil {
		t.Error("altération non détectée")
	}
	if _, err := Export(c, ExportOptions{Password: "court"}); err == nil {
		t.Error("mot de passe trop court accepté")
	}
}

func TestForeignFileRejected(t *testing.T) {
	for _, text := range []string{`{"hello":"world"}`, `pas du json`, `{"format":"autre","version":1,"exportedAt":"x"}`} {
		if _, err := Import([]byte(text), ""); reason(err) != NotABackup {
			t.Errorf("%s : %v", text, err)
		}
	}
	if _, err := Import([]byte(`{"format":"wakeonlan-config","version":2,"exportedAt":"x"}`), ""); reason(err) != NewerVersion {
		t.Errorf("version future : %v", err)
	}
}

// Sauvegardes produites indépendamment (Node.js / OpenSSL), également relues par les tests Android.
func TestSharedExportVectors(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "protocol", "export-vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var vectors struct {
		Password        string          `json:"password"`
		Config          json.RawMessage `json:"config"`
		EncryptedExport json.RawMessage `json:"encryptedExport"`
		PlainExport     json.RawMessage `json:"plainExport"`
	}
	if err := json.Unmarshal(raw, &vectors); err != nil {
		t.Fatal(err)
	}
	want, err := Decode(vectors.Config)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Import(vectors.EncryptedExport, vectors.Password)
	if err != nil {
		t.Fatalf("sauvegarde chiffrée de référence illisible : %v", err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Errorf("contenu différent :\n%+v\n%+v", want, got)
	}
	if got.Settings.ConfirmPowerActions || got.Settings.WakeTimeoutSeconds != 240 || *got.Devices[1].BroadcastAddress != "192.168.1.255" {
		t.Errorf("valeurs inattendues : %+v", got)
	}
	plain, err := Import(vectors.PlainExport, "")
	if err != nil || plain.Devices[0].Agent.Key != "" || len(plain.Devices) != 2 {
		t.Errorf("sauvegarde en clair de référence : %+v %v", plain, err)
	}
}

func TestStore(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	empty, err := s.Load()
	if err != nil || len(empty.Devices) != 0 || empty.Settings != model.DefaultSettings() {
		t.Fatalf("configuration initiale : %+v %v", empty, err)
	}
	c := sample(t)
	if err := s.Save(c); err != nil {
		t.Fatal(err)
	}
	back, err := s.Load()
	if err != nil || !reflect.DeepEqual(c, back) {
		t.Fatalf("relecture : %v", err)
	}
	// Un fichier abîmé est mis de côté, jamais supprimé.
	if err := os.WriteFile(s.Path(), []byte("abîmé"), 0o600); err != nil {
		t.Fatal(err)
	}
	reset, err := s.Load()
	if err == nil || len(reset.Devices) != 0 {
		t.Fatalf("fichier abîmé : %+v %v", reset, err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || !strings.Contains(entries[0].Name(), ".illisible-") {
		t.Errorf("fichier non mis de côté : %v", entries)
	}
}

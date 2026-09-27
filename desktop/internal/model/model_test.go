package model

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

const testKey = "q83vEjRWeJq83vEjRWeJq83vEjRWeJq83vEjRWeJq80"

func strPtr(s string) *string { return &s }

func sampleConfig() AppConfig {
	c := NewConfig()
	c.Devices = []Device{
		{ID: "pc1", Name: "PC Bureau", MAC: mustMAC("AA:BB:CC:DD:EE:01"), Host: "192.168.1.20", WolPort: 9,
			ProbePorts: DefaultProbePorts(), Agent: &AgentSettings{Port: 9770, Key: testKey}},
		{ID: "nas", Name: "NAS", MAC: mustMAC("AA:BB:CC:DD:EE:02"), WolPort: 9, ProbePorts: DefaultProbePorts(),
			SecureOnPassword: strPtr("11:22:33:44:55:66")},
	}
	c.Settings.PollIntervalSeconds = 5
	return c
}

func mustMAC(s string) MAC {
	m, err := ParseMAC(s)
	if err != nil {
		panic(err)
	}
	return m
}

func TestParseMACFormats(t *testing.T) {
	for _, in := range []string{"01:23:45:67:89:ab", "01-23-45-67-89-AB", "0123.4567.89ab", "0123456789AB", "  01:23:45:67:89:AB "} {
		m, err := ParseMAC(in)
		if err != nil || m.String() != "01:23:45:67:89:AB" {
			t.Errorf("ParseMAC(%q) = %v, %v", in, m, err)
		}
	}
	for _, in := range []string{"", "01:23:45:67:89", "01:23:45:67:89:AB:CD", "01:23-45:67:89:AB", "GG:23:45:67:89:AB",
		"00:00:00:00:00:00", "FF:FF:FF:FF:FF:FF", "0123456789A"} {
		if _, err := ParseMAC(in); err == nil {
			t.Errorf("ParseMAC(%q) aurait dû échouer", in)
		}
	}
}

func TestJSONRoundTripAndAndroidFormat(t *testing.T) {
	c := sampleConfig()
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var back AppConfig
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c, back) {
		t.Fatalf("aller-retour différent :\n%+v\n%+v", c, back)
	}
	// Même forme que kotlinx.serialization (encodeDefaults = true, explicitNulls = false).
	want := `{"id":"nas","name":"NAS","mac":"AA:BB:CC:DD:EE:02","host":"","wolPort":9,"secureOnPassword":"11:22:33:44:55:66","probePorts":[3389,445,22,139]}`
	if !strings.Contains(string(data), want) {
		t.Errorf("format inattendu : %s", data)
	}
	if !strings.Contains(string(data), `"settings":{"pollIntervalSeconds":5,"confirmPowerActions":true,"wakeTimeoutSeconds":180}`) {
		t.Errorf("réglages inattendus : %s", data)
	}
}

func TestJSONDefaultsAndRequiredFields(t *testing.T) {
	var c AppConfig
	if err := json.Unmarshal([]byte(`{"devices":[{"id":"a","name":"A","mac":"aabbccddeeff","agent":{"key":""}}]}`), &c); err != nil {
		t.Fatal(err)
	}
	d := c.Devices[0]
	if c.SchemaVersion != 1 || d.WolPort != 9 || !reflect.DeepEqual(d.ProbePorts, DefaultProbePorts()) ||
		d.Agent.Port != 9770 || c.Settings != DefaultSettings() {
		t.Errorf("valeurs par défaut incorrectes : %+v", c)
	}
	for _, bad := range []string{
		`{"devices":[{"name":"A","mac":"aabbccddeeff"}]}`,
		`{"devices":[{"id":"a","mac":"aabbccddeeff"}]}`,
		`{"devices":[{"id":"a","name":"A"}]}`,
		`{"devices":[{"id":"a","name":"A","mac":"zz"}]}`,
		`{"devices":[{"id":"a","name":"A","mac":"aabbccddeeff","agent":{"port":1}}]}`,
		`{"devices":[{"id":"a","name":"A","mac":"aabbccddeeff","wolPort":"9"}]}`,
	} {
		if err := json.Unmarshal([]byte(bad), &c); err == nil {
			t.Errorf("%s aurait dû être refusé", bad)
		}
	}
}

func TestSecretsNeverInString(t *testing.T) {
	c := sampleConfig()
	for _, d := range c.Devices {
		s := d.String()
		if strings.Contains(s, testKey) || strings.Contains(s, "11:22:33:44:55:66") {
			t.Errorf("secret visible : %s", s)
		}
	}
}

func TestMergeAndMove(t *testing.T) {
	c := sampleConfig()
	renamed := c.Devices[0]
	renamed.Name = "Renommé"
	other := c.Devices[1]
	other.ID = "new"
	merged := c.MergeDevicesFrom(AppConfig{Devices: []Device{renamed, other}})
	if ids(merged) != "pc1,nas,new" || merged.Devices[0].Name != "Renommé" {
		t.Errorf("fusion : %s", ids(merged))
	}
	if ids(c.Move("pc1", 1)) != "nas,pc1" || ids(c.Move("pc1", -1)) != "pc1,nas" || ids(c) != "pc1,nas" {
		t.Error("déplacement incorrect")
	}
	if ids(c.Remove("pc1")) != "nas" || ids(c) != "pc1,nas" {
		t.Error("suppression incorrecte")
	}
}

func ids(c AppConfig) string {
	var out []string
	for _, d := range c.Devices {
		out = append(out, d.ID)
	}
	return strings.Join(out, ",")
}

func TestValidation(t *testing.T) {
	ok := sampleConfig().Devices[0]
	if errs := Validate(ok); len(errs) != 0 {
		t.Fatalf("valide attendu : %v", errs)
	}
	fields := func(mutate func(*Device)) string {
		d := ok.Clone()
		mutate(&d)
		var out []string
		for _, e := range Validate(d) {
			out = append(out, string(e.Field))
		}
		return strings.Join(out, ",")
	}
	cases := map[string]func(*Device){
		"NAME":      func(d *Device) { d.Name = " " },
		"HOST":      func(d *Device) { d.Host = "256.1.1.1" },
		"WOL_PORT":  func(d *Device) { d.WolPort = 70000 },
		"AGENT_KEY": func(d *Device) { d.Agent = &AgentSettings{Port: 9770, Key: "xyz"} },
		"BROADCAST": func(d *Device) { d.BroadcastAddress = strPtr("192.168.1") },
		"SECURE_ON": func(d *Device) { d.SecureOnPassword = strPtr("123") },
	}
	for want, mutate := range cases {
		if got := fields(mutate); got != want {
			t.Errorf("attendu %s, obtenu %s", want, got)
		}
	}
	if got := fields(func(d *Device) { d.Host = "" }); got != "HOST" {
		t.Errorf("agent sans adresse : %s", got)
	}
	for _, h := range []string{"pc-bureau", "pc-bureau.local", "10.0.0.1", "fe80::1%wlan0", "2001:db8::1"} {
		if !IsValidHost(h) {
			t.Errorf("%s devrait être valide", h)
		}
	}
	for _, h := range []string{"pc bureau", "-pc", "1.2.3", "http://pc", "a..b", " pc"} {
		if IsValidHost(h) {
			t.Errorf("%s devrait être invalide", h)
		}
	}
	if DecodeAgentKey(" "+testKey+"= ") == nil || DecodeAgentKey(testKey[:40]) != nil {
		t.Error("décodage de clé incorrect")
	}
}

func TestFormBuild(t *testing.T) {
	f := NewForm()
	f.Name = "  PC Salon "
	f.MAC = "aa-bb-cc-dd-ee-ff"
	f.Host = " 192.168.1.30 "
	f.ProbePorts = "3389; 22 22,, 445"
	d, errs := f.Build("")
	if errs != nil {
		t.Fatal(errs)
	}
	if d.Name != "PC Salon" || d.Host != "192.168.1.30" || d.MAC.String() != "AA:BB:CC:DD:EE:FF" ||
		!reflect.DeepEqual(d.ProbePorts, []int{3389, 22, 445}) || d.Agent != nil || len(d.ID) != 36 {
		t.Errorf("formulaire mal converti : %+v", d)
	}
	if !reflect.DeepEqual(FormFrom(d).ProbePorts, "3389, 22, 445") {
		t.Error("ports mal formatés")
	}

	f.MAC = "12"
	f.WolPort = "x"
	f.ProbePorts = "a"
	f.AgentEnabled = true
	f.AgentPort = ""
	_, errs = f.Build("id")
	want := map[Field]string{
		FieldMAC:        "Adresse MAC invalide (ex. AA:BB:CC:DD:EE:FF)",
		FieldWolPort:    "Nombre attendu",
		FieldProbePorts: "Liste de ports séparés par des virgules",
		FieldAgentPort:  "Nombre attendu",
	}
	if !reflect.DeepEqual(errs, want) {
		t.Errorf("erreurs : %v", errs)
	}

	f = NewForm()
	f.Name = "PC"
	f.MAC = "aabbccddeeff"
	f.AgentEnabled = true
	f.AgentKey = "abc"
	_, errs = f.Build("id")
	if errs[FieldAgentKey] != "Clé d'agent invalide" || errs[FieldHost] != "L'adresse du PC est nécessaire pour l'agent" {
		t.Errorf("erreurs agent : %v", errs)
	}
}

func TestUTF16Helpers(t *testing.T) {
	if UTF16Len("é😀") != 3 || TruncateUTF16("ab😀c", 3) != "ab" || TruncateUTF16("abc", 5) != "abc" {
		t.Error("calcul UTF-16 incorrect")
	}
}

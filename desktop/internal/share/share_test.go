package share

import (
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/slaynaw/wakeonlan/desktop/internal/model"
)

// Vecteurs communs avec l'application Android (core/src/test/.../ShareTest.kt).
const vectorsPath = "../../../protocol/share-vectors.json"

type vectors struct {
	OwnerPrivate     string `json:"ownerPrivate"`
	OwnerPublic      string `json:"ownerPublic"`
	DevicePrivate    string `json:"devicePrivate"`
	DevicePublic     string `json:"devicePublic"`
	OtherPrivate     string `json:"otherDevicePrivate"`
	EphemeralPrivate string `json:"ephemeralPrivate"`
	IV               string `json:"iv"`
	Revision         int64  `json:"revision"`
	IssuedAt         string `json:"issuedAt"`
	Plaintext        string `json:"plaintext"`
	Expected         struct {
		OwnerKeyID       string   `json:"ownerKeyId"`
		DeviceKeyID      string   `json:"deviceKeyId"`
		FileName         string   `json:"fileName"`
		VerificationCode string   `json:"verificationCode"`
		EPK              string   `json:"epk"`
		Data             string   `json:"data"`
		OwnerName        string   `json:"ownerName"`
		RecipientName    string   `json:"recipientName"`
		DeviceNames      []string `json:"deviceNames"`
		AgentKeys        []string `json:"agentKeys"`
	} `json:"expected"`
	File   string `json:"file"`
	Invite struct {
		Link   string `json:"link"`
		Fields Invite `json:"fields"`
	} `json:"invite"`
	Request struct {
		Link   string  `json:"link"`
		Fields Request `json:"fields"`
	} `json:"request"`
	Invalid []struct {
		Name  string `json:"name"`
		File  string `json:"file"`
		Error string `json:"error"`
	} `json:"invalid"`
	InvalidLinks []string `json:"invalidLinks"`
}

func loadVectors(t *testing.T) vectors {
	t.Helper()
	data, err := os.ReadFile(vectorsPath)
	if err != nil {
		t.Fatal(err)
	}
	var v vectors
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestVectors(t *testing.T) {
	v := loadVectors(t)
	owner, err := ParseOwnerPrivate(v.OwnerPrivate)
	if err != nil {
		t.Fatal(err)
	}
	device, err := ParseDevicePrivate(v.DevicePrivate)
	if err != nil {
		t.Fatal(err)
	}
	if OwnerPublic(owner) != v.OwnerPublic || DevicePublic(device) != v.DevicePublic {
		t.Fatal("clés publiques différentes des vecteurs")
	}
	if KeyID(v.OwnerPublic) != v.Expected.OwnerKeyID || KeyID(v.DevicePublic) != v.Expected.DeviceKeyID {
		t.Errorf("identifiants : %s %s", KeyID(v.OwnerPublic), KeyID(v.DevicePublic))
	}
	if FileName(v.DevicePublic) != v.Expected.FileName {
		t.Errorf("nom de fichier : %s", FileName(v.DevicePublic))
	}
	if code := VerificationCode(v.OwnerPublic, v.DevicePublic); code != v.Expected.VerificationCode {
		t.Errorf("code de vérification : %s", code)
	}

	// Chiffrement déterministe (clé éphémère et IV imposés) : mêmes octets qu'Android.
	eph, err := ParseDevicePrivate(v.EphemeralPrivate)
	if err != nil {
		t.Fatal(err)
	}
	iv, _ := base64.StdEncoding.DecodeString(v.IV)
	p, err := encrypt(v.OwnerPublic, v.DevicePublic, v.Revision, v.IssuedAt, []byte(v.Plaintext), eph, iv)
	if err != nil {
		t.Fatal(err)
	}
	if p.EPK != v.Expected.EPK || p.Data != v.Expected.Data {
		t.Errorf("chiffrement différent des vecteurs :\n%s\n%s", p.EPK, p.Data)
	}

	// Fichier de référence : vérifié et déchiffré.
	opened, err := Open([]byte(v.File), v.OwnerPublic, device, 0)
	if err != nil {
		t.Fatal(err)
	}
	c := opened.Content
	if opened.Revision != v.Revision || c.OwnerName != v.Expected.OwnerName || c.RecipientName != v.Expected.RecipientName {
		t.Errorf("contenu : %+v", opened)
	}
	var names, keys []string
	for _, d := range c.Devices {
		names = append(names, d.Name)
		key := ""
		if d.Agent != nil {
			key = d.Agent.Key
		}
		keys = append(keys, key)
	}
	if strings.Join(names, "|") != strings.Join(v.Expected.DeviceNames, "|") || strings.Join(keys, "|") != strings.Join(v.Expected.AgentKeys, "|") {
		t.Errorf("PC reçus : %v %v", names, keys)
	}

	// Cas refusés.
	other, _ := ParseDevicePrivate(v.OtherPrivate)
	for _, bad := range v.Invalid {
		key := device
		if bad.Name == "autre appareil" {
			key = other
		}
		_, err := Open([]byte(bad.File), v.OwnerPublic, key, v.Revision)
		if err == nil || errorName(err) != bad.Error {
			t.Errorf("%s : erreur %v, attendu %s", bad.Name, err, bad.Error)
		}
	}

	// Liens.
	inv, err := ParseInvite(v.Invite.Link)
	if err != nil || inv != v.Invite.Fields {
		t.Errorf("invitation : %+v %v", inv, err)
	}
	req, err := ParseRequest(v.Request.Link)
	if err != nil || req != v.Request.Fields {
		t.Errorf("demande : %+v %v", req, err)
	}
	for _, link := range v.InvalidLinks {
		_, err1 := ParseInvite(link)
		_, err2 := ParseRequest(link)
		if err1 == nil || err2 == nil {
			t.Errorf("lien accepté : %s", link)
		}
	}
}

// errorName traduit une erreur d'ouverture en nom stable (commun avec Kotlin).
func errorName(err error) string {
	switch {
	case errors.Is(err, ErrSignature):
		return "signature"
	case errors.Is(err, ErrRecipient):
		return "recipient"
	case errors.Is(err, ErrOlder):
		return "older"
	case errors.Is(err, ErrInvalid):
		return "invalid"
	}
	return err.Error()
}

func TestSealOpenRoundTrip(t *testing.T) {
	owner, _ := NewOwnerKey()
	device, _ := NewDeviceKey()
	secureOn := "01:02:03:04:05:06"
	content := Content{OwnerName: "Hugo", RecipientName: "Léa", Devices: []model.Device{{
		ID: "a", Name: "PC streaming", MAC: mustMAC(t, "AA:BB:CC:DD:EE:FF"), Host: "192.168.1.20",
		WolPort: 9, SecureOnPassword: &secureOn, ProbePorts: model.DefaultProbePorts(),
	}}}
	data, err := Seal(owner, DevicePublic(device), 5, "2026-09-28T15:00:00Z", content)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := Open(data, OwnerPublic(owner), device, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(opened.Content.Devices) != 1 || opened.Content.Devices[0].Host != "192.168.1.20" || *opened.Content.Devices[0].SecureOnPassword != secureOn {
		t.Errorf("contenu : %+v", opened.Content)
	}
	// Le contenu en clair n'apparaît pas dans le fichier.
	if strings.Contains(string(data), "streaming") || strings.Contains(string(data), "192.168") {
		t.Error("contenu lisible dans le fichier")
	}
	// Mauvaise clé de propriétaire, révision plus ancienne, autre appareil.
	intruder, _ := NewOwnerKey()
	if _, err := Open(data, OwnerPublic(intruder), device, 0); !errors.Is(err, ErrSignature) {
		t.Errorf("autre propriétaire : %v", err)
	}
	if _, err := Open(data, OwnerPublic(owner), device, 6); !errors.Is(err, ErrOlder) {
		t.Errorf("révision : %v", err)
	}
	stranger, _ := NewDeviceKey()
	if _, err := Open(data, OwnerPublic(owner), stranger, 0); !errors.Is(err, ErrRecipient) {
		t.Errorf("autre appareil : %v", err)
	}
	// Un intrus qui signe un fichier avec sa propre clé est refusé.
	forged, _ := Seal(intruder, DevicePublic(device), 9, "2026-09-28T15:00:00Z", content)
	if _, err := Open(forged, OwnerPublic(owner), device, 0); !errors.Is(err, ErrSignature) {
		t.Errorf("fichier d'un intrus : %v", err)
	}
}

func TestInvalidContentRejected(t *testing.T) {
	owner, _ := NewOwnerKey()
	device, _ := NewDeviceKey()
	bad := Content{OwnerName: "Hugo", Devices: []model.Device{{ID: "a", Name: "PC", MAC: mustMAC(t, "AA:BB:CC:DD:EE:FF"), WolPort: 70000}}}
	data, err := Seal(owner, DevicePublic(device), 1, "", bad)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(data, OwnerPublic(owner), device, 0); !errors.Is(err, ErrInvalid) {
		t.Errorf("PC invalide accepté : %v", err)
	}
}

func TestLinks(t *testing.T) {
	owner, _ := NewOwnerKey()
	device, _ := NewDeviceKey()
	inv := Invite{Name: "Hugo Dupont", Owner: OwnerPublic(owner), User: "slaynAW", Gist: "0123456789abcdef0123456789abcdef"}
	parsed, err := ParseInvite("  " + inv.Link() + "\n")
	if err != nil || parsed != inv {
		t.Fatalf("invitation : %+v %v", parsed, err)
	}
	req := Request{Name: "Léa & Co", Device: DevicePublic(device), Owner: KeyID(inv.Owner)}
	if got, err := ParseRequest(req.Link()); err != nil || got != req {
		t.Fatalf("demande : %+v %v", got, err)
	}
	if Kind(inv.Link()) != "invite" || Kind(req.Link()) != "request" || Kind("wolagent://pair?x=1") != "" {
		t.Error("type de lien")
	}
	// Une clé de réception n'est pas une clé de propriétaire valide pour une invitation ? Les deux sont
	// des points P-256 : seule la forme est vérifiée ici, la signature fait le reste.
	for _, name := range []string{"", " Hugo", strings.Repeat("a", MaxNameLength+1), "a\nb"} {
		if ValidName(name) {
			t.Errorf("nom accepté : %q", name)
		}
	}
	if !ValidName(strings.Repeat("é", MaxNameLength)) {
		t.Error("nom de 40 caractères refusé")
	}
}

func mustMAC(t *testing.T, s string) model.MAC {
	t.Helper()
	m, err := model.ParseMAC(s)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// TestWriteVectors régénère protocol/share-vectors.json (WOL_WRITE_SHARE_VECTORS=1 go test -run TestWriteVectors).
func TestWriteVectors(t *testing.T) {
	if os.Getenv("WOL_WRITE_SHARE_VECTORS") != "1" {
		t.Skip("régénération des vecteurs désactivée")
	}
	seed := func(label string) []byte {
		sum := sha256.Sum256([]byte("wakeonlan-share test " + label))
		return sum[:]
	}
	owner, err := ecdsa.ParseRawPrivateKey(elliptic.P256(), seed("owner"))
	if err != nil {
		t.Fatal(err)
	}
	device, _ := ecdh.P256().NewPrivateKey(seed("device"))
	other, _ := ecdh.P256().NewPrivateKey(seed("other device"))
	eph, _ := ecdh.P256().NewPrivateKey(seed("ephemeral"))
	iv := seed("iv")[:ivLen]
	agentKey := base64.RawURLEncoding.EncodeToString(seed("agent"))
	secureOn := "01:23:45:67:89:AB"
	content := Content{OwnerName: "Hugo", RecipientName: "Léa", Devices: []model.Device{
		{ID: "4f1c2a9e-0b7d-4c3e-9a51-2d6f8e0b1c37", Name: "PC streaming", MAC: mustMAC(t, "3C:7C:3F:1A:2B:4C"),
			Host: "192.168.1.20", WolPort: 9, SecureOnPassword: &secureOn, ProbePorts: []int{3389, 445}},
		{ID: "b2e8d7c6-5a4f-4e3d-8c2b-1a0f9e8d7c6b", Name: "Bureau", MAC: mustMAC(t, "00:11:22:33:44:55"),
			Host: "bureau.local", WolPort: 7, ProbePorts: model.DefaultProbePorts(),
			Agent: &model.AgentSettings{Port: 9770, Key: agentKey}},
	}}
	plain, _ := json.Marshal(content)
	const revision, issued = 1790600000000, "2026-09-28T15:00:00Z"
	ownerPub, devicePub := OwnerPublic(owner), DevicePublic(device)
	p, err := encrypt(ownerPub, devicePub, revision, issued, plain, eph, iv)
	if err != nil {
		t.Fatal(err)
	}
	file, err := seal(owner, devicePub, revision, issued, plain, eph, iv)
	if err != nil {
		t.Fatal(err)
	}

	// Variantes refusées.
	var f File
	_ = json.Unmarshal(file, &f)
	body, _ := base64.StdEncoding.DecodeString(f.Payload)
	tampered := strings.Replace(string(body), `"revision":1790600000000`, `"revision":1790600000001`, 1)
	tamperedFile := f
	tamperedFile.Payload = base64.StdEncoding.EncodeToString([]byte(tampered))
	intruder, _ := ecdsa.ParseRawPrivateKey(elliptic.P256(), seed("intruder"))
	forged, _ := seal(intruder, devicePub, revision, issued, plain, eph, iv)
	older, _ := seal(owner, devicePub, revision-1, issued, plain, eph, iv)
	badData := p
	raw, _ := base64.StdEncoding.DecodeString(p.Data)
	raw[0] ^= 1
	badData.Data = base64.StdEncoding.EncodeToString(raw)
	badBody, _ := json.Marshal(badData)
	digest := sha256.Sum256(append([]byte(signPrefix), badBody...))
	sig, _ := ecdsa.SignASN1(rand.Reader, owner, digest[:])
	corrupted, _ := json.Marshal(File{Format: Format, Version: Version,
		Payload: base64.StdEncoding.EncodeToString(badBody), Signature: base64.StdEncoding.EncodeToString(sig)})
	mustJSON := func(v any) string { b, _ := json.Marshal(v); return string(b) }

	invite := Invite{Name: "Hugo", Owner: ownerPub, User: "slaynAW", Gist: "8f3a1c2b4d5e6f708192a3b4c5d6e7f8"}
	request := Request{Name: "Léa", Device: devicePub, Owner: KeyID(ownerPub)}
	out := map[string]any{
		"comment":            "Vecteurs du format de partage wakeonlan-share/1 (docs/PARTAGE.md), communs à Windows (Go) et Android (Kotlin). Générés par desktop/internal/share (TestWriteVectors).",
		"ownerPrivate":       EncodePrivate(OwnerPrivateBytes(owner)),
		"ownerPublic":        ownerPub,
		"devicePrivate":      EncodePrivate(device.Bytes()),
		"devicePublic":       devicePub,
		"otherDevicePrivate": EncodePrivate(other.Bytes()),
		"otherDevicePublic":  DevicePublic(other),
		"ephemeralPrivate":   EncodePrivate(eph.Bytes()),
		"ephemeralPublic":    DevicePublic(eph),
		"iv":                 base64.StdEncoding.EncodeToString(iv),
		"revision":           revision,
		"issuedAt":           issued,
		"plaintext":          string(plain),
		"expected": map[string]any{
			"ownerKeyId": KeyID(ownerPub), "deviceKeyId": KeyID(devicePub), "fileName": FileName(devicePub),
			"verificationCode": VerificationCode(ownerPub, devicePub),
			"epk":              p.EPK, "data": p.Data,
			"ownerName": "Hugo", "recipientName": "Léa",
			"deviceNames": []string{"PC streaming", "Bureau"}, "agentKeys": []string{"", agentKey},
		},
		"file":    string(file),
		"invite":  map[string]any{"link": invite.Link(), "fields": invite},
		"request": map[string]any{"link": request.Link(), "fields": request},
		"invalid": []map[string]string{
			{"name": "contenu modifié", "file": mustJSON(tamperedFile), "error": "signature"},
			{"name": "signé par un intrus", "file": string(forged), "error": "signature"},
			{"name": "autre appareil", "file": string(file), "error": "recipient"},
			{"name": "révision plus ancienne", "file": string(older), "error": "older"},
			{"name": "données chiffrées abîmées", "file": string(corrupted), "error": "invalid"},
			{"name": "format inconnu", "file": `{"format":"autre","version":1,"payload":"","signature":""}`, "error": "invalid"},
		},
		"invalidLinks": []string{
			"wolagent://pair?host=1",
			"wolshare://invite?v=1&name=Hugo&owner=AAAA&user=slaynAW&gist=8f3a1c2b4d5e6f708192a3b4c5d6e7f8",
			"wolshare://invite?v=2&name=Hugo&owner=" + ownerPub + "&user=slaynAW&gist=8f3a1c2b4d5e6f708192a3b4c5d6e7f8",
			"wolshare://invite?v=1&name=Hugo&owner=" + ownerPub + "&user=-bad&gist=8f3a1c2b4d5e6f708192a3b4c5d6e7f8",
			"wolshare://request?v=1&name=&device=" + devicePub + "&owner=" + KeyID(ownerPub),
			"wolshare://request?v=1&name=L%C3%A9a&device=" + devicePub + "&owner=XYZ",
		},
	}
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)
	if err := os.WriteFile(vectorsPath, []byte(buf.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestAndroidFile vérifie un fichier d'accès produit par l'application Android (core : ShareTest).
func TestAndroidFile(t *testing.T) {
	v := loadVectors(t)
	data, err := os.ReadFile("../../../protocol/share-android.json")
	if err != nil {
		t.Fatal(err)
	}
	var android struct {
		File     string `json:"file"`
		Revision int64  `json:"revision"`
	}
	if err := json.Unmarshal(data, &android); err != nil {
		t.Fatal(err)
	}
	device, _ := ParseDevicePrivate(v.DevicePrivate)
	opened, err := Open([]byte(android.File), v.OwnerPublic, device, 0)
	if err != nil {
		t.Fatal(err)
	}
	c := opened.Content
	if opened.Revision != android.Revision || c.OwnerName != "Hugo" || len(c.Devices) != 2 {
		t.Fatalf("contenu : %+v", opened)
	}
	a, b := c.Devices[0], c.Devices[1]
	if a.WolPort != 7 || a.BroadcastAddress == nil || *a.BroadcastAddress != "192.168.1.255" || a.SecureOnPassword == nil || a.Agent != nil {
		t.Errorf("premier PC : %+v", a)
	}
	if b.Agent == nil || b.Agent.Key == "" || b.Host != "bureau.local" || len(b.ProbePorts) != 0 {
		t.Errorf("second PC : %+v", b)
	}
}

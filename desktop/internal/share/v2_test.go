package share

import (
	"bytes"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol2"
	"github.com/slaynaw/wakeonlan/desktop/internal/model"
)

// Vecteurs du partage v2 communs avec l'application Android (core/src/test/.../ShareV2Test.kt).
const v2VectorsPath = "../../../protocol/share-v2-vectors.json"

// testSeed dérive des octets fixes d'un libellé (mêmes clés que share-vectors.json).
func testSeed(label string) []byte {
	sum := sha256.Sum256([]byte("wakeonlan-share test " + label))
	return sum[:]
}

// testIdentity est une empreinte wolagent/2 fixe.
func testIdentity(label string) string { return protocol2.PinOf([]byte(label)).String() }

// testShareKey est une clé publique de réception fixe.
func testShareKey(t *testing.T, label string) string {
	t.Helper()
	key, err := ecdh.P256().NewPrivateKey(testSeed(label))
	if err != nil {
		t.Fatal(err)
	}
	return DevicePublic(key)
}

// v2Devices sont les PC du propriétaire : sans agent, agent wolagent/1 seulement, agent joint par son
// empreinte avec sa clé gardée (partagée en v1), agent joint par son empreinte sans clé.
func v2Devices(t *testing.T) []model.Device {
	key := func(label string) string { return base64.RawURLEncoding.EncodeToString(testSeed(label)) }
	return []model.Device{
		{ID: "a", Name: "PC streaming", MAC: mustMAC(t, "3C:7C:3F:1A:2B:4C"), Host: "192.168.1.20", WolPort: 9, ProbePorts: []int{3389, 445}},
		{ID: "b", Name: "Bureau", MAC: mustMAC(t, "00:11:22:33:44:55"), Host: "bureau.local", WolPort: 9,
			ProbePorts: model.DefaultProbePorts(), Agent: &model.AgentSettings{Port: 9770, Key: key("agent b")}},
		{ID: "c", Name: "Salon", MAC: mustMAC(t, "00:11:22:33:44:66"), Host: "192.168.1.30", WolPort: 9,
			ProbePorts: []int{3389}, Agent: &model.AgentSettings{Port: 9770, Key: key("agent c"), Pin: testIdentity("agent c")}},
		{ID: "d", Name: "Atelier", MAC: mustMAC(t, "00:11:22:33:44:77"), Host: "atelier.local", WolPort: 9,
			ProbePorts: []int{22}, Agent: &model.AgentSettings{Port: 9771, Pin: testIdentity("agent d")}},
	}
}

var v2Now = time.Unix(1_790_600_000, 0)

type v2Case struct {
	Name    string          `json:"name"`
	Person  Person          `json:"person"`
	Content json.RawMessage `json:"content"`
}

// v2Cases renvoie l'état du propriétaire et une personne par règle de Content (docs/PARTAGE.md,
// « Partage v2 »), sans le contenu attendu.
func v2Cases(t *testing.T) (*Owner, []v2Case) {
	person := func(label, identity string, rights map[string]Right) Person {
		return Person{Name: "Léa", Device: testShareKey(t, "person "+label), Identity: identity, Added: 1_790_000_000_000, Rights: rights}
	}
	full := map[string]Right{"a": RightFull, "b": RightFull, "c": RightFull, "d": RightFull}
	v2 := person("v2", testIdentity("lea"), map[string]Right{"a": RightView, "b": RightFull, "c": RightFull, "d": RightView})
	expired := person("expired", testIdentity("lea expired"), map[string]Right{"c": RightView})
	moved := person("moved", testIdentity("lea new"), map[string]Right{"c": RightFull})
	wake := person("wake", testIdentity("lea wake"), map[string]Right{"c": RightWake})
	owner := &Owner{Name: "Hugo", Pairings: []Pairing{
		{Person: v2.Device, Device: "c", Identity: v2.Identity, Role: "control", Invite: "AQIDBAUGBwg", Code: "7KQ29MXP", Expires: v2Now.Unix() + 3600},
		{Person: v2.Device, Device: "d", Identity: v2.Identity, Role: "wake", Registered: true},
		{Person: expired.Device, Device: "c", Identity: expired.Identity, Role: "wake", Invite: "CQoLDA0ODxA", Code: "9MXP7KQ2", Expires: v2Now.Unix() - 1},
		{Person: moved.Device, Device: "c", Identity: testIdentity("lea old"), Role: "control", Registered: true},
		{Person: wake.Device, Device: "c", Identity: wake.Identity, Role: "control", Registered: true},
	}}
	return owner, []v2Case{
		{Name: "partage v1 : clé de l'agent pour « démarrer et éteindre », jamais son empreinte", Person: person("v1", "", full)},
		{Name: "partage v2 : invitation, appareil enregistré, et repli v1 pour un agent wolagent/1", Person: v2},
		{Name: "partage v2 avant l'invitation : ni clé ni empreinte", Person: person("pending", testIdentity("lea pending"),
			map[string]Right{"c": RightFull, "d": RightView})},
		{Name: "invitation expirée : empreinte de l'agent sans invitation", Person: expired},
		{Name: "appairage d'une ancienne empreinte : ignoré", Person: moved},
		{Name: "« démarrer » : jamais d'agent", Person: wake},
	}
}

func TestContentV2Rules(t *testing.T) {
	owner, cases := v2Cases(t)
	devices := v2Devices(t)
	content := func(i int) Content { return owner.Content(cases[i].Person, devices, v2Now) }
	agents := func(c Content) map[string]*model.AgentSettings {
		out := map[string]*model.AgentSettings{}
		for _, d := range c.Devices {
			out[d.ID] = d.Agent
		}
		return out
	}

	v1 := content(0)
	a := agents(v1)
	if v1.Identity != "" || len(v1.Invites) != 0 || len(v1.Devices) != 4 || a["a"] != nil || a["d"] != nil {
		t.Fatalf("partage v1 : %+v", v1)
	}
	if a["b"].Key != devices[1].Agent.Key || a["b"].Pin != "" || a["c"].Key != devices[2].Agent.Key || a["c"].Pin != "" {
		t.Errorf("partage v1, agents : %v %v", a["b"], a["c"])
	}

	v2 := content(1)
	a = agents(v2)
	if v2.Identity != testIdentity("lea") || a["a"] != nil || a["b"] == nil || a["b"].Key != devices[1].Agent.Key {
		t.Fatalf("partage v2 : %+v", v2)
	}
	if *a["c"] != (model.AgentSettings{Port: 9770, Pin: testIdentity("agent c")}) || *a["d"] != (model.AgentSettings{Port: 9771, Pin: testIdentity("agent d")}) {
		t.Errorf("partage v2 : agents %v %v", a["c"], a["d"])
	}
	want := []AgentInvite{{Device: "c", Code: "7KQ29MXP", Role: "control", Expires: v2Now.Unix() + 3600}}
	if !reflect.DeepEqual(v2.Invites, want) {
		t.Errorf("invitations : %v", v2.Invites)
	}

	for i, check := range []func(map[string]*model.AgentSettings) bool{
		2: func(a map[string]*model.AgentSettings) bool { return a["c"] == nil && a["d"] == nil },
		3: func(a map[string]*model.AgentSettings) bool {
			return a["c"] != nil && a["c"].Key == "" && a["c"].Pin != ""
		},
		4: func(a map[string]*model.AgentSettings) bool { return a["c"] == nil },
		5: func(a map[string]*model.AgentSettings) bool { return a["c"] == nil },
	} {
		if check == nil {
			continue
		}
		c := content(i)
		if !check(agents(c)) || len(c.Invites) != 0 || c.Identity != cases[i].Person.Identity {
			t.Errorf("%s : %+v", cases[i].Name, c)
		}
	}
	// La clé de l'agent n'est jamais transmise à un appareil qui peut être enregistré par le PC.
	for _, cs := range cases[1:] {
		data, _ := json.Marshal(owner.Content(cs.Person, devices, v2Now))
		if strings.Contains(string(data), devices[2].Agent.Key) {
			t.Errorf("%s : clé de l'agent transmise", cs.Name)
		}
	}
	if devices[2].Agent.Key == "" || devices[2].Agent.Pin == "" {
		t.Error("configuration du propriétaire modifiée")
	}
}

type v2Step struct {
	Op          string       `json:"op"`
	Person      *Person      `json:"person,omitempty"`
	Device      string       `json:"device,omitempty"`
	Pairings    []Pairing    `json:"pairings"`
	Revocations []Revocation `json:"revocations"`
}

type v2RevocationScenario struct {
	People   []Person  `json:"people"`
	Pairings []Pairing `json:"pairings"`
	Steps    []v2Step  `json:"steps"`
}

// v2Revocations décrit des changements de droits successifs et la file de retraits attendue.
func v2Revocations(t *testing.T) v2RevocationScenario {
	lea, paul := testShareKey(t, "person revocation"), testShareKey(t, "person paul")
	i, j := testIdentity("lea"), testIdentity("lea new phone")
	c := Pairing{Person: lea, Device: "c", Identity: i, Role: "control", Registered: true}
	d := Pairing{Person: lea, Device: "d", Identity: i, Role: "wake", Invite: "AQIDBAUGBwg", Code: "7KQ29MXP", Expires: v2Now.Unix() + 600}
	grant := func(device, identity string, rights map[string]Right) *Person {
		name := "Léa"
		if device == paul {
			name = "Paul"
		}
		return &Person{Name: name, Device: device, Identity: identity, Added: 1_790_000_000_000, Rights: rights}
	}
	rev := func(device, identity string) Revocation { return Revocation{Device: device, Identity: identity} }
	sorted := func(r ...Revocation) []Revocation {
		slices.SortFunc(r, func(a, b Revocation) int {
			return strings.Compare(a.Device+" "+a.Identity, b.Device+" "+b.Identity)
		})
		return r
	}
	return v2RevocationScenario{
		People:   []Person{*grant(lea, i, map[string]Right{"c": RightFull, "d": RightView})},
		Pairings: []Pairing{c, d},
		Steps: []v2Step{
			{Op: "grant", Person: grant(lea, i, map[string]Right{"c": RightFull, "d": RightWake}),
				Pairings: []Pairing{c}, Revocations: []Revocation{rev("d", i)}},
			{Op: "grant", Person: grant(lea, j, map[string]Right{"c": RightFull, "d": RightView}),
				Pairings: []Pairing{}, Revocations: sorted(rev("c", i), rev("d", i))},
			{Op: "grant", Person: grant(lea, i, map[string]Right{"c": RightView}),
				Pairings: []Pairing{}, Revocations: sorted(rev("c", j), rev("d", i), rev("d", j))},
			{Op: "grant", Person: grant(paul, i, map[string]Right{"d": RightFull}),
				Pairings: []Pairing{}, Revocations: sorted(rev("c", j), rev("d", j))},
			{Op: "withdraw", Device: lea, Pairings: []Pairing{}, Revocations: sorted(rev("c", i), rev("c", j), rev("d", j))},
			{Op: "withdraw", Device: paul, Pairings: []Pairing{}, Revocations: sorted(rev("c", i), rev("c", j), rev("d", i), rev("d", j))},
		},
	}
}

// runRevocations applique les étapes et compare la file obtenue à celle attendue.
func runRevocations(t *testing.T, s v2RevocationScenario) {
	t.Helper()
	o := &Owner{Name: "Hugo", People: slices.Clone(s.People), Pairings: slices.Clone(s.Pairings)}
	for n, step := range s.Steps {
		switch step.Op {
		case "grant":
			if err := o.Grant(*step.Person); err != nil {
				t.Fatal(err)
			}
		case "withdraw":
			if !o.Withdraw(step.Device) {
				t.Fatalf("étape %d : personne absente", n+1)
			}
		default:
			t.Fatalf("étape %d : %s inconnu", n+1, step.Op)
		}
		if len(o.Pairings) != len(step.Pairings) || (len(step.Pairings) > 0 && !reflect.DeepEqual(o.Pairings, step.Pairings)) {
			t.Errorf("étape %d (%s) : appairages %v, attendu %v", n+1, step.Op, o.Pairings, step.Pairings)
		}
		if !reflect.DeepEqual(o.Revocations, step.Revocations) {
			t.Errorf("étape %d (%s) : retraits %v, attendu %v", n+1, step.Op, o.Revocations, step.Revocations)
		}
	}
}

func TestRevocationQueue(t *testing.T) { runRevocations(t, v2Revocations(t)) }

func TestRequestIdentity(t *testing.T) {
	owner, _ := NewOwnerKey()
	device, _ := NewDeviceKey()
	req := Request{Name: "Léa", Device: DevicePublic(device), Owner: KeyID(OwnerPublic(owner)), Identity: testIdentity("lea")}
	got, err := ParseRequest(req.Link())
	if err != nil || got != req || !strings.Contains(req.Link(), "&id=") {
		t.Fatalf("demande : %+v %v (%s)", got, err, req.Link())
	}
	// Sans empreinte : la demande des versions précédentes, inchangée.
	req.Identity = ""
	if got, err := ParseRequest(req.Link()); err != nil || got != req || strings.Contains(req.Link(), "id=") {
		t.Errorf("demande v1 : %+v %v", got, err)
	}
	for _, bad := range invalidRequests(req) {
		if _, err := ParseRequest(bad); err == nil {
			t.Errorf("demande acceptée : %s", bad)
		}
	}
}

// invalidRequests renvoie des demandes dont l'empreinte est vide, invalide, non canonique ou en double.
func invalidRequests(r Request) []string {
	r.Identity = ""
	base := r.Link()
	pin := testIdentity("lea")
	// Dernier caractère B64 avec des bits de remplissage non nuls : même octets, autre texte.
	noncanonical := pin[:len(pin)-1] + string(b64alphabet[strings.IndexByte(b64alphabet, pin[len(pin)-1])^1])
	return []string{
		base + "&id=",
		base + "&id=AAAA",
		base + "&id=" + pin + "&id=" + pin,
		base + "&id=" + noncanonical,
		base + "&id=%2B" + pin[1:],
	}
}

const b64alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"

type v2Content struct {
	Name      string        `json:"name"`
	Plaintext string        `json:"plaintext"`
	Identity  string        `json:"identity,omitempty"`
	Invites   []AgentInvite `json:"invites,omitempty"`
}

// v2Contents renvoie des contenus v2 valides (codes normalisés à la lecture) et invalides.
func v2Contents(t *testing.T) (valid, invalid []v2Content) {
	devices, _ := json.Marshal(v2Devices(t))
	identity := testIdentity("lea")
	text := func(fields string) string {
		return `{"ownerName":"Hugo","recipientName":"Léa",` + fields + `"devices":` + string(devices) + `}`
	}
	invite := func(device, code, role string, expires int64) string {
		data, _ := json.Marshal(AgentInvite{Device: device, Code: code, Role: role, Expires: expires})
		return string(data)
	}
	ok := invite("c", "7kq2-9mxp", "control", 1_790_603_600)
	valid = []v2Content{
		{Name: "invitation (code normalisé)", Plaintext: text(`"identity":"` + identity + `","invites":[` + ok + `,` +
			invite("d", "9MXP 7KQ2", "wake", 253_402_300_799) + `],`), Identity: identity, Invites: []AgentInvite{
			{Device: "c", Code: "7KQ29MXP", Role: "control", Expires: 1_790_603_600},
			{Device: "d", Code: "9MXP7KQ2", Role: "wake", Expires: 253_402_300_799},
		}},
		{Name: "empreinte sans invitation", Plaintext: text(`"identity":"` + identity + `","invites":[],`), Identity: identity},
		{Name: "partage v1", Plaintext: text(``)},
	}
	bad := func(name, fields string) v2Content { return v2Content{Name: name, Plaintext: text(fields)} }
	invalid = []v2Content{
		bad("empreinte invalide", `"identity":"AAAA",`),
		bad("invitations sans empreinte", `"invites":[`+ok+`],`),
		bad("invitation pour un PC absent", `"identity":"`+identity+`","invites":[`+invite("z", "7KQ29MXP", "control", 1_790_603_600)+`],`),
		bad("invitation pour un PC sans empreinte d'agent", `"identity":"`+identity+`","invites":[`+invite("b", "7KQ29MXP", "control", 1_790_603_600)+`],`),
		bad("deux invitations pour un PC", `"identity":"`+identity+`","invites":[`+ok+`,`+ok+`],`),
		bad("code invalide", `"identity":"`+identity+`","invites":[`+invite("c", "7KQ2-9MXU", "control", 1_790_603_600)+`],`),
		bad("rôle inconnu", `"identity":"`+identity+`","invites":[`+invite("c", "7KQ29MXP", "owner", 1_790_603_600)+`],`),
		bad("expiration nulle", `"identity":"`+identity+`","invites":[`+invite("c", "7KQ29MXP", "control", 0)+`],`),
		bad("expiration trop lointaine", `"identity":"`+identity+`","invites":[`+invite("c", "7KQ29MXP", "control", 253_402_300_800)+`],`),
	}
	return valid, invalid
}

// sealPlain chiffre un contenu en clair pour l'appareil des vecteurs.
func sealPlain(t *testing.T, owner *ecdsa.PrivateKey, device *ecdh.PrivateKey, plain string) []byte {
	t.Helper()
	eph, _ := ecdh.P256().NewPrivateKey(testSeed("ephemeral"))
	data, err := seal(owner, DevicePublic(device), 1, "2026-10-13T12:00:00Z", []byte(plain), eph, testSeed("iv")[:ivLen])
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func testKeys(t *testing.T) (*ecdsa.PrivateKey, *ecdh.PrivateKey) {
	t.Helper()
	owner, err := ecdsa.ParseRawPrivateKey(elliptic.P256(), testSeed("owner"))
	if err != nil {
		t.Fatal(err)
	}
	device, err := ecdh.P256().NewPrivateKey(testSeed("device"))
	if err != nil {
		t.Fatal(err)
	}
	return owner, device
}

func checkContents(t *testing.T, owner *ecdsa.PrivateKey, device *ecdh.PrivateKey, valid, invalid []v2Content) {
	t.Helper()
	for _, c := range valid {
		opened, err := Open(sealPlain(t, owner, device, c.Plaintext), OwnerPublic(owner), device, 0)
		if err != nil {
			t.Errorf("%s : %v", c.Name, err)
			continue
		}
		if opened.Content.Identity != c.Identity || !reflect.DeepEqual(opened.Content.Invites, c.Invites) || len(opened.Content.Devices) != 4 {
			t.Errorf("%s : %+v", c.Name, opened.Content)
		}
	}
	for _, c := range invalid {
		if _, err := Open(sealPlain(t, owner, device, c.Plaintext), OwnerPublic(owner), device, 0); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s : %v", c.Name, err)
		}
	}
}

func TestOpenV2Content(t *testing.T) {
	owner, device := testKeys(t)
	valid, invalid := v2Contents(t)
	checkContents(t, owner, device, valid, invalid)
}

func TestExportKeepsIdentitiesAndRevocations(t *testing.T) {
	key, _ := NewOwnerKey()
	owner, cases := v2Cases(t)
	owner.Key, owner.Name = EncodePrivate(OwnerPrivateBytes(key)), "Hugo"
	owner.People = []Person{cases[1].Person}
	owner.Revocations = []Revocation{{Device: "c", Identity: testIdentity("lea old")}}
	exported, err := owner.Export()
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(exported)
	if strings.Contains(string(data), "7KQ29MXP") || strings.Contains(string(data), "pairings") {
		t.Errorf("appairages exportés : %s", data)
	}
	var back ExportedOwner
	_ = json.Unmarshal(data, &back)
	restored, err := back.Owner()
	if err != nil || restored.People[0].Identity != testIdentity("lea") || !reflect.DeepEqual(restored.Revocations, owner.Revocations) {
		t.Fatalf("sauvegarde relue : %+v %v", restored, err)
	}
	for _, mutate := range []func(*ExportedOwner){
		func(e *ExportedOwner) { e.People[0].Identity = "AAAA" },
		func(e *ExportedOwner) { e.People[0].Rights = map[string]Right{"c": "owner"} },
		func(e *ExportedOwner) { e.Revocations = []Revocation{{Device: "", Identity: testIdentity("x")}} },
		func(e *ExportedOwner) { e.Revocations = []Revocation{{Device: "c", Identity: "AAAA"}} },
	} {
		var e ExportedOwner
		_ = json.Unmarshal(data, &e)
		mutate(&e)
		if _, err := e.Owner(); err == nil {
			t.Errorf("sauvegarde invalide acceptée : %+v", e)
		}
	}
}

// v2Vectors est le contenu de protocol/share-v2-vectors.json.
type v2Vectors struct {
	Comment       string               `json:"comment"`
	OwnerPrivate  string               `json:"ownerPrivate"`
	OwnerPublic   string               `json:"ownerPublic"`
	DevicePrivate string               `json:"devicePrivate"`
	DevicePublic  string               `json:"devicePublic"`
	Now           int64                `json:"now"`
	OwnerName     string               `json:"ownerName"`
	Devices       []model.Device       `json:"devices"`
	Pairings      []Pairing            `json:"pairings"`
	Cases         []v2Case             `json:"cases"`
	Revocations   v2RevocationScenario `json:"revocations"`
	Request       struct {
		Link   string  `json:"link"`
		Fields Request `json:"fields"`
	} `json:"request"`
	InvalidRequests []string    `json:"invalidRequests"`
	Contents        []v2Content `json:"contents"`
	InvalidContents []v2Content `json:"invalidContents"`
}

func buildV2Vectors(t *testing.T) v2Vectors {
	owner, device := testKeys(t)
	o, cases := v2Cases(t)
	devices := v2Devices(t)
	for i := range cases {
		data, err := json.Marshal(o.Content(cases[i].Person, devices, v2Now))
		if err != nil {
			t.Fatal(err)
		}
		cases[i].Content = data
	}
	v := v2Vectors{
		Comment: "Vecteurs du partage v2 (docs/PARTAGE.md, « Partage v2 »), communs à Windows (Go) et Android (Kotlin). " +
			"Générés par desktop/internal/share (TestWriteV2Vectors). Clés : celles de share-vectors.json.",
		OwnerPrivate: EncodePrivate(OwnerPrivateBytes(owner)), OwnerPublic: OwnerPublic(owner),
		DevicePrivate: EncodePrivate(device.Bytes()), DevicePublic: DevicePublic(device),
		Now: v2Now.Unix(), OwnerName: o.Name, Devices: devices, Pairings: o.Pairings, Cases: cases,
		Revocations: v2Revocations(t),
	}
	req := Request{Name: "Léa", Device: DevicePublic(device), Owner: KeyID(OwnerPublic(owner)), Identity: testIdentity("lea")}
	v.Request.Link, v.Request.Fields = req.Link(), req
	v.InvalidRequests = invalidRequests(req)
	v.Contents, v.InvalidContents = v2Contents(t)
	return v
}

func encodeVectors(t *testing.T, v any) []byte {
	t.Helper()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestV2Vectors vérifie que le fichier de vecteurs est à jour et que Go le respecte.
func TestV2Vectors(t *testing.T) {
	data, err := os.ReadFile(v2VectorsPath)
	if err != nil {
		t.Fatal(err)
	}
	if want := encodeVectors(t, buildV2Vectors(t)); !bytes.Equal(data, want) {
		t.Fatal("protocol/share-v2-vectors.json n'est plus à jour : WOL_WRITE_SHARE_VECTORS=1 go test ./internal/share -run TestWriteV2Vectors")
	}
	var v v2Vectors
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	o := &Owner{Name: v.OwnerName, Pairings: v.Pairings}
	for _, c := range v.Cases {
		got, _ := json.Marshal(o.Content(c.Person, v.Devices, time.Unix(v.Now, 0)))
		var want bytes.Buffer
		_ = json.Compact(&want, c.Content)
		if !bytes.Equal(got, want.Bytes()) {
			t.Errorf("%s :\n%s\nattendu\n%s", c.Name, got, want.Bytes())
		}
	}
	runRevocations(t, v.Revocations)
	if r, err := ParseRequest(v.Request.Link); err != nil || r != v.Request.Fields {
		t.Errorf("demande : %+v %v", r, err)
	}
	for _, link := range v.InvalidRequests {
		if _, err := ParseRequest(link); err == nil {
			t.Errorf("demande acceptée : %s", link)
		}
	}
	owner, _ := ParseOwnerPrivate(v.OwnerPrivate)
	device, _ := ParseDevicePrivate(v.DevicePrivate)
	checkContents(t, owner, device, v.Contents, v.InvalidContents)
}

// TestWriteV2Vectors régénère protocol/share-v2-vectors.json (WOL_WRITE_SHARE_VECTORS=1).
func TestWriteV2Vectors(t *testing.T) {
	if os.Getenv("WOL_WRITE_SHARE_VECTORS") != "1" {
		t.Skip("régénération des vecteurs désactivée")
	}
	if err := os.WriteFile(v2VectorsPath, encodeVectors(t, buildV2Vectors(t)), 0o644); err != nil {
		t.Fatal(err)
	}
}

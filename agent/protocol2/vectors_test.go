package protocol2

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/cryptotest"
	"time"
	"unicode"
	"unicode/utf16"
)

var update = flag.Bool("update", false, "régénère protocol/v2-vectors.json à partir de ce paquet")

var vectorsPath = filepath.Join("..", "..", "protocol", "v2-vectors.json")

type vectorFile struct {
	Description string            `json:"description"`
	Protocol    string            `json:"protocol"`
	Pins        pinVectors        `json:"pins"`
	Codes       codeVectors       `json:"codes"`
	Links       linkVectors       `json:"links"`
	Migration   []migrationVector `json:"migration"`
	Roles       roleVectors       `json:"roles"`
	Frames      frameVectors      `json:"frames"`
}

type pinVectors struct {
	Valid   []pinVector `json:"valid"`
	Invalid []pinVector `json:"invalid"`
}

type pinVector struct {
	Name        string `json:"name"`
	Certificate string `json:"certificate"` // DER, en B64
	Pin         string `json:"pin,omitempty"`
	Short       string `json:"short,omitempty"`
	Error       string `json:"error,omitempty"` // "certificate" ou "key_type"
}

type codeVectors struct {
	Encode    []codeEncodeVector    `json:"encode"`
	Normalize []codeNormalizeVector `json:"normalize"`
	Invalid   []codeInvalidVector   `json:"invalid"`
}

type codeEncodeVector struct {
	Bytes string `json:"bytes"` // 5 octets, en hexadécimal
	Code  string `json:"code"`
}

type codeNormalizeVector struct {
	Input   string `json:"input"`
	Code    string `json:"code"`
	Display string `json:"display"`
}

type codeInvalidVector struct {
	Name  string `json:"name"`
	Input string `json:"input"`
}

type linkVectors struct {
	Valid   []linkVector        `json:"valid"`
	Invalid []linkInvalidVector `json:"invalid"`
	Expiry  []linkExpiryVector  `json:"expiry"`
}

type linkVector struct {
	Name      string     `json:"name"`
	Link      string     `json:"link"`
	Canonical bool       `json:"canonical"` // écrit par Link.String : le réécrire redonne le même texte
	Expect    linkFields `json:"expect"`
}

type linkFields struct {
	Name    string `json:"name"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
	MAC     string `json:"mac"`
	Agent   string `json:"agent"`
	Code    string `json:"code"`
	Role    string `json:"role"`
	Expires int64  `json:"expires"`
}

type linkInvalidVector struct {
	Name  string `json:"name"`
	Link  string `json:"link"`
	Field string `json:"field"`
}

type linkExpiryVector struct {
	Link    string `json:"link"`
	Now     int64  `json:"now"`
	Expired bool   `json:"expired"`
}

type migrationVector struct {
	Name        string `json:"name"`
	Key         string `json:"key"`
	Agent       string `json:"agent"`
	Device      string `json:"device"`
	ClientNonce string `json:"nc"`
	AgentNonce  string `json:"na"`
	AgentProof  string `json:"agentProof"`
	DeviceProof string `json:"deviceProof"`
}

type roleVectors struct {
	Order     []string          `json:"order"`
	Authorize []authorizeVector `json:"authorize"`
}

// authorizeVector : réponse de l'agent à la commande cmd (vérifications 1 à 3 du §5), pour une identité
// inconnue puis pour chaque rôle.
type authorizeVector struct {
	Cmd          string `json:"cmd"`
	MinRole      string `json:"minRole"` // rôle minimal, "pairing" (pair, migrate) ou "unknown"
	Unregistered string `json:"unregistered"`
	View         string `json:"view"`
	Wake         string `json:"wake"`
	Control      string `json:"control"`
	Admin        string `json:"admin"`
}

type frameVectors struct {
	MaxRequest  int           `json:"maxRequest"`
	MaxResponse int           `json:"maxResponse"`
	Streams     []frameStream `json:"streams"`
}

type frameStream struct {
	Name     string   `json:"name"`
	Stream   string   `json:"stream"` // octets reçus, en hexadécimal
	Limit    int      `json:"limit"`
	Payloads []string `json:"payloads"` // messages lus, dans l'ordre
	End      string   `json:"end"`      // "eof", "empty", "too_large" ou "truncated"
}

// TestVectors vérifie protocol/v2-vectors.json, partagé avec les applications. Avec -update, le fichier
// est d'abord régénéré à partir de ce paquet ; ses valeurs doivent alors être recalculées indépendamment
// (.github/scripts/check-v2-vectors.py).
func TestVectors(t *testing.T) {
	if *update {
		writeVectors(t, generateVectors(t))
	}
	raw, err := os.ReadFile(vectorsPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc vectorFile
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&doc); err != nil {
		t.Fatal(err)
	}
	if doc.Protocol != ALPN {
		t.Fatalf("protocole %q", doc.Protocol)
	}
	t.Run("pins", func(t *testing.T) { checkPins(t, doc.Pins) })
	t.Run("codes", func(t *testing.T) { checkCodes(t, doc.Codes) })
	t.Run("links", func(t *testing.T) { checkLinks(t, doc.Links) })
	t.Run("migration", func(t *testing.T) { checkMigration(t, doc.Migration) })
	t.Run("roles", func(t *testing.T) { checkRoles(t, doc.Roles) })
	t.Run("frames", func(t *testing.T) { checkFrames(t, doc.Frames) })
}

func checkPins(t *testing.T, v pinVectors) {
	if len(v.Valid) == 0 || len(v.Invalid) == 0 {
		t.Fatal("vecteurs manquants")
	}
	certificates := map[string]string{} // empreinte → certificat
	sameKey := false
	for _, c := range v.Valid {
		pin, err := PinFromCertificate(mustB64(t, c.Certificate))
		if err != nil {
			t.Errorf("%s : %v", c.Name, err)
			continue
		}
		if pin.String() != c.Pin || pin.Short() != c.Short {
			t.Errorf("%s : %s %s, attendu %s %s", c.Name, pin, pin.Short(), c.Pin, c.Short)
		}
		if parsed, err := ParsePin(c.Pin); err != nil || parsed != pin {
			t.Errorf("%s : relecture de l'empreinte : %v", c.Name, err)
		}
		if other, ok := certificates[c.Pin]; ok && other != c.Certificate {
			sameKey = true
		}
		certificates[c.Pin] = c.Certificate
	}
	if !sameKey {
		t.Error("il manque deux certificats différents d'une même clé")
	}
	for _, c := range v.Invalid {
		_, err := PinFromCertificate(mustB64(t, c.Certificate))
		want := map[string]error{"certificate": ErrCertificate, "key_type": ErrKeyType}[c.Error]
		if want == nil || !errors.Is(err, want) {
			t.Errorf("%s : %v, attendu %s", c.Name, err, c.Error)
		}
	}
}

func checkCodes(t *testing.T, v codeVectors) {
	if len(v.Encode) == 0 || len(v.Normalize) == 0 || len(v.Invalid) == 0 {
		t.Fatal("vecteurs manquants")
	}
	for _, c := range v.Encode {
		raw, err := hex.DecodeString(c.Bytes)
		if err != nil || len(raw) != 5 {
			t.Fatalf("%s : %v", c.Bytes, err)
		}
		if got := EncodeCode([5]byte(raw)); got != c.Code {
			t.Errorf("%s : %s, attendu %s", c.Bytes, got, c.Code)
		}
	}
	for _, c := range v.Normalize {
		got, err := NormalizeCode(c.Input)
		if err != nil || got != c.Code || FormatCode(got) != c.Display {
			t.Errorf("%q : %q (%v), attendu %q", c.Input, got, err, c.Code)
		}
	}
	for _, c := range v.Invalid {
		if got, err := NormalizeCode(c.Input); err == nil {
			t.Errorf("%s : %q accepté (%s)", c.Name, c.Input, got)
		}
	}
}

func checkLinks(t *testing.T, v linkVectors) {
	if len(v.Valid) == 0 || len(v.Invalid) == 0 || len(v.Expiry) == 0 {
		t.Fatal("vecteurs manquants")
	}
	for _, c := range v.Valid {
		l, err := ParseLink(c.Link)
		if err != nil {
			t.Errorf("%s : %v", c.Name, err)
			continue
		}
		if got := fieldsOf(l); got != c.Expect {
			t.Errorf("%s :\n%+v\nattendu\n%+v", c.Name, got, c.Expect)
		}
		if c.Canonical && l.String() != c.Link {
			t.Errorf("%s : réécrit\n%s", c.Name, l.String())
		}
		if again, err := ParseLink(l.String()); err != nil || again != l {
			t.Errorf("%s : relecture du lien réécrit : %+v, %v", c.Name, again, err)
		}
	}
	for _, c := range v.Invalid {
		_, err := ParseLink(c.Link)
		var linkErr *LinkError
		if !errors.As(err, &linkErr) || linkErr.Field != c.Field {
			t.Errorf("%s : %v, attendu %s", c.Name, err, c.Field)
		}
	}
	for _, c := range v.Expiry {
		l, err := ParseLink(c.Link)
		if err != nil || l.Expired(c.Now) != c.Expired {
			t.Errorf("%s à %d : %v", c.Link, c.Now, err)
		}
	}
}

func fieldsOf(l Link) linkFields {
	return linkFields{
		Name: l.Name, Host: l.Host, Port: l.Port, MAC: l.MAC, Agent: l.Agent.String(), Code: l.Code,
		Role: l.Role.String(), Expires: l.Expires,
	}
}

func checkMigration(t *testing.T, vectors []migrationVector) {
	if len(vectors) == 0 {
		t.Fatal("vecteurs manquants")
	}
	for _, c := range vectors {
		key := mustB64(t, c.Key)
		agent, err1 := ParsePin(c.Agent)
		device, err2 := ParsePin(c.Device)
		nc, err3 := ParseNonce(c.ClientNonce)
		na, err4 := ParseNonce(c.AgentNonce)
		pa, err5 := ParseProof(c.AgentProof)
		pd, err6 := ParseProof(c.DeviceProof)
		if err := errors.Join(err1, err2, err3, err4, err5, err6); err != nil {
			t.Fatalf("%s : %v", c.Name, err)
		}
		if got := AgentProof(key, agent, device, nc, na); !got.Equal(pa) {
			t.Errorf("%s : Pa %s, attendu %s", c.Name, got, pa)
		}
		if got := DeviceProof(key, agent, device, nc, na); !got.Equal(pd) {
			t.Errorf("%s : Pd %s, attendu %s", c.Name, got, pd)
		}
		if pa.Equal(pd) {
			t.Errorf("%s : Pa et Pd identiques", c.Name)
		}
	}
}

func checkRoles(t *testing.T, v roleVectors) {
	var order []string
	for _, r := range Roles {
		order = append(order, r.String())
	}
	if !slices.Equal(order, v.Order) {
		t.Errorf("ordre des rôles %v, attendu %v", order, v.Order)
	}
	covered := map[string]bool{}
	for _, c := range v.Authorize {
		covered[c.Cmd] = true
		if got := authorizeRow(c.Cmd); got != c {
			t.Errorf("%q :\n%+v\nattendu\n%+v", c.Cmd, got, c)
		}
	}
	for _, cmd := range Commands {
		if !covered[cmd] {
			t.Errorf("commande %s absente des vecteurs", cmd)
		}
	}
}

func authorizeRow(cmd string) authorizeVector {
	minimum, known := MinRole(cmd)
	kind := minimum.String()
	switch {
	case IsEnrollment(cmd):
		kind = "pairing"
	case !known:
		kind = "unknown"
	}
	return authorizeVector{
		Cmd: cmd, MinRole: kind, Unregistered: Authorize(false, 0, cmd),
		View: Authorize(true, RoleView, cmd), Wake: Authorize(true, RoleWake, cmd),
		Control: Authorize(true, RoleControl, cmd), Admin: Authorize(true, RoleAdmin, cmd),
	}
}

func checkFrames(t *testing.T, v frameVectors) {
	if v.MaxRequest != MaxRequestBytes || v.MaxResponse != MaxResponseBytes {
		t.Errorf("limites %d et %d", v.MaxRequest, v.MaxResponse)
	}
	if len(v.Streams) == 0 {
		t.Fatal("vecteurs manquants")
	}
	for _, c := range v.Streams {
		stream, err := hex.DecodeString(c.Stream)
		if err != nil {
			t.Fatalf("%s : %v", c.Name, err)
		}
		payloads, end := readStream(stream, c.Limit)
		if !slices.Equal(payloads, c.Payloads) || end != c.End {
			t.Errorf("%s : %q puis %s, attendu %q puis %s", c.Name, payloads, end, c.Payloads, c.End)
		}
		if c.End != "eof" {
			continue
		}
		var written bytes.Buffer
		for _, p := range c.Payloads {
			if err := WriteFrame(&written, []byte(p), c.Limit); err != nil {
				t.Fatalf("%s : %v", c.Name, err)
			}
		}
		if !bytes.Equal(written.Bytes(), stream) {
			t.Errorf("%s : écrit %x", c.Name, written.Bytes())
		}
	}
}

// readStream lit les trames d'un flux jusqu'à sa fin ou à la première erreur.
func readStream(stream []byte, limit int) ([]string, string) {
	r := bytes.NewReader(stream)
	payloads := []string{}
	for {
		payload, err := ReadFrame(r, limit)
		switch {
		case err == nil:
			payloads = append(payloads, string(payload))
			continue
		case err == io.EOF:
			return payloads, "eof"
		case errors.Is(err, ErrEmptyFrame):
			return payloads, "empty"
		case errors.Is(err, ErrFrameTooLarge):
			return payloads, "too_large"
		case errors.Is(err, ErrTruncatedFrame):
			return payloads, "truncated"
		}
		return payloads, err.Error()
	}
}

func mustB64(t *testing.T, s string) []byte {
	t.Helper()
	raw, err := b64.DecodeString(s)
	if err != nil {
		t.Fatalf("%q : %v", s, err)
	}
	return raw
}

// generateVectors produit les vecteurs à partir d'entrées fixes. Les clés sont dérivées de libellés et
// le hasard des certificats est rendu reproductible : régénérer le fichier ne change rien, sauf
// évolution de la bibliothèque standard.
func generateVectors(t *testing.T) vectorFile {
	cryptotest.SetGlobalRandom(t, 2)
	agentKey, deviceKey := vectorKey("agent"), vectorKey("appareil")
	day := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	agentCert := must(NewCertificate(agentKey, day))
	deviceCert := must(NewCertificate(deviceKey, day))
	agentPin, devicePin := PinOf(mustSPKI(agentCert)), PinOf(mustSPKI(deviceCert))

	return vectorFile{
		Description: "Vecteurs de test du protocole wolagent/2 (docs/PROTOCOLE-V2.md, §14), partagés par " +
			"l'agent (Go) et les applications (Kotlin). Produits par agent/protocol2 (go test ./protocol2 " +
			"-run TestVectors -update), recalculés indépendamment par .github/scripts/check-v2-vectors.py " +
			"(Python et OpenSSL). Ne pas modifier à la main. B64 : Base64 URL sans remplissage.",
		Protocol:  ALPN,
		Pins:      generatePins(t, agentKey, deviceKey, agentCert, deviceCert, day),
		Codes:     generateCodes(t),
		Links:     generateLinks(t, agentPin),
		Migration: generateMigration(agentPin, devicePin),
		Roles:     generateRoles(),
		Frames:    generateFrames(t),
	}
}

func generatePins(t *testing.T, agentKey, deviceKey *ecdsa.PrivateKey, agentCert, deviceCert []byte, day time.Time) pinVectors {
	valid := func(name string, der []byte) pinVector {
		pin, err := PinFromCertificate(der)
		if err != nil {
			t.Fatalf("%s : %v", name, err)
		}
		return pinVector{Name: name, Certificate: b64.EncodeToString(der), Pin: pin.String(), Short: pin.Short()}
	}
	// Un certificat d'un tout autre profil, pour une clé P-256 : seule la clé compte.
	foreign := must(x509.CreateCertificate(rand.Reader, &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "autre profil", Organization: []string{"Vecteurs"}},
		NotBefore:             time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:              time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		BasicConstraintsValid: true,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}, &x509.Certificate{Subject: pkix.Name{CommonName: "autre émetteur"}}, deviceKey.Public(), vectorKey("émetteur")))

	selfSigned := func(pub, priv any) string {
		template := &x509.Certificate{
			SerialNumber: big.NewInt(2),
			Subject:      pkix.Name{CommonName: "wolagent"},
			NotBefore:    day.Add(-24 * time.Hour),
			NotAfter:     time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC),
		}
		return b64.EncodeToString(must(x509.CreateCertificate(rand.Reader, template, template, pub, priv)))
	}
	rsaKey := must(rsa.GenerateKey(rand.Reader, 2048))
	p384Key := must(ecdsa.GenerateKey(elliptic.P384(), rand.Reader))
	edSeed := sha256.Sum256([]byte("wolagent/2 vecteurs : ed25519"))
	edKey := ed25519.NewKeyFromSeed(edSeed[:])

	return pinVectors{
		Valid: []pinVector{
			valid("agent : certificat créé le 12/10/2026", agentCert),
			valid("agent : certificat recréé le 12/01/2027 (autre certificat, même clé, même empreinte)",
				must(NewCertificate(agentKey, day.AddDate(0, 3, 0)))),
			valid("appareil", deviceCert),
			valid("appareil : autre profil (sujet, émetteur, dates dépassées, extensions, signature d'une autre clé), ignoré",
				foreign),
		},
		Invalid: []pinVector{
			{Name: "clé RSA 2048", Certificate: selfSigned(rsaKey.Public(), rsaKey), Error: "key_type"},
			{Name: "clé ECDSA P-384", Certificate: selfSigned(p384Key.Public(), p384Key), Error: "key_type"},
			{Name: "clé Ed25519", Certificate: selfSigned(edKey.Public(), edKey), Error: "key_type"},
			{Name: "pas un certificat", Certificate: b64.EncodeToString([]byte(ALPN)), Error: "certificate"},
			{Name: "certificat tronqué", Certificate: b64.EncodeToString(agentCert[:len(agentCert)/2]), Error: "certificate"},
		},
	}
}

func generateCodes(t *testing.T) codeVectors {
	var v codeVectors
	for _, h := range []string{"0000000000", "ffffffffff", "0123456789", "8421084210", "deadbeef42"} {
		raw := must(hex.DecodeString(h))
		v.Encode = append(v.Encode, codeEncodeVector{Bytes: h, Code: EncodeCode([5]byte(raw))})
	}
	for _, input := range []string{"7KQ29MXP", "7kq2-9mxp", " 7KQ2 9MXP ", "7KQ2--9MXP", "oil0-OIL0", "abcd efgh", "JKMN-PQRS", "tvwx-yz00"} {
		code := must(NormalizeCode(input))
		v.Normalize = append(v.Normalize, codeNormalizeVector{Input: input, Code: code, Display: FormatCode(code)})
	}
	v.Invalid = []codeInvalidVector{
		{"vide", ""},
		{"7 caractères", "7KQ2-9MX"},
		{"9 caractères", "7KQ2-9MXPA"},
		{"U, absent de l'alphabet", "7KQ2-9MXU"},
		{"tiret bas", "7KQ2_9MXP"},
		{"signe plus", "7KQ2+9MXP"},
		{"tabulation", "7KQ2\t9MXP"},
		{"espace insécable", "7KQ2 9MXP"},
		{"tiret demi-cadratin", "7KQ2–9MXP"},
		{"chiffre pleine largeur", "７KQ29MXP"},
		{"i sans point (I une fois en majuscule, hors ASCII)", "7KQ29MXı"},
		{"s long (S une fois en majuscule, hors ASCII)", "7KQ29MXſ"},
	}
	for _, c := range v.Invalid {
		if _, err := NormalizeCode(c.Input); err == nil {
			t.Fatalf("%s : accepté", c.Name)
		}
	}
	return v
}

func generateLinks(t *testing.T, agent Pin) linkVectors {
	var v linkVectors
	pin := agent.String()
	base := "wolagent://pair?v=2&h=pc-bureau.local&a=" + pin + "&c=7KQ29MXP"
	padded := func(size int) string { return base + "&x=" + strings.Repeat("a", size-len(base)-3) }
	add := func(name, link string) {
		l, err := ParseLink(link)
		if err != nil {
			t.Fatalf("%s : %v", name, err)
		}
		v.Valid = append(v.Valid, linkVector{Name: name, Link: link, Canonical: l.String() == link, Expect: fieldsOf(l)})
	}
	for _, c := range []struct {
		name string
		link Link
	}{
		{"écrit par l'agent : tous les paramètres", Link{Name: "PC de Salon & Jeux", Host: "192.168.1.20", Port: 9770,
			MAC: "AA:BB:CC:DD:EE:FF", Agent: agent, Code: "7KQ29MXP", Role: RoleAdmin, Expires: 1791763800}},
		{"écrit par l'agent : port 9771, sans MAC, rôle ni expiration", Link{Name: "serveur", Host: "serveur.local",
			Port: 9771, Agent: agent, Code: "0123ABCD"}},
		{"écrit par l'agent : nom accentué, IPv6 avec zone", Link{Name: "PC de Léa", Host: "fe80::1%eth0", Port: 9770,
			Agent: agent, Code: "ZZZZ0000", Role: RoleView, Expires: 1791763800}},
	} {
		add(c.name, c.link.String())
		if !v.Valid[len(v.Valid)-1].Canonical {
			t.Fatalf("%s : pas canonique", c.name)
		}
	}
	longHost := "poste-de-travail-du-bureau-principal.salle-informatique.maison.local"
	for _, c := range []struct{ name, link string }{
		{"minimal : nom, port, MAC, rôle et expiration absents", base},
		{"préfixe en majuscules, espaces autour, paramètres vides ou inconnus, code en minuscules, MAC avec tirets",
			"  WOLAGENT://PAIR?v=2&&h=192.168.1.20&a=" + pin + "&c=7kq2-9mxp&x=1&y=&m=aa-bb-cc-dd-ee-ff\n"},
		{"nom : caractères de contrôle et espaces retirés", base + "&n=%20%20Bureau%09%0A%20"},
		{"nom : U+0085 (contrôle) retiré, espace insécable gardée", base + "&n=%C2%85Nom%C2%A0"},
		{"nom : « + » lu comme une espace", base + "&n=PC+de+L%C3%A9a"},
		{"nom : UTF-8 non encodé", base + "&n=Léa"},
		{"nom vide : l'hôte", base + "&n="},
		{"nom de 64 unités UTF-16 : gardé entier", base + "&n=" + strings.Repeat("a", 62) + "%F0%9F%98%80"},
		{"nom de 65 unités UTF-16 : coupé avant l'émoji", base + "&n=" + strings.Repeat("a", 63) + "%F0%9F%98%80"},
		{"hôte de 68 caractères sans nom : ses 64 premiers caractères", strings.Replace(base, "pc-bureau.local", longHost, 1)},
		{"empreinte en partie encodée (%XX)", strings.Replace(base, "a="+pin, fmt.Sprintf("a=%%%02X%s", pin[0], pin[1:]), 1)},
		{"port avec un zéro initial", base + "&p=09770"},
		{"rôle wake, expiration maximale", base + "&r=wake&e=253402300799"},
		{"lien de 2 048 octets", padded(MaxLinkLength)},
	} {
		add(c.name, c.link)
	}

	last := strings.IndexByte(base64URL, pin[len(pin)-1])
	nonCanonical := pin[:len(pin)-1] + string(base64URL[last|1])
	for _, c := range []struct{ name, field, link string }{
		{"autre schéma", "format", "https://example.com/pair?v=2&h=pc&a=" + pin + "&c=7KQ29MXP"},
		{"préfixe incomplet", "format", "wolagent://pair"},
		{"préfixe avec un i sans point", "format", strings.Replace(base, "pair", "paır", 1)},
		{"paramètre sans nom", "format", base + "&=x"},
		{"paramètre sans « = »", "format", base + "&x"},
		{"encodage %zz", "format", base + "&n=%zz"},
		{"encodage incomplet", "format", base + "&x=%4"},
		{"UTF-8 tronqué", "format", base + "&n=%C3"},
		{"octets hors UTF-8", "format", base + "&n=%FF%FE"},
		{"lien de 2 049 octets", "format", padded(MaxLinkLength + 1)},
		{"version absente", "v", strings.Replace(base, "v=2&", "", 1)},
		{"version 1", "v", strings.Replace(base, "v=2", "v=1", 1)},
		{"version 02", "v", strings.Replace(base, "v=2", "v=02", 1)},
		{"version en double", "duplicate", base + "&v=2"},
		{"paramètre inconnu en double", "duplicate", base + "&x=1&x=1"},
		{"hôte absent", "h", strings.Replace(base, "h=pc-bureau.local&", "", 1)},
		{"hôte vide", "h", strings.Replace(base, "h=pc-bureau.local", "h=", 1)},
		{"hôte : IPv4 incomplète", "h", strings.Replace(base, "h=pc-bureau.local", "h=192.168.1", 1)},
		{"hôte : 256.1.1.1", "h", strings.Replace(base, "h=pc-bureau.local", "h=256.1.1.1", 1)},
		{"hôte : tiret bas", "h", strings.Replace(base, "h=pc-bureau.local", "h=pc_bureau", 1)},
		{"hôte : label commençant par un tiret", "h", strings.Replace(base, "h=pc-bureau.local", "h=-pc.local", 1)},
		{"hôte : espace", "h", strings.Replace(base, "h=pc-bureau.local", "h=pc%20bureau", 1)},
		{"empreinte absente", "a", strings.Replace(base, "&a="+pin, "", 1)},
		{"empreinte trop courte", "a", strings.Replace(base, pin, pin[:42], 1)},
		{"empreinte avec remplissage", "a", strings.Replace(base, pin, pin+"%3D", 1)},
		{"empreinte non canonique (bits de fin non nuls)", "a", strings.Replace(base, pin, nonCanonical, 1)},
		{"empreinte en Base64 standard (+)", "a", strings.Replace(base, pin, "%2B"+pin[1:], 1)},
		{"code absent", "c", strings.Replace(base, "&c=7KQ29MXP", "", 1)},
		{"code invalide", "c", strings.Replace(base, "c=7KQ29MXP", "c=7KQ2-9MXU", 1)},
		{"port 0", "p", base + "&p=0"},
		{"port 65536", "p", base + "&p=65536"},
		{"port vide", "p", base + "&p="},
		{"port « +80 » (+ lu comme une espace)", "p", base + "&p=+80"},
		{"port %2B80", "p", base + "&p=%2B80"},
		{"port de 6 chiffres", "p", base + "&p=009770"},
		{"MAC nulle", "m", base + "&m=00:00:00:00:00:00"},
		{"MAC de broadcast", "m", base + "&m=ff:ff:ff:ff:ff:ff"},
		{"MAC incomplète", "m", base + "&m=AA:BB:CC:DD:EE"},
		{"MAC à séparateurs mélangés", "m", base + "&m=AA:BB-CC:DD:EE:FF"},
		{"MAC sans séparateur", "m", base + "&m=AABBCCDDEEFF"},
		{"MAC non hexadécimale", "m", base + "&m=GG:BB:CC:DD:EE:FF"},
		{"MAC vide", "m", base + "&m="},
		{"rôle inconnu", "r", base + "&r=owner"},
		{"rôle en majuscules", "r", base + "&r=Admin"},
		{"rôle vide", "r", base + "&r="},
		{"expiration 0", "e", base + "&e=0"},
		{"expiration après l'an 9999", "e", base + "&e=253402300800"},
		{"expiration négative", "e", base + "&e=-1"},
		{"expiration décimale", "e", base + "&e=1.5"},
	} {
		var linkErr *LinkError
		if _, err := ParseLink(c.link); !errors.As(err, &linkErr) || linkErr.Field != c.field {
			t.Fatalf("%s : %v, attendu %s", c.name, err, c.field)
		}
		v.Invalid = append(v.Invalid, linkInvalidVector{Name: c.name, Link: c.link, Field: c.field})
	}

	expiring := base + "&e=1791763800"
	v.Expiry = []linkExpiryVector{
		{Link: expiring, Now: 1791763799, Expired: false},
		{Link: expiring, Now: 1791763800, Expired: true},
		{Link: expiring, Now: 1791850200, Expired: true},
		{Link: base, Now: MaxExpires, Expired: false},
	}
	return v
}

// base64URL est l'alphabet de B64, dans l'ordre des valeurs.
const base64URL = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"

func generateMigration(agent, device Pin) []migrationVector {
	sequence := func(first byte) (b [32]byte) {
		for i := range b {
			b[i] = first + byte(i)
		}
		return b
	}
	derived := func(label string) [32]byte { return sha256.Sum256([]byte("wolagent/2 vecteurs : " + label)) }
	vector := func(name string, key [32]byte, agent, device Pin, nc, na Nonce) migrationVector {
		return migrationVector{
			Name: name, Key: b64.EncodeToString(key[:]), Agent: agent.String(), Device: device.String(),
			ClientNonce: nc.String(), AgentNonce: na.String(),
			AgentProof:  AgentProof(key[:], agent, device, nc, na).String(),
			DeviceProof: DeviceProof(key[:], agent, device, nc, na).String(),
		}
	}
	key, nc, na := sequence(0x00), Nonce(sequence(0x40)), Nonce(sequence(0x80))
	return []migrationVector{
		vector("octets successifs", key, agent, device, nc, na),
		vector("valeurs dérivées", derived("K"), agent, device, Nonce(derived("Nc")), Nonce(derived("Na"))),
		vector("empreintes inversées, comme vues par un intermédiaire : autres preuves", key, device, agent, nc, na),
	}
}

func generateRoles() roleVectors {
	var v roleVectors
	for _, r := range Roles {
		v.Order = append(v.Order, r.String())
	}
	for _, cmd := range append(slices.Clone(Commands), "", "STATUS", "status ", "devices.", "update") {
		v.Authorize = append(v.Authorize, authorizeRow(cmd))
	}
	return v
}

func generateFrames(t *testing.T) frameVectors {
	frame := func(payload string) string {
		var b bytes.Buffer
		if err := WriteFrame(&b, []byte(payload), MaxResponseBytes); err != nil {
			t.Fatal(err)
		}
		return hex.EncodeToString(b.Bytes())
	}
	status := `{"id":1,"cmd":"status"}`
	shutdown := `{"id":2,"cmd":"shutdown","delay":0,"force":false}`
	response := `{"id":1,"ok":true,"code":"ok","message":"Extinction dans 0 s","hostname":"PC-SALON-É"}`
	v := frameVectors{MaxRequest: MaxRequestBytes, MaxResponse: MaxResponseBytes}
	for _, c := range []struct {
		name, stream string
		limit        int
		end          string
	}{
		{"requête", frame(status), MaxRequestBytes, "eof"},
		{"deux requêtes à la suite", frame(status) + frame(shutdown), MaxRequestBytes, "eof"},
		{"réponse en UTF-8", frame(response), MaxResponseBytes, "eof"},
		{"flux vide", "", MaxRequestBytes, "eof"},
		{"message à la limite (limite réduite à 23 octets)", frame(status), len(status), "eof"},
		{"message d'un octet de trop (limite réduite à 22 octets)", frame(status), len(status) - 1, "too_large"},
		{"longueur nulle", "00000000", MaxRequestBytes, "empty"},
		{"requête trop longue (16 385 octets annoncés)", "00004001", MaxRequestBytes, "too_large"},
		{"réponse trop longue (4 194 305 octets annoncés)", "00400001", MaxResponseBytes, "too_large"},
		{"longueur 0xFFFFFFFF", "ffffffff", MaxResponseBytes, "too_large"},
		{"longueur tronquée", "0000", MaxRequestBytes, "truncated"},
		{"message tronqué", "0000000a" + hex.EncodeToString([]byte(`{"id"`)), MaxRequestBytes, "truncated"},
		{"trame puis trame tronquée", frame(status) + "00000010" + hex.EncodeToString([]byte(`{"i`)), MaxRequestBytes, "truncated"},
		{"trame puis longueur nulle", frame(status) + "00000000", MaxRequestBytes, "empty"},
	} {
		payloads, end := readStream(must(hex.DecodeString(c.stream)), c.limit)
		if end != c.end {
			t.Fatalf("%s : %s, attendu %s", c.name, end, c.end)
		}
		v.Streams = append(v.Streams, frameStream{Name: c.name, Stream: c.stream, Limit: c.limit, Payloads: payloads, End: end})
	}
	return v
}

// vectorKey dérive une clé P-256 fixe d'un libellé.
func vectorKey(label string) *ecdsa.PrivateKey {
	d := sha256.Sum256([]byte("wolagent/2 vecteurs : clé " + label))
	return must(ecdsa.ParseRawPrivateKey(elliptic.P256(), d[:]))
}

func mustSPKI(der []byte) []byte { return must(x509.ParseCertificate(der)).RawSubjectPublicKeyInfo }

// must renvoie la valeur, ou interrompt la génération des vecteurs sur une erreur.
func must[V any](v V, err error) V {
	if err != nil {
		panic(err)
	}
	return v
}

func writeVectors(t *testing.T, doc vectorFile) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(vectorsPath, escapeInvisible(buf.Bytes()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// escapeInvisible écrit en \uXXXX les caractères non ASCII invisibles ou ambigus à l'œil (espaces,
// contrôles, formats), pour un fichier sans piège à la relecture. Le JSON n'a de caractères non ASCII
// que dans ses chaînes, où l'échappement ne change pas la valeur.
func escapeInvisible(data []byte) []byte {
	var out bytes.Buffer
	for _, r := range string(data) {
		if r < 0x80 || unicode.In(r, unicode.L, unicode.M, unicode.N, unicode.P, unicode.S) {
			out.WriteRune(r)
			continue
		}
		for _, unit := range utf16.Encode([]rune{r}) {
			fmt.Fprintf(&out, `\u%04x`, unit)
		}
	}
	return out.Bytes()
}

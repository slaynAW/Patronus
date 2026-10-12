package protocol2

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol"
)

func TestCertificateProfile(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	spki, err := x509.MarshalPKIXPublicKey(key.Public())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 12, 15, 4, 5, 6, time.UTC)
	first, err1 := NewCertificate(key, now)
	second, err2 := NewCertificate(key, now)
	if err := errors.Join(err1, err2); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(first, second) {
		t.Fatal("deux certificats identiques : numéro de série non aléatoire")
	}
	for _, der := range [][]byte{first, second} {
		if pin, err := PinFromCertificate(der); err != nil || pin != PinOf(spki) {
			t.Fatalf("empreinte %s (%v), attendu %s", pin, err, PinOf(spki))
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			t.Fatal(err)
		}
		switch {
		case cert.Subject.String() != "CN=wolagent" || cert.Issuer.String() != "CN=wolagent":
			t.Errorf("sujet %s, émetteur %s", cert.Subject, cert.Issuer)
		case cert.SerialNumber.Sign() <= 0 || len(cert.SerialNumber.Bytes()) != 16:
			t.Errorf("numéro de série %x", cert.SerialNumber)
		case !cert.NotBefore.Equal(time.Date(2026, 10, 11, 15, 4, 5, 0, time.UTC)):
			t.Errorf("début de validité %s", cert.NotBefore)
		case !cert.NotAfter.Equal(time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)):
			t.Errorf("fin de validité %s", cert.NotAfter)
		case cert.SignatureAlgorithm != x509.ECDSAWithSHA256:
			t.Errorf("signature %s", cert.SignatureAlgorithm)
		}
		if err := cert.CheckSignature(cert.SignatureAlgorithm, cert.RawTBSCertificate, cert.Signature); err != nil {
			t.Errorf("autosignature : %v", err)
		}
	}
	p384, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []*ecdsa.PrivateKey{p384, nil} {
		if _, err := NewCertificate(k, now); !errors.Is(err, ErrKeyType) {
			t.Errorf("clé refusée attendue : %v", err)
		}
	}
}

func TestPinText(t *testing.T) {
	var p Pin
	for i := range p {
		p[i] = byte(i * 7)
	}
	data, err := json.Marshal(map[string]Pin{"device": p})
	if err != nil || string(data) != `{"device":"`+p.String()+`"}` {
		t.Fatalf("%s %v", data, err)
	}
	var back map[string]Pin
	if err := json.Unmarshal(data, &back); err != nil || back["device"] != p {
		t.Fatalf("%v %v", back, err)
	}
	s := p.String()
	for _, bad := range []string{"", s[:42], s + "A", s + "=", s[:20] + "\n" + s[21:], s[:20] + "\n" + s[20:], s[:42] + "+", " " + s[1:]} {
		if _, err := ParsePin(bad); !errors.Is(err, ErrPin) {
			t.Errorf("%q accepté", bad)
		}
	}
	if err := json.Unmarshal([]byte(`{"device":"court"}`), &back); err == nil {
		t.Error("empreinte invalide acceptée dans le JSON")
	}
}

func TestShortPin(t *testing.T) {
	var zero, ones, pattern Pin
	for i := range ones {
		ones[i] = 0xff
	}
	copy(pattern[:], []byte{0x08, 0x42, 0x10, 0x84, 0x21, 0x08, 0x42, 0x10})
	for pin, want := range map[Pin]string{zero: "0000-0000-0000", ones: "ZZZZ-ZZZZ-ZZZZ", pattern: "1111-1111-1111"} {
		if got := pin.Short(); got != want {
			t.Errorf("%s : %s, attendu %s", pin, got, want)
		}
	}
}

func TestCodes(t *testing.T) {
	seen := map[string]bool{}
	for range 100 {
		code := NewCode()
		for _, input := range []string{code, FormatCode(code), strings.ToLower(FormatCode(code))} {
			if got, err := NormalizeCode(input); err != nil || got != code {
				t.Fatalf("%q : %q %v", input, got, err)
			}
		}
		seen[code] = true
	}
	if len(seen) < 99 {
		t.Errorf("%d codes différents sur 100", len(seen))
	}
	if FormatCode("ABC") != "ABC" {
		t.Error("code de mauvaise longueur transformé")
	}
}

func TestRoles(t *testing.T) {
	for _, r := range Roles {
		text, err := r.MarshalText()
		var back Role
		if err != nil || back.UnmarshalText(text) != nil || back != r {
			t.Errorf("%d : %s %v", r, text, err)
		}
	}
	var r Role
	if r.UnmarshalText([]byte("owner")) == nil || r.UnmarshalText([]byte("Admin")) == nil {
		t.Error("rôle inconnu accepté")
	}
	for _, bad := range []Role{0, RoleAdmin + 1, 255} {
		if _, err := bad.MarshalText(); err == nil || bad.String() != "" {
			t.Errorf("rôle %d écrit", bad)
		}
		if got := Authorize(true, bad, "status"); got != CodeInsufficientRole {
			t.Errorf("rôle %d : %s", bad, got)
		}
	}
	if len(Commands) != len(minRoles)+2 {
		t.Errorf("%d commandes, %d avec un rôle minimal", len(Commands), len(minRoles))
	}
	for _, cmd := range Commands {
		if _, ok := MinRole(cmd); ok == IsEnrollment(cmd) {
			t.Errorf("%s : rôle minimal et appairage", cmd)
		}
		// Rôles cumulatifs : ce qu'un rôle permet, tout rôle supérieur le permet.
		for i := 1; i < len(Roles); i++ {
			if Authorize(true, Roles[i-1], cmd) == CodeOK && Authorize(true, Roles[i], cmd) != CodeOK {
				t.Errorf("%s permis à %s mais pas à %s", cmd, Roles[i-1], Roles[i])
			}
		}
	}
}

func TestLinkRoundTrip(t *testing.T) {
	agent := PinOf([]byte("agent"))
	for _, l := range []Link{
		{Name: "PC", Host: "10.0.0.1", Port: 1, Agent: agent, Code: "00000000"},
		{Name: strings.Repeat("é", MaxPCNameLength), Host: "::1", Port: 65535, MAC: "01:23:45:67:89:AB", Agent: agent,
			Code: "ZZZZZZZZ", Role: RoleWake, Expires: MaxExpires},
		{Name: "a+b & c=d %41 #1", Host: "pc.local.", Port: 9770, Agent: agent, Code: "7KQ29MXP", Role: RoleControl, Expires: 1},
	} {
		if got, err := ParseLink(l.String()); err != nil || got != l {
			t.Errorf("%s :\n%+v %v", l, got, err)
		}
	}
	// Un lien v=1 n'est pas lu ici : l'application le confie à l'ancien lecteur.
	var linkErr *LinkError
	if _, err := ParseLink("wolagent://pair?v=1&h=pc&k=abc"); !errors.As(err, &linkErr) || linkErr.Field != "v" {
		t.Errorf("lien v=1 : %v", err)
	}
}

func TestCleanName(t *testing.T) {
	for _, c := range []struct {
		in, want string
		limit    int
	}{
		{"  Pixel 8  ", "Pixel 8", 40},
		{"a\u0000b\u007fc\u009fd\r\n", "abcd", 40},
		{"abc def", "abc", 4},
		{"ab😀", "ab", 3},
		{"\xff\xfe", "��", 40},
	} {
		if got := CleanName(c.in, c.limit); got != c.want {
			t.Errorf("%q : %q, attendu %q", c.in, got, c.want)
		}
	}
}

type countingWriter struct {
	calls int
	bytes.Buffer
}

func (w *countingWriter) Write(p []byte) (int, error) {
	w.calls++
	return w.Buffer.Write(p)
}

func TestFrames(t *testing.T) {
	var w countingWriter
	if err := WriteFrame(&w, nil, 10); !errors.Is(err, ErrEmptyFrame) {
		t.Errorf("trame vide : %v", err)
	}
	if err := WriteFrame(&w, make([]byte, 11), 10); !errors.Is(err, ErrFrameTooLarge) {
		t.Errorf("trame trop longue : %v", err)
	}
	if err := WriteFrame(&w, []byte(`{"id":1}`), 10); err != nil || w.calls != 1 {
		t.Errorf("écriture en %d appels : %v", w.calls, err)
	}
	if got, err := ReadFrame(iotest.OneByteReader(bytes.NewReader(w.Bytes())), 10); err != nil || string(got) != `{"id":1}` {
		t.Errorf("lecture octet par octet : %q %v", got, err)
	}
	boom := errors.New("connexion coupée")
	if _, err := ReadFrame(iotest.ErrReader(boom), 10); err != boom {
		t.Errorf("erreur de lecture de la longueur : %v", err)
	}
	if _, err := ReadFrame(io.MultiReader(bytes.NewReader([]byte{0, 0, 0, 5, '{'}), iotest.ErrReader(boom)), 10); err != boom {
		t.Errorf("erreur de lecture du message : %v", err)
	}
}

func TestMigrationProofs(t *testing.T) {
	key := make([]byte, 32)
	agent, device := PinOf([]byte("agent")), PinOf([]byte("appareil"))
	nc, na, other := NewNonce(), NewNonce(), NewNonce()
	pa, pd := AgentProof(key, agent, device, nc, na), DeviceProof(key, agent, device, nc, na)
	if pa.Equal(pd) {
		t.Fatal("Pa et Pd identiques")
	}
	for name, changed := range map[string]Proof{
		"clé":                  AgentProof(append([]byte{1}, key[1:]...), agent, device, nc, na),
		"agent":                AgentProof(key, device, device, nc, na),
		"appareil":             AgentProof(key, agent, agent, nc, na),
		"Nc":                   AgentProof(key, agent, device, other, na),
		"Na":                   AgentProof(key, agent, device, nc, other),
		"empreintes inversées": AgentProof(key, device, agent, nc, na),
		"nonces inversés":      AgentProof(key, agent, device, na, nc),
	} {
		if changed.Equal(pa) {
			t.Errorf("%s : même preuve", name)
		}
	}
	if p, err := ParseProof(pa.String()); err != nil || p != pa {
		t.Errorf("preuve relue : %v", err)
	}
	if n, err := ParseNonce(nc.String()); err != nil || n != nc {
		t.Errorf("nonce relu : %v", err)
	}
	if _, err := ParseNonce(nc.String()[:42]); !errors.Is(err, ErrNonce) {
		t.Errorf("nonce court : %v", err)
	}
	if _, err := ParseProof(""); !errors.Is(err, ErrProof) {
		t.Errorf("preuve vide : %v", err)
	}
	// Les messages signés en wolagent/1 commencent par « wolagent/1\n » : aucun ne peut servir de preuve.
	for _, label := range []string{agentProofLabel, deviceProofLabel} {
		if strings.HasPrefix(label, protocol.Proto) || !strings.HasPrefix(label, ALPN+" ") {
			t.Errorf("libellé %q", label)
		}
	}
}

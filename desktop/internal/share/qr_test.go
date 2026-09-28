package share

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"rsc.io/qr"
)

// Vecteurs de QR code communs avec Android (core : QrCode.kt), produits par rsc.io/qr.
const qrVectorsPath = "../../../protocol/qr-vectors.json"

type qrVector struct {
	Text string   `json:"text"`
	Rows []string `json:"rows"`
}

func qrRows(t *testing.T, text string) []string {
	t.Helper()
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		t.Fatal(err)
	}
	rows := make([]string, code.Size)
	for y := range code.Size {
		var b strings.Builder
		for x := range code.Size {
			if code.Black(x, y) {
				b.WriteByte('#')
			} else {
				b.WriteByte('.')
			}
		}
		rows[y] = b.String()
	}
	return rows
}

func TestQRVectors(t *testing.T) {
	data, err := os.ReadFile(qrVectorsPath)
	if err != nil {
		t.Fatal(err)
	}
	var vectors []qrVector
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	for _, v := range vectors {
		if got := qrRows(t, v.Text); strings.Join(got, "\n") != strings.Join(v.Rows, "\n") {
			t.Errorf("QR code différent pour %q", v.Text)
		}
	}
	svg, err := QRCodeSVG(vectors[0].Text)
	if err != nil || !strings.HasPrefix(svg, "<svg") {
		t.Errorf("SVG : %v", err)
	}
}

// TestWriteQRVectors régénère protocol/qr-vectors.json (WOL_WRITE_SHARE_VECTORS=1).
func TestWriteQRVectors(t *testing.T) {
	if os.Getenv("WOL_WRITE_SHARE_VECTORS") != "1" {
		t.Skip("régénération des vecteurs désactivée")
	}
	var v vectors
	raw, _ := os.ReadFile(vectorsPath)
	_ = json.Unmarshal(raw, &v)
	texts := []string{
		"wolshare://test",
		v.Invite.Link,
		v.Request.Link,
		"wolshare://invite?v=1&name=" + strings.Repeat("%C3%A9", 40) + "&owner=" + v.OwnerPublic + "&user=" + strings.Repeat("a", 39) + "&gist=" + strings.Repeat("f", 40),
	}
	var out []qrVector
	for _, text := range texts {
		out = append(out, qrVector{Text: text, Rows: qrRows(t, text)})
	}
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)
	if err := os.WriteFile(qrVectorsPath, []byte(buf.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

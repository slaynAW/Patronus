package pairing

import (
	"bytes"
	"image/png"
	"strings"
	"testing"
)

func TestLink(t *testing.T) {
	link := Link(Info{Name: "PC de Salon & Jeux", Host: "192.168.1.20", Port: 9770, MAC: "aa:bb:cc:dd:ee:ff", Key: "k-_K"})
	want := "wolagent://pair?v=1&n=PC+de+Salon+%26+Jeux&h=192.168.1.20&p=9770&m=AA:BB:CC:DD:EE:FF&k=k-_K"
	if link != want {
		t.Fatalf("\n%s\n%s", link, want)
	}
}

func TestQR(t *testing.T) {
	text := Link(Info{Name: "PC", Host: "192.168.1.20", Port: 9770, Key: strings.Repeat("A", 43)})
	var term bytes.Buffer
	if err := WriteTerminalQR(&term, text, false, false); err != nil {
		t.Fatal(err)
	}
	if lines := strings.Count(term.String(), "\n"); lines < 10 {
		t.Fatalf("QR trop petit : %d lignes", lines)
	}
	var ansi bytes.Buffer
	if err := WriteTerminalQR(&ansi, text, true, false); err != nil || !strings.Contains(ansi.String(), "\x1b[") {
		t.Fatal("QR ANSI incorrect")
	}
	var img bytes.Buffer
	if err := WritePNG(&img, text, 4); err != nil {
		t.Fatal(err)
	}
	if _, err := png.Decode(&img); err != nil {
		t.Fatal(err)
	}
}

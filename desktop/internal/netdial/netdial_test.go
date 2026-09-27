package netdial

import (
	"context"
	"net"
	"testing"
	"time"
)

// Une connexion refusée doit être signalée vite (y compris sous Windows, voir control) et reconnue.
func TestRefusedIsFastAndRecognized(t *testing.T) {
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := l.Addr().String()
	l.Close()

	start := time.Now()
	conn, err := Dial(context.Background(), address, 3*time.Second)
	if err == nil {
		conn.Close()
		t.Skip("port réutilisé entre-temps")
	}
	if elapsed := time.Since(start); elapsed > 900*time.Millisecond {
		t.Errorf("refus signalé en %v : trop lent pour les sondes (1,5 s)", elapsed)
	}
	if !IsRefused(err) {
		t.Errorf("refus non reconnu : %v", err)
	}
}

func TestAcceptedConnection(t *testing.T) {
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	conn, err := Dial(context.Background(), l.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	if IsRefused(nil) || IsRefused(context.DeadlineExceeded) {
		t.Error("faux refus")
	}
}

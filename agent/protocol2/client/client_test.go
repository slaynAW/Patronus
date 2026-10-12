package client

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/protocol2"
)

// TestHello : avant TLS, le client ne lit qu'une ligne de salut, bornée, et reconnaît le blocage.
func TestHello(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	der, _ := protocol2.NewCertificate(key, time.Now())
	id := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	for hello, want := range map[string]error{
		`{"error":"rate_limited"}` + "\n":              ErrRateLimited,
		`{"error":"unauthorized"}` + "\n":              ErrNotAgent,
		"SSH-2.0-OpenSSH_9.6\r\n":                      ErrNotAgent,
		`{"other":1}` + "\n":                           ErrNotAgent,
		strings.Repeat("a", protocol2.MaxHelloBytes+1): ErrNotAgent,
	} {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		go func() {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			defer conn.Close()
			_, _ = conn.Write([]byte(hello))
			time.Sleep(time.Second)
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, err = Dial(ctx, ln.Addr().String(), Options{Identity: id})
		cancel()
		_ = ln.Close()
		if !errors.Is(err, want) {
			t.Errorf("%.30q : %v, attendu %v", hello, err, want)
		}
	}
	if _, err := Dial(context.Background(), "127.0.0.1:1", Options{}); err == nil {
		t.Error("identité absente acceptée")
	}
}

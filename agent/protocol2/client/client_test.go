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

// TestHelloProtocol : avec Options.Protocol, seul le salut de ce protocole est accepté.
func TestHelloProtocol(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	der, _ := protocol2.NewCertificate(key, time.Now())
	id := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	for _, c := range []struct {
		hello, protocol string
		unexpected      bool
	}{
		{`{"proto":"wolagent/2"}`, "patronus-hub/1", true},
		{`{"proto":"wolagent/1","nonce":"x"}`, "patronus-hub/1", true},
		{`{"proto":"patronus-hub/1"}`, "patronus-hub/1", false},
		{`{"proto":"patronus-hub/1"}`, "", false},
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
			_, _ = conn.Write([]byte(c.hello + "\n"))
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, err = Dial(ctx, ln.Addr().String(), Options{Identity: id, Protocol: c.protocol})
		cancel()
		_ = ln.Close()
		if err == nil || errors.Is(err, ErrUnexpectedProtocol) != c.unexpected {
			t.Errorf("%s avec %q : %v", c.hello, c.protocol, err)
		}
	}
}

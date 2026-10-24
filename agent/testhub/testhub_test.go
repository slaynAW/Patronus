package testhub

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/slaynaw/wakeonlan/agent/hubprotocol"
	"github.com/slaynaw/wakeonlan/agent/protocol2"
	"github.com/slaynaw/wakeonlan/agent/protocol2/client"
)

// TestHub appaire une application au hub par son lien, puis lui confie un vrai agent, que le hub relève.
func TestHub(t *testing.T) {
	h := Start(t)
	link, err := hubprotocol.ParseLink(h.Link(t, protocol2.RoleAdmin))
	if err != nil {
		t.Fatal(err)
	}
	if link.Agent != h.Pin || link.Port != h.Port || link.Host != "127.0.0.1" {
		t.Fatalf("lien : %+v", link)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := protocol2.NewCertificate(key, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	id := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	sess, err := client.Dial(ctx, net.JoinHostPort(link.Host, strconv.Itoa(link.Port)), client.Options{
		Identity: id, Agent: &link.Agent, Protocol: hubprotocol.ALPN,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	call := func(req hubprotocol.Request) hubprotocol.Response {
		t.Helper()
		var resp hubprotocol.Response
		if err := sess.Call(ctx, req, &resp); err != nil || !resp.OK {
			t.Fatalf("%s : %+v %v", req.Cmd, resp, err)
		}
		return resp
	}
	if resp := call(hubprotocol.Request{Cmd: protocol2.CmdPair, Code: link.Code, Name: "PC de test"}); resp.Role != "admin" {
		t.Fatalf("appairage : %+v", resp)
	}
	if devs := h.Devices(); len(devs) != 1 || devs[0].Name != "PC de test" {
		t.Fatalf("appareils : %+v", devs)
	}

	agent := StartAgent(t)
	code := agent.Invite(t, protocol2.RoleWake, h.Pin)
	added := call(hubprotocol.Request{Cmd: hubprotocol.CmdPCsAdd, Name: "PC Bureau", Host: "127.0.0.1", Port: agent.Port,
		Agent: agent.ID.Pin.String(), Code: code})
	if added.PC == nil || added.PC.Role != "wake" {
		t.Fatalf("PC confié : %+v", added)
	}
	for {
		status := call(hubprotocol.Request{Cmd: hubprotocol.CmdStatus})
		if len(status.States) == 1 && status.States[0].State == hubprotocol.StateOnline {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("PC jamais relevé : %+v\n%s", status.States, h.Log())
		case <-time.After(50 * time.Millisecond):
		}
	}
}

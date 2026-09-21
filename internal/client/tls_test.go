package client

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"xchat/internal/protocol"
	"xchat/internal/securestore"
	"xchat/internal/server"
)

func TestTransportPolicy(t *testing.T) {
	for _, address := range []string{"wss://chat.example.invalid/ws", "ws://127.0.0.1:18081/ws", "ws://[::1]/ws", "ws://localhost/ws"} {
		if err := ValidateTransport(address, false); err != nil {
			t.Fatal(address, err)
		}
	}
	for _, address := range []string{"ws://192.168.1.2/ws", "ws://chat.example.invalid/ws", "ws://localhost.evil.invalid/ws", "https://example.com/ws"} {
		if ValidateTransport(address, false) == nil {
			t.Fatal("unsafe address accepted", address)
		}
	}
	if err := ValidateTransport("ws://192.168.1.2/ws", true); err != nil {
		t.Fatal(err)
	}
}

func TestTLSChatRecallMentionsAndPeerIdentity(t *testing.T) {
	db, err := securestore.Open(filepath.Join(t.TempDir(), "rooms.db"), bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service := server.NewRooms(db)
	defer service.Close()
	versions := make(chan uint16, 10)
	handler := service.Handler()
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil {
			t.Error("plaintext request")
		} else {
			versions <- r.TLS.Version
		}
		handler.ServeHTTP(w, r)
	}))
	defer ts.Close()
	pool := x509.NewCertPool()
	pool.AddCert(ts.Certificate())
	address := "wss" + strings.TrimPrefix(ts.URL, "https") + "/ws"
	alice := New(address, Options{RootCAs: pool})
	bob := New(address, Options{RootCAs: pool})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	start := func(network *Client, name string) {
		done := make(chan struct{})
		go func() { network.Run(ctx, name, "secret-room"); close(done) }()
		t.Cleanup(func() {
			cancel()
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Error("client did not stop")
			}
		})
		awaitEvent(t, network.Events(), func(event Event) bool { return event.State == "connected" })
	}
	start(alice, "Alice")
	start(bob, "Bob Smith")
	body := protocol.MentionText("Bob Smith") + " (^_^)"
	if err := alice.Send("send", body); err != nil {
		t.Fatal(err)
	}
	ack := awaitEvent(t, alice.Events(), func(event Event) bool { return event.Frame != nil && event.Frame.Type == "ack" })
	sent, err := decode[protocol.Message](*ack.Frame)
	if err != nil || !sent.CanRecall || sent.Body != body {
		t.Fatal(sent, err)
	}
	event := awaitEvent(t, bob.Events(), func(event Event) bool { return event.Frame != nil && event.Frame.Type == "message" })
	message, _ := decode[protocol.Message](*event.Frame)
	if message.CanRecall || len(message.Mentions) != 1 || message.Mentions[0] != "Bob Smith" {
		t.Fatal(message)
	}
	if err := alice.Recall("recall", sent.ID); err != nil {
		t.Fatal(err)
	}
	for _, network := range []*Client{alice, bob} {
		event := awaitEvent(t, network.Events(), func(event Event) bool { return event.Frame != nil && event.Frame.Type == "recalled" })
		recalled, _ := decode[protocol.Recalled](*event.Frame)
		if !recalled.Message.Recalled || recalled.Message.Body != "" {
			t.Fatal(recalled)
		}
	}
	peer := alice.NewPeer()
	if peer.token != alice.token || peer.options.RootCAs != pool {
		t.Fatal("room switch lost identity or trust")
	}
	for i := 0; i < 2; i++ {
		if version := <-versions; version < tls.VersionTLS12 {
			t.Fatalf("weak TLS version %x", version)
		}
	}
}

func TestTLSRejectsUntrustedExpiredAndWrongHostname(t *testing.T) {
	for _, scenario := range []string{"untrusted", "expired", "hostname"} {
		t.Run(scenario, func(t *testing.T) {
			key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			template := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-2 * time.Hour), NotAfter: time.Now().Add(time.Hour), IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
			if scenario == "expired" {
				template.NotAfter = time.Now().Add(-time.Hour)
			}
			if scenario == "hostname" {
				template.IPAddresses = nil
				template.DNSNames = []string{"wrong.example.invalid"}
			}
			der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
			if err != nil {
				t.Fatal(err)
			}
			certificate, err := x509.ParseCertificate(der)
			if err != nil {
				t.Fatal(err)
			}
			ts := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("join reached server despite bad certificate") }))
			ts.Config.ErrorLog = log.New(io.Discard, "", 0)
			ts.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
			ts.StartTLS()
			defer ts.Close()
			pool := x509.NewCertPool()
			if scenario != "untrusted" {
				pool.AddCert(certificate)
			}
			network := New("wss"+strings.TrimPrefix(ts.URL, "https")+"/ws", Options{RootCAs: pool, AllowInsecure: true})
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			go network.Run(ctx, "Alice", "secret")
			event := awaitEvent(t, network.Events(), func(event Event) bool { return event.State == "tls_error" })
			if !strings.Contains(event.Detail, "证书校验失败") {
				t.Fatal("missing certificate error", event)
			}
			if _, open := <-network.Events(); open {
				t.Fatal("certificate failure retried")
			}
		})
	}
}

func TestTLSRejectsRedirectAndLoadsPrivateCA(t *testing.T) {
	requests := 0
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer target.Close()
	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer source.Close()
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: source.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	pool, err := LoadRootCAs(path)
	if err != nil {
		t.Fatal(err)
	}
	network := New("wss"+strings.TrimPrefix(source.URL, "https")+"/ws", Options{RootCAs: pool})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := network.connect(ctx, "Alice", "secret"); err == nil {
		t.Fatal("redirect accepted")
	}
	if requests != 0 {
		t.Fatal("plaintext redirect followed")
	}
}

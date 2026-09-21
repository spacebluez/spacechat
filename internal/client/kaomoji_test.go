package client

import (
	"context"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"xchat/internal/kaomoji"
)

func TestFetchKaomojiTLSCachePeerAndInvalidResponse(t *testing.T) {
	var mode atomic.Int32
	service := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/chat/api/kaomoji" || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "" {
			t.Error("unexpected catalog request", r.Method, r.URL, r.Header)
		}
		switch mode.Load() {
		case 1:
			if r.Header.Get("If-None-Match") != `"one"` {
				t.Error("cache validator missing")
			}
			w.WriteHeader(http.StatusNotModified)
			return
		case 2:
			w.Header().Set("ETag", `"bad"`)
			w.Write([]byte(`{"version":1,"categories":[]}`))
			return
		case 3:
			w.Write([]byte(strings.Repeat(" ", kaomoji.MaxCatalogBytes+1)))
			return
		}
		w.Header().Set("ETag", `"one"`)
		w.Write([]byte(`{"version":1,"categories":[{"id":"remote","name":"Remote","items":[{"text":"remote-only","keywords":["test"]}]}]}`))
	}))
	defer service.Close()
	pool := x509.NewCertPool()
	pool.AddCert(service.Certificate())
	address := "wss" + strings.TrimPrefix(service.URL, "https") + "/chat/ws?unused=secret"
	network := New(address, Options{RootCAs: pool})
	catalog, err := network.FetchKaomoji(context.Background())
	if err != nil || catalog.Categories[0].Items[0].Text != "remote-only" {
		t.Fatal("server catalog not fetched", catalog, err)
	}
	catalog.Categories[0].Items[0].Text = "mutated"
	catalog.Categories[0].Items[0].Keywords[0] = "mutated"
	mode.Store(1)
	peer := network.NewPeer()
	for _, client := range []*Client{network, peer} {
		cached, err := client.FetchKaomoji(context.Background())
		if err != nil || cached.Categories[0].Items[0].Text != "remote-only" || cached.Categories[0].Items[0].Keywords[0] != "test" {
			t.Fatal("cache corrupted or lost across room switch", cached, err)
		}
	}
	for _, invalidMode := range []int32{2, 3} {
		mode.Store(invalidMode)
		if _, err := network.FetchKaomoji(context.Background()); err == nil {
			t.Fatal("accepted invalid response", invalidMode)
		}
	}
	mode.Store(1)
	if _, err := network.FetchKaomoji(context.Background()); err != nil {
		t.Fatal("invalid response destroyed cache", err)
	}
	if _, err := New(address).FetchKaomoji(context.Background()); err == nil {
		t.Fatal("accepted untrusted TLS certificate")
	}
}

func TestFetchKaomojiRejectsRedirectAndRemotePlaintext(t *testing.T) {
	var targetRequests atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetRequests.Add(1) }))
	defer target.Close()
	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer source.Close()
	pool := x509.NewCertPool()
	pool.AddCert(source.Certificate())
	network := New("wss"+strings.TrimPrefix(source.URL, "https")+"/ws", Options{RootCAs: pool})
	if _, err := network.FetchKaomoji(context.Background()); err == nil || targetRequests.Load() != 0 {
		t.Fatal("catalog redirected to plaintext", err)
	}
	if _, err := New("ws://192.0.2.1/ws").FetchKaomoji(context.Background()); err == nil {
		t.Fatal("catalog bypassed remote TLS policy")
	}
}

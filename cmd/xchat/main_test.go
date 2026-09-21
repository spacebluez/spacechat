package main

import (
	"encoding/base64"
	"encoding/pem"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultServerUsesLoopback(t *testing.T) {
	if defaultServer != "wss://127.0.0.1:18081/ws" {
		t.Fatal("source default must use loopback; provide a deployment address at build or runtime")
	}
}

func TestBuiltInTLSRootAndExplicitOverride(t *testing.T) {
	server := httptest.NewTLSServer(nil)
	defer server.Close()
	certificate := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	encoded := base64.StdEncoding.EncodeToString(certificate)
	if pool, err := trustedRoots("", encoded); err != nil || pool == nil {
		t.Fatal("embedded CA not loaded", err)
	}
	if pool, err := trustedRoots("", ""); err != nil || pool != nil {
		t.Fatal("system trust default changed", err)
	}
	for _, invalid := range []string{"not-base64", base64.StdEncoding.EncodeToString([]byte("not a certificate"))} {
		if _, err := trustedRoots("", invalid); err == nil {
			t.Fatal("invalid built-in trust accepted")
		}
	}
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, certificate, 0600); err != nil {
		t.Fatal(err)
	}
	if pool, err := trustedRoots(path, "invalid-embedded-value"); err != nil || pool == nil {
		t.Fatal("explicit CA did not override embedded CA", err)
	}
}

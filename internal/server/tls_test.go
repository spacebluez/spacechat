package server

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestServerTransportRequiresTLSForRemoteListeners(t *testing.T) {
	for _, address := range []string{"0.0.0.0:18081", ":18081", "192.168.1.2:18081", "[::]:18081", "localhost:18081"} {
		if _, err := TransportConfig(address, "", "", false); err == nil {
			t.Fatal("plaintext remote listener accepted", address)
		}
	}
	for _, address := range []string{"127.0.0.1:18081", "[::1]:18081"} {
		if config, err := TransportConfig(address, "", "", false); err != nil || config != nil {
			t.Fatal(address, err)
		}
	}
	if _, err := TransportConfig(":18081", "", "", true); err != nil {
		t.Fatal(err)
	}
	if _, err := TransportConfig(":18081", "cert.pem", "", true); err == nil {
		t.Fatal("partial certificate configuration accepted")
	}
	if _, err := TransportConfig(":18081", "missing.pem", "missing.key", true); err == nil {
		t.Fatal("bad certificates downgraded to plaintext")
	}
}

func TestServerLoadsTLSCertificate(t *testing.T) {
	source := httptest.NewTLSServer(nil)
	defer source.Close()
	certificate := source.TLS.Certificates[0]
	key, err := x509.MarshalPKCS8PrivateKey(certificate.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	certFile, keyFile := filepath.Join(t.TempDir(), "cert.pem"), filepath.Join(t.TempDir(), "key.pem")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Certificate[0]}), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}), 0600); err != nil {
		t.Fatal(err)
	}
	config, err := TransportConfig(":18081", certFile, keyFile, false)
	if err != nil || config.MinVersion != tls.VersionTLS12 || len(config.Certificates) != 1 {
		t.Fatal(config, err)
	}
}

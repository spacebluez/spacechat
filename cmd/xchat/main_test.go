package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"xchat/internal/client"
	"xchat/internal/tui"
	"xchat/internal/update"
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

func TestUpdateRemoteUsesConfiguredTLSRoots(t *testing.T) {
	artifact := []byte("linux")
	digest := sha256.Sum256(artifact)
	raw := []byte(fmt.Sprintf(`{"schema":1,"latest_version":"0.4.0","minimum_supported_version":"0.3.2","release_notes":"private CA","artifacts":{"linux-amd64":{"file":"spacechat-client-linux-amd64-0.4.0","size":%d,"sha256":"%s"},"windows-amd64":{"file":"spacechat-client-windows-amd64-0.4.0.exe","size":%d,"sha256":"%s"}}}`,
		len(artifact), hex.EncodeToString(digest[:]), len(artifact), hex.EncodeToString(digest[:])))
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signature := ed25519.Sign(privateKey, raw)
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/updates/v1/manifest":
			_, _ = writer.Write(raw)
		case "/updates/v1/manifest.sig":
			_, _ = writer.Write(signature)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	certificate := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	pool, err := client.RootCAsFromPEM(certificate)
	if err != nil {
		t.Fatal(err)
	}
	current, _ := update.ParseVersion("0.3.2")
	remote := newUpdateRemote(publicKey, client.Options{RootCAs: pool})
	address := "wss" + strings.TrimPrefix(server.URL, "https") + "/ws"
	if _, err = remote.Check(context.Background(), address, current, "linux", "amd64"); err != nil {
		t.Fatal("private-CA update check failed:", err)
	}
}

func TestRunVersionAndSelfCheckModes(t *testing.T) {
	originalVersion, originalKey := version, updatePublicKey
	t.Cleanup(func() { version, updatePublicKey = originalVersion, originalKey })
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	version = "0.4.0"
	updatePublicKey = base64.StdEncoding.EncodeToString(publicKey)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	if code := run([]string{"--version"}, strings.NewReader(""), stdout, stderr); code != 0 {
		t.Fatalf("--version exit = %d, stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "0.4.0") {
		t.Fatalf("version output = %q", stdout.String())
	}
	if code := run([]string{"--self-check"}, strings.NewReader(""), new(bytes.Buffer), stderr); code != 0 {
		t.Fatalf("valid --self-check exit = %d, stderr=%q", code, stderr.String())
	}

	for name, values := range map[string][2]string{
		"development version": {"dev", updatePublicKey},
		"invalid version":     {"v0.4.0", updatePublicKey},
		"missing key":         {"0.4.0", ""},
		"short key":           {"0.4.0", base64.StdEncoding.EncodeToString([]byte("short"))},
	} {
		t.Run(name, func(t *testing.T) {
			version, updatePublicKey = values[0], values[1]
			if code := run([]string{"--self-check"}, strings.NewReader(""), new(bytes.Buffer), new(bytes.Buffer)); code == 0 {
				t.Fatal("invalid release metadata passed self-check")
			}
		})
	}
}

func TestRunCommandLineServerOverridesManagedConfig(t *testing.T) {
	configuration := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configuration)
	path := filepath.Join(configuration, "spacechat", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"server":"ws://configured.example/ws"}`), 0600); err != nil {
		t.Fatal(err)
	}

	originalVersion := version
	originalNewModel, originalTerminal := newModel, runTerminal
	t.Cleanup(func() {
		version = originalVersion
		newModel = originalNewModel
		runTerminal = originalTerminal
	})
	version = "dev"
	var receivedAddress string
	var receivedOptions client.Options
	newModel = func(address string, info client.Info, configuration ...client.Options) *tui.Model {
		receivedAddress = address
		if len(configuration) > 0 {
			receivedOptions = configuration[0]
		}
		return tui.NewWithClientInfo(address, info, configuration...)
	}
	runTerminal = func(tea.Model) error { return nil }

	code := run([]string{"--server", "wss://command.example/ws"}, strings.NewReader(""), new(bytes.Buffer), new(bytes.Buffer))
	if code != 0 {
		t.Fatalf("run exit = %d", code)
	}
	if receivedAddress != "wss://command.example/ws" {
		t.Fatalf("server = %q", receivedAddress)
	}
	if receivedOptions.AllowInsecure || receivedOptions.RootCAs != nil {
		t.Fatalf("unexpected transport options = %+v", receivedOptions)
	}
}

func TestRunUsesInstallationAuthorizationOnlyForConfiguredServer(t *testing.T) {
	configuration := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configuration)
	path := filepath.Join(configuration, "spacechat", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"server":"ws://configured.example/ws","allow_insecure":true}`), 0600); err != nil {
		t.Fatal(err)
	}

	originalVersion := version
	originalNewModel, originalTerminal := newModel, runTerminal
	t.Cleanup(func() {
		version = originalVersion
		newModel = originalNewModel
		runTerminal = originalTerminal
	})
	version = "dev"
	var receivedAddress string
	var receivedOptions client.Options
	newModel = func(address string, info client.Info, configuration ...client.Options) *tui.Model {
		receivedAddress = address
		if len(configuration) > 0 {
			receivedOptions = configuration[0]
		}
		return tui.NewWithClientInfo(address, info, configuration...)
	}
	runTerminal = func(tea.Model) error { return nil }

	if code := run(nil, strings.NewReader(""), new(bytes.Buffer), new(bytes.Buffer)); code != 0 {
		t.Fatalf("configured server exit = %d", code)
	}
	if receivedAddress != "ws://configured.example/ws" || !receivedOptions.AllowInsecure {
		t.Fatalf("configured transport = %q, %+v", receivedAddress, receivedOptions)
	}

	stderr := new(bytes.Buffer)
	if code := run([]string{"--server", "ws://other.example/ws"}, strings.NewReader(""), new(bytes.Buffer), stderr); code != 2 {
		t.Fatalf("explicit insecure server exit = %d, stderr=%q", code, stderr.String())
	}
}

func TestRunHelpHidesInternalSelfCheckFlag(t *testing.T) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	if code := run([]string{"--help"}, strings.NewReader(""), stdout, stderr); code != 0 {
		t.Fatalf("--help exit = %d", code)
	}
	if strings.Contains(stderr.String(), "self-check") {
		t.Fatalf("internal flag is public: %q", stderr.String())
	}
}

func TestLaunchUpdatedWaitsForImmediateChildFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-only")
	}
	path := filepath.Join(t.TempDir(), "failing-client")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 7\n"), 0755); err != nil {
		t.Fatal(err)
	}
	err := launchUpdated(path, nil, strings.NewReader(""), new(bytes.Buffer), new(bytes.Buffer))
	if err == nil {
		t.Fatal("immediately failing updated client was treated as a successful relaunch")
	}
}

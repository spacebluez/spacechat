package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"xchat/internal/client"
	"xchat/internal/tui"
)

func TestDefaultServerUsesLoopback(t *testing.T) {
	if defaultServer != "ws://127.0.0.1:18081/ws" {
		t.Fatal("source default must use loopback; provide a deployment address at build or runtime")
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
	newModel = func(address string, info client.Info) *tui.Model {
		receivedAddress = address
		return tui.NewWithClientInfo(address, info)
	}
	runTerminal = func(tea.Model) error { return nil }

	code := run([]string{"--server", "wss://command.example/ws"}, strings.NewReader(""), new(bytes.Buffer), new(bytes.Buffer))
	if code != 0 {
		t.Fatalf("run exit = %d", code)
	}
	if receivedAddress != "wss://command.example/ws" {
		t.Fatalf("server = %q", receivedAddress)
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

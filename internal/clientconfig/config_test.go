package clientconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadUsesFallbackWhenConfigIsMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")
	config, err := Load(path, "ws://127.0.0.1:18081/ws")
	if err != nil {
		t.Fatal(err)
	}
	if config.Server != "ws://127.0.0.1:18081/ws" {
		t.Fatalf("Server = %q", config.Server)
	}
}

func TestLoadAcceptsOnlyOneValidServerField(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "config.json")
	if err := os.WriteFile(path, []byte("{\"server\":\"wss://chat.example/ws\"}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	config, err := Load(path, "ws://fallback/ws")
	if err != nil {
		t.Fatal(err)
	}
	if config.Server != "wss://chat.example/ws" {
		t.Fatalf("Server = %q", config.Server)
	}

	invalid := []string{
		`{"server":"ws://chat/ws","extra":true}`,
		`{"server":"http://chat/ws"}`,
		`{"server":"ws://chat/ws"} {}`,
		`{}`,
	}
	for index, contents := range invalid {
		if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path, "ws://fallback/ws"); err == nil {
			t.Fatalf("invalid config %d accepted: %s", index, contents)
		}
	}
}

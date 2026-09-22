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

func TestLoadAcceptsAllowInsecureWithStrictJSONDecoding(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "config.json")
	valid := []struct {
		raw  string
		want bool
	}{
		{`{"server":"wss://chat.example/ws"}`, false},
		{`{"server":"ws://chat.example/ws","allow_insecure":true}`, true},
	}
	for _, test := range valid {
		if err := os.WriteFile(path, []byte(test.raw), 0600); err != nil {
			t.Fatal(err)
		}
		config, err := Load(path, "ws://fallback/ws")
		if err != nil {
			t.Fatalf("Load(%s): %v", test.raw, err)
		}
		if config.AllowInsecure != test.want {
			t.Fatalf("Load(%s).AllowInsecure = %t, want %t", test.raw, config.AllowInsecure, test.want)
		}
	}

	invalid := []string{
		`{"server":"ws://chat/ws","allow_insecure":"yes"}`,
		`{"server":"ws://chat/ws","allow_insecure":null}`,
		`{"server":"ws://chat/ws","allow_insecure":true,"allow_insecure":false}`,
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

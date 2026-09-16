package accesskey

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestReadKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "access.key")
	if _, err := Read(path); err == nil {
		t.Fatal("missing file accepted")
	}
	for _, contents := range []string{"", "   ", "bad\nkey"} {
		if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Read(path); err == nil {
			t.Fatal("invalid key accepted")
		}
	}
	if err := os.WriteFile(path, []byte("test-access-key\n"), 0600); err != nil {
		t.Fatal(err)
	}
	key, err := Read(path)
	if err != nil || key != "test-access-key" {
		t.Fatalf("read failed: %v", err)
	}
	if runtime.GOOS != "windows" {
		os.Chmod(path, 0644)
		if _, err := Read(path); err == nil {
			t.Fatal("world-readable key accepted")
		}
	}
}

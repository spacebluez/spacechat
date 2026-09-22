package securestore

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestEnsureKeyCreatesAndPreservesRandomKey(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "encryption.key")
	database := filepath.Join(directory, "rooms.db")

	if err := EnsureKey(path, database); err != nil {
		t.Fatal(err)
	}
	first, err := ReadKey(path)
	if err != nil || len(first) != 32 {
		t.Fatalf("key=%x err=%v", first, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("mode=%o", info.Mode().Perm())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(raw), "\n") || bytes.Count(raw, []byte("\n")) != 1 {
		t.Fatalf("key file must contain one newline-terminated value: %q", raw)
	}

	if err = EnsureKey(path, database); err != nil {
		t.Fatal(err)
	}
	second, err := ReadKey(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("existing key was replaced")
	}
}

func TestEnsureKeyRefusesDatabaseWithoutKey(t *testing.T) {
	directory := t.TempDir()
	database := filepath.Join(directory, "rooms.db")
	if err := os.WriteFile(database, []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := EnsureKey(filepath.Join(directory, "missing.key"), database); err == nil {
		t.Fatal("existing database received a new key")
	}
}

func TestEnsureKeyPreservesInvalidExistingKey(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "encryption.key")
	invalid := []byte("not a base64 database key\n")
	if err := os.WriteFile(path, invalid, 0600); err != nil {
		t.Fatal(err)
	}

	if err := EnsureKey(path, filepath.Join(directory, "rooms.db")); err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, invalid) {
		t.Fatalf("invalid key was replaced: got %q want %q", actual, invalid)
	}
	if _, err = ReadKey(path); err == nil {
		t.Fatal("invalid existing key became valid")
	}
}

func TestEnsureKeyRejectsSymlinkPaths(t *testing.T) {
	t.Run("key", func(t *testing.T) {
		directory := t.TempDir()
		path := filepath.Join(directory, "encryption.key")
		target := filepath.Join(directory, "target.key")
		if err := os.WriteFile(target, []byte("target"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
		if err := EnsureKey(path, filepath.Join(directory, "rooms.db")); err == nil {
			t.Fatal("symlink key path accepted")
		}
	})

	t.Run("database", func(t *testing.T) {
		directory := t.TempDir()
		database := filepath.Join(directory, "rooms.db")
		if err := os.Symlink(filepath.Join(directory, "target.db"), database); err != nil {
			t.Fatal(err)
		}
		if err := EnsureKey(filepath.Join(directory, "encryption.key"), database); err == nil {
			t.Fatal("symlink database path accepted")
		}
	})

	t.Run("parent", func(t *testing.T) {
		directory := t.TempDir()
		target := filepath.Join(directory, "real")
		if err := os.Mkdir(target, 0750); err != nil {
			t.Fatal(err)
		}
		parent := filepath.Join(directory, "keys")
		if err := os.Symlink(target, parent); err != nil {
			t.Fatal(err)
		}
		if err := EnsureKey(filepath.Join(parent, "encryption.key"), filepath.Join(directory, "rooms.db")); err == nil {
			t.Fatal("symlink parent path accepted")
		}
	})

	t.Run("parent with existing key", func(t *testing.T) {
		directory := t.TempDir()
		target := filepath.Join(directory, "real")
		if err := os.Mkdir(target, 0750); err != nil {
			t.Fatal(err)
		}
		parent := filepath.Join(directory, "keys")
		if err := os.Symlink(target, parent); err != nil {
			t.Fatal(err)
		}
		keyPath := filepath.Join(parent, "encryption.key")
		if err := os.WriteFile(keyPath, []byte("MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := EnsureKey(keyPath, filepath.Join(directory, "rooms.db")); err == nil {
			t.Fatal("existing key below symlink parent path accepted")
		}
	})
}

func TestReadKeyRejectsSymlink(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target.key")
	if err := os.WriteFile(target, []byte("MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=\n"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "encryption.key")
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadKey(path); err == nil {
		t.Fatal("symlink encryption key was accepted")
	}
}

func TestEnsureKeyConcurrentCallsPreserveOneKey(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "encryption.key")
	database := filepath.Join(directory, "rooms.db")
	const callers = 16

	errors := make(chan error, callers)
	var group sync.WaitGroup
	for range callers {
		group.Add(1)
		go func() {
			defer group.Done()
			errors <- EnsureKey(path, database)
		}()
	}
	group.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	key, err := ReadKey(path)
	if err != nil || len(key) != 32 {
		t.Fatalf("key=%x err=%v", key, err)
	}
}

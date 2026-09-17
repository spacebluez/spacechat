package securestore

import (
	"bytes"
	"path/filepath"
	"testing"
	"xchat/internal/store"
)

func TestRejectsLegacyDatabaseWithoutChangingItsMessages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = legacy.Append("legacy", "keep this"); err != nil {
		t.Fatal(err)
	}
	legacy.Close()
	if database, err := Open(path, bytes.Repeat([]byte{1}, 32)); err == nil {
		database.Close()
		t.Fatal("opened plaintext legacy database")
	}
	legacy, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer legacy.Close()
	latest, err := legacy.LatestID()
	if err != nil || latest != 1 {
		t.Fatal("legacy data changed")
	}
}

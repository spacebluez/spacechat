package store

import (
	"path/filepath"
	"testing"
)

func TestClearIsPersistentAndDoesNotReuseIDs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chat.db")
	repository, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	first, err := repository.Append("Alice", "old")
	if err != nil {
		t.Fatal(err)
	}
	instance := repository.InstanceID()
	count, err := repository.Clear()
	if err != nil || count != 1 {
		t.Fatalf("clear %d: %v", count, err)
	}
	if repository.InstanceID() == instance {
		t.Fatal("instance unchanged")
	}
	changed := repository.InstanceID()
	latest, err := repository.LatestID()
	if err != nil || latest != 0 {
		t.Fatal("history remained")
	}
	next, err := repository.Append("Alice", "new")
	if err != nil || next.ID <= first.ID {
		t.Fatal("ID reused")
	}
	repository.Close()
	repository, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	if repository.InstanceID() != changed {
		t.Fatal("clear generation not persisted")
	}
	page, err := repository.Page(0, 0, next.ID)
	if err != nil || len(page.Messages) != 1 || page.Messages[0].Body != "new" {
		t.Fatalf("old data restored %+v %v", page, err)
	}
}
func TestClearRollsBackOnFailure(t *testing.T) {
	repository, err := Open(filepath.Join(t.TempDir(), "chat.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	repository.Append("Alice", "one")
	repository.Append("Alice", "two")
	instance := repository.InstanceID()
	_, err = repository.database.Exec("CREATE TRIGGER reject_clear BEFORE DELETE ON messages WHEN OLD.id=2 BEGIN SELECT RAISE(ABORT, 'injected failure'); END")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repository.Clear(); err == nil {
		t.Fatal("failure ignored")
	}
	if repository.InstanceID() != instance {
		t.Fatal("failed clear changed instance")
	}
	page, err := repository.Page(0, 0, 2)
	if err != nil || len(page.Messages) != 2 {
		t.Fatal("partial deletion survived")
	}
}

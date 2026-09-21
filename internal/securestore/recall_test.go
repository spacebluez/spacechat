package securestore

import (
	"bytes"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"xchat/internal/protocol"
)

func TestRecallOwnershipIsolationAndPersistence(t *testing.T) {
	path, key := filepath.Join(t.TempDir(), "rooms.db"), bytes.Repeat([]byte{4}, 32)
	db, err := Open(path, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	room, _ := db.Room("one")
	other, _ := db.Room("two")
	epoch, otherEpoch := room.InstanceID(), other.InstanceID()
	message, err := room.AppendChat("Alice", "secret @Bob", "owner", []string{"Bob"})
	if err != nil {
		t.Fatal(err)
	}
	page, err := room.Page(0, 0, message.ID)
	if err != nil || !reflect.DeepEqual(page.Messages[0], message) {
		t.Fatal("message metadata lost", page, err)
	}
	for _, owner := range []string{"", "impostor"} {
		if _, err := room.Recall(message.ID, owner); !errors.Is(err, ErrRecallDenied) {
			t.Fatalf("wrong owner accepted: %v", err)
		}
	}
	if _, err := other.Recall(message.ID, "owner"); !errors.Is(err, ErrRecallDenied) {
		t.Fatal("cross-room recall", err)
	}
	if room.InstanceID() != epoch {
		t.Fatal("failed recall changed epoch")
	}
	recalled, err := room.Recall(message.ID, "owner")
	if err != nil || !recalled.Recalled || recalled.Body != "" || len(recalled.Mentions) != 0 {
		t.Fatal(recalled, err)
	}
	if room.InstanceID() == epoch || other.InstanceID() != otherEpoch {
		t.Fatal("wrong room epoch changed")
	}
	epoch = room.InstanceID()
	if _, err := room.Recall(message.ID, "owner"); err != nil || room.InstanceID() != epoch {
		t.Fatal("recall retry not idempotent", err)
	}
	db.Close()
	db, err = Open(path, key)
	if err != nil {
		t.Fatal(err)
	}
	room, _ = db.Room("one")
	page, err = room.Page(0, 0, message.ID)
	if err != nil || len(page.Messages) != 1 || !page.Messages[0].Recalled || page.Messages[0].Body != "" || len(page.Messages[0].Mentions) != 0 {
		t.Fatal("recalled content resurrected", page, err)
	}
	if room.InstanceID() != epoch {
		t.Fatal("recall epoch not persisted")
	}
	if _, err := db.ClearAll(); err != nil {
		t.Fatal(err)
	}
	if _, ok := db.epochs.Load(room.id); ok {
		t.Fatal("cleanup retained recall epoch")
	}
}

func TestRecallEnforcesServerTimeWindow(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "rooms.db"), bytes.Repeat([]byte{5}, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	room, _ := db.Room("one")
	message, err := room.AppendChat("Alice", "old", "owner", nil)
	if err != nil {
		t.Fatal(err)
	}
	var ciphertext []byte
	if err := db.database.QueryRow("SELECT payload FROM messages WHERE id=?", message.ID).Scan(&ciphertext); err != nil {
		t.Fatal(err)
	}
	plaintext, err := db.cipher.Open(nil, nil, ciphertext, room.aad(message))
	if err != nil {
		t.Fatal(err)
	}
	message.CreatedAt = time.Now().Add(-protocol.RecallWindow - time.Second).UTC().Format(time.RFC3339Nano)
	ciphertext = db.cipher.Seal(nil, nil, plaintext, room.aad(message))
	if _, err := db.database.Exec("UPDATE messages SET payload=?,created_at=? WHERE id=?", ciphertext, message.CreatedAt, message.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := room.Recall(message.ID, "owner"); !errors.Is(err, ErrRecallExpired) {
		t.Fatal("expired recall accepted", err)
	}
	page, err := room.Page(0, 0, message.ID)
	if err != nil || page.Messages[0].Body != "old" {
		t.Fatal("expired recall modified message", err)
	}
}

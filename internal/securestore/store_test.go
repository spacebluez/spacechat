package securestore

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestEncryptedRoomsPersistenceAndClear(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rooms.db")
	key := bytes.Repeat([]byte{7}, 32)
	database, err := Open(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	first, err := database.Room("first-secret")
	if err != nil {
		t.Fatal(err)
	}
	second, err := database.Room("second-secret")
	if err != nil {
		t.Fatal(err)
	}
	message, err := first.Append("secret-nickname", "private-line-one\nprivate-line-two")
	if err != nil {
		t.Fatal(err)
	}
	if latest, _ := second.LatestID(); latest != 0 {
		t.Fatal("room leaked")
	}
	page, err := first.Page(0, 0, message.ID)
	if err != nil || len(page.Messages) != 1 || !reflect.DeepEqual(page.Messages[0], message) {
		t.Fatalf("round trip: %+v %v", page, err)
	}
	var payload []byte
	if err := database.database.QueryRow("SELECT payload FROM messages").Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(payload, []byte("private")) {
		t.Fatal("plaintext payload")
	}
	for _, suffix := range []string{"", "-wal"} {
		raw, err := os.ReadFile(path + suffix)
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{"first-secret", "secret-nickname", "private-line-one"} {
			if bytes.Contains(raw, []byte(secret)) {
				t.Fatal("plaintext found in database or WAL")
			}
		}
	}
	instance := first.InstanceID()
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if wrong, err := Open(path, bytes.Repeat([]byte{8}, 32)); err == nil {
		wrong.Close()
		t.Fatal("wrong key accepted")
	}
	database, err = Open(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	first, _ = database.Room("first-secret")
	if first.InstanceID() != instance {
		t.Fatal("instance changed across reopen")
	}
	page, err = first.Page(0, 0, message.ID)
	if err != nil || !reflect.DeepEqual(page.Messages[0], message) {
		t.Fatal("reopen failed", err)
	}
	deleted, err := database.ClearAll()
	if err != nil || deleted != 1 {
		t.Fatal(deleted, err)
	}
	var rooms int
	if err := database.database.QueryRow("SELECT COUNT(*) FROM rooms").Scan(&rooms); err != nil || rooms != 0 {
		t.Fatal("rooms not cleared", err)
	}
	if first.InstanceID() == instance {
		t.Fatal("clear did not reset cursor identity")
	}
	if _, err := first.Append("next", "after clear"); err != nil {
		t.Fatal(err)
	}
	if err := database.database.QueryRow("SELECT COUNT(*) FROM rooms").Scan(&rooms); err != nil || rooms != 1 {
		t.Fatal("active room not recreated", err)
	}
}

func TestCiphertextTamperingAndInvalidPassphrases(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "rooms.db"), bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	for _, pass := range []string{"", "   ", "bad\nkey"} {
		if _, err := database.Room(pass); err == nil {
			t.Fatal("invalid passphrase accepted")
		}
	}
	first, _ := database.Room("one")
	second, _ := database.Room("two")
	message, _ := first.Append("name", "text")
	if _, err := database.database.Exec("UPDATE messages SET room_id=? WHERE id=?", second.ID(), message.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := second.Page(0, 0, message.ID); err == nil {
		t.Fatal("ciphertext room substitution accepted")
	}
}

func TestEncryptedRoomPagination(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "rooms.db"), bytes.Repeat([]byte{9}, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	room, _ := database.Room("pagination")
	for index := 0; index < 205; index++ {
		if _, err := room.Append("name", "message"); err != nil {
			t.Fatal(err)
		}
	}
	page, err := room.Page(0, 0, 205)
	if err != nil || len(page.Messages) != 100 || !page.HasMore || page.Messages[0].ID != 106 {
		t.Fatal("latest page", err)
	}
	page, err = room.Page(-1, 100, 205)
	if err != nil || len(page.Messages) != 100 || page.Messages[0].ID != 101 || !page.HasMore {
		t.Fatal("sync page", err)
	}
}

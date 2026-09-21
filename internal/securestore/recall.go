package securestore

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"xchat/internal/protocol"
)

var ErrRecallDenied = errors.New("message cannot be recalled by this client")
var ErrRecallExpired = errors.New("message recall window expired")

func (room *Room) Recall(id int64, owner string) (protocol.Message, error) {
	room.store.mutations.Lock()
	defer room.store.mutations.Unlock()
	var message protocol.Message
	tx, err := room.store.database.Begin()
	if err != nil {
		return message, err
	}
	defer tx.Rollback()
	var ciphertext []byte
	err = tx.QueryRow("SELECT id,payload,created_at FROM messages WHERE room_id=? AND id=?", room.id, id).Scan(&message.ID, &ciphertext, &message.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return message, ErrRecallDenied
	}
	if err != nil {
		return message, err
	}
	plaintext, err := room.store.cipher.Open(nil, nil, ciphertext, room.aad(message))
	if err != nil {
		return message, err
	}
	var decoded content
	if err = json.Unmarshal(plaintext, &decoded); err != nil {
		return message, err
	}
	if owner == "" || decoded.OwnerID != owner {
		return message, ErrRecallDenied
	}
	message.Nickname, message.OwnerID = decoded.Nickname, decoded.OwnerID
	message.Recalled = true
	if decoded.Recalled {
		return message, nil
	}
	created, err := time.Parse(time.RFC3339Nano, message.CreatedAt)
	if err != nil {
		return message, err
	}
	if time.Since(created) > protocol.RecallWindow {
		return message, ErrRecallExpired
	}
	decoded.Body, decoded.Mentions, decoded.Recalled = "", nil, true
	plaintext, err = json.Marshal(decoded)
	if err != nil {
		return message, err
	}
	ciphertext = room.store.cipher.Seal(nil, nil, plaintext, room.aad(message))
	if _, err = tx.Exec("UPDATE messages SET payload=? WHERE room_id=? AND id=?", ciphertext, room.id, id); err != nil {
		return message, err
	}
	// Changing the room epoch forces offline clients to discard stale cached bodies.
	epoch := rand.Text()
	if _, err = tx.Exec("INSERT INTO metadata(key,value) VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", "recall_epoch/"+room.id, epoch); err != nil {
		return message, err
	}
	if err = tx.Commit(); err != nil {
		return message, err
	}
	room.store.epochs.Store(room.id, epoch)
	return message, nil
}

package securestore

import (
	"encoding/json"
	"fmt"
	"time"

	"xchat/internal/protocol"
)

type Room struct {
	store *Store
	id    string
}

type content struct {
	Nickname string
	Body     string
	OwnerID  string   `json:",omitempty"`
	Mentions []string `json:",omitempty"`
	Recalled bool     `json:",omitempty"`
}

func (room *Room) ID() string { return room.id }
func (room *Room) InstanceID() string {
	instance := room.store.instance.Load().(string) + ":" + room.id
	if epoch, ok := room.store.epochs.Load(room.id); ok {
		instance += ":" + epoch.(string)
	}
	return instance
}
func (room *Room) LatestID() (int64, error) {
	var latest int64
	err := room.store.database.QueryRow("SELECT COALESCE(MAX(id),0) FROM messages WHERE room_id=?", room.id).Scan(&latest)
	return latest, err
}
func (room *Room) Clear() (int64, error) { return room.store.ClearAll() }
func (room *Room) aad(message protocol.Message) []byte {
	return []byte(fmt.Sprintf("xchat-message/v1/%s/%d/%s", room.id, message.ID, message.CreatedAt))
}
func (room *Room) Append(name, body string) (protocol.Message, error) {
	return room.AppendChat(name, body, "", nil)
}
func (room *Room) AppendChat(name, body, owner string, mentions []string) (protocol.Message, error) {
	message := protocol.Message{Nickname: name, Body: body, OwnerID: owner, Mentions: mentions, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	plaintext, err := json.Marshal(content{Nickname: name, Body: body, OwnerID: owner, Mentions: mentions})
	if err != nil {
		return message, err
	}
	transaction, err := room.store.database.Begin()
	if err != nil {
		return message, err
	}
	defer transaction.Rollback()
	if _, err = transaction.Exec("INSERT OR IGNORE INTO rooms(id) VALUES (?)", room.id); err != nil {
		return message, err
	}
	result, err := transaction.Exec("INSERT INTO messages(room_id,payload,created_at) VALUES (?,X'',?)", room.id, message.CreatedAt)
	if err != nil {
		return message, err
	}
	message.ID, err = result.LastInsertId()
	if err != nil {
		return message, err
	}
	ciphertext := room.store.cipher.Seal(nil, nil, plaintext, room.aad(message))
	if _, err = transaction.Exec("UPDATE messages SET payload=? WHERE id=?", ciphertext, message.ID); err != nil {
		return message, err
	}
	return message, transaction.Commit()
}
func (room *Room) Page(before, after, through int64) (protocol.Page, error) {
	page := protocol.Page{Messages: []protocol.Message{}, ThroughID: through}
	query := "SELECT id,payload,created_at FROM messages WHERE room_id=? AND id<=?"
	args := []any{room.id, through}
	if before > 0 {
		query += " AND id<?"
		args = append(args, before)
	}
	ascending := after > 0 || before < 0
	if ascending {
		query += " AND id>? ORDER BY id ASC"
		args = append(args, after)
	} else {
		query += " ORDER BY id DESC"
	}
	query += " LIMIT ?"
	args = append(args, protocol.PageSize+1)
	rows, err := room.store.database.Query(query, args...)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var message protocol.Message
		var ciphertext []byte
		if err = rows.Scan(&message.ID, &ciphertext, &message.CreatedAt); err != nil {
			return page, err
		}
		plaintext, err := room.store.cipher.Open(nil, nil, ciphertext, room.aad(message))
		if err != nil {
			return protocol.Page{}, fmt.Errorf("message authentication failed: %w", err)
		}
		var decoded content
		if err = json.Unmarshal(plaintext, &decoded); err != nil {
			return protocol.Page{}, err
		}
		message.Nickname, message.Body = decoded.Nickname, decoded.Body
		message.OwnerID, message.Mentions, message.Recalled = decoded.OwnerID, decoded.Mentions, decoded.Recalled
		page.Messages = append(page.Messages, message)
	}
	if err = rows.Err(); err != nil {
		return page, err
	}
	page.HasMore = len(page.Messages) > protocol.PageSize
	if page.HasMore {
		page.Messages = page.Messages[:protocol.PageSize]
	}
	if !ascending {
		for left, right := 0, len(page.Messages)-1; left < right; left, right = left+1, right-1 {
			page.Messages[left], page.Messages[right] = page.Messages[right], page.Messages[left]
		}
	}
	return page, nil
}

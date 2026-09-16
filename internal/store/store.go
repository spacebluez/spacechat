package store

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
	"xchat/internal/protocol"
)

type Store struct {
	database *sql.DB
	instance string
}

func Open(path string) (*Store, error) {
	database, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	database.SetMaxOpenConns(1)
	success := false
	defer func() {
		if !success {
			database.Close()
		}
	}()
	for _, statement := range []string{
		"PRAGMA journal_mode=WAL", "PRAGMA busy_timeout=5000", "PRAGMA foreign_keys=ON", "PRAGMA synchronous=FULL",
		"CREATE TABLE IF NOT EXISTS metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL)",
		"CREATE TABLE IF NOT EXISTS messages (id INTEGER PRIMARY KEY AUTOINCREMENT, nickname TEXT NOT NULL, body TEXT NOT NULL, created_at TEXT NOT NULL)",
	} {
		if _, err = database.Exec(statement); err != nil {
			return nil, err
		}
	}
	if _, err = database.Exec("INSERT OR IGNORE INTO metadata(key,value) VALUES ('instance_id',?),('schema_version','1')", rand.Text()); err != nil {
		return nil, err
	}
	repository := &Store{database: database}
	if err = database.QueryRow("SELECT value FROM metadata WHERE key='instance_id'").Scan(&repository.instance); err != nil {
		return nil, err
	}
	var version string
	if err = database.QueryRow("SELECT value FROM metadata WHERE key='schema_version'").Scan(&version); err != nil {
		return nil, err
	}
	if version != "1" {
		return nil, fmt.Errorf("unsupported database schema %s", version)
	}
	success = true
	return repository, nil
}

func (repository *Store) InstanceID() string { return repository.instance }
func (repository *Store) Close() error       { return repository.database.Close() }
func (repository *Store) LatestID() (int64, error) {
	var latest int64
	err := repository.database.QueryRow("SELECT COALESCE(MAX(id),0) FROM messages").Scan(&latest)
	return latest, err
}
func (repository *Store) Append(name, body string) (protocol.Message, error) {
	message := protocol.Message{Nickname: name, Body: body, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	result, err := repository.database.Exec("INSERT INTO messages(nickname,body,created_at) VALUES (?,?,?)", name, body, message.CreatedAt)
	if err != nil {
		return message, err
	}
	message.ID, err = result.LastInsertId()
	return message, err
}
func (repository *Store) Page(before, after, through int64) (protocol.Page, error) {
	page := protocol.Page{Messages: []protocol.Message{}, ThroughID: through}
	query := "SELECT id,nickname,body,created_at FROM messages WHERE id<=?"
	args := []any{through}
	if before > 0 {
		query += " AND id<?"
		args = append(args, before)
	}
	ascending := after > 0 || before < 0
	if ascending {
		query += " AND id>?"
		args = append(args, after)
		query += " ORDER BY id ASC"
	} else {
		query += " ORDER BY id DESC"
	}
	query += " LIMIT ?"
	args = append(args, protocol.PageSize+1)
	rows, err := repository.database.Query(query, args...)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var message protocol.Message
		if err = rows.Scan(&message.ID, &message.Nickname, &message.Body, &message.CreatedAt); err != nil {
			return page, err
		}
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

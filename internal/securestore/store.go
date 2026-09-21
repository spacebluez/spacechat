package securestore

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"sync/atomic"

	_ "modernc.org/sqlite"
	"xchat/internal/accesskey"
)

type Store struct {
	database  *sql.DB
	cipher    cipher.AEAD
	roomKey   []byte
	instance  atomic.Value
	epochs    sync.Map
	mutations sync.Mutex
}

func derive(master []byte, purpose string) []byte {
	hash := hmac.New(sha256.New, master)
	hash.Write([]byte(purpose))
	return hash.Sum(nil)
}

func Open(path string, key []byte) (*Store, error) {
	if len(key) != 32 {
		return nil, errors.New("encryption key must contain 32 random bytes")
	}
	block, err := aes.NewCipher(derive(key, "xchat-rooms/encryption/v1"))
	if err != nil {
		return nil, err
	}
	encryption, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return nil, err
	}
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
	var existing int
	if err = database.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'").Scan(&existing); err != nil {
		return nil, err
	}
	if existing > 0 {
		var version string
		if err = database.QueryRow("SELECT value FROM metadata WHERE key='schema_version'").Scan(&version); err != nil || version != "rooms-encrypted-1" {
			return nil, errors.New("not an encrypted rooms database; legacy migration is not supported")
		}
	}
	for _, statement := range []string{
		"PRAGMA journal_mode=WAL", "PRAGMA busy_timeout=5000", "PRAGMA foreign_keys=ON", "PRAGMA synchronous=FULL",
	} {
		if _, err = database.Exec(statement); err != nil {
			return nil, err
		}
	}
	repository := &Store{database: database, cipher: encryption, roomKey: derive(key, "xchat-rooms/identity/v1")}
	if existing == 0 {
		transaction, err := database.Begin()
		if err != nil {
			return nil, err
		}
		defer transaction.Rollback()
		for _, statement := range []string{
			"CREATE TABLE metadata (key TEXT PRIMARY KEY, value BLOB NOT NULL)",
			"CREATE TABLE rooms (id TEXT PRIMARY KEY)",
			"CREATE TABLE messages (id INTEGER PRIMARY KEY AUTOINCREMENT, room_id TEXT NOT NULL REFERENCES rooms(id), payload BLOB NOT NULL, created_at TEXT NOT NULL)",
			"CREATE INDEX room_messages ON messages(room_id,id)",
		} {
			if _, err = transaction.Exec(statement); err != nil {
				return nil, err
			}
		}
		check := encryption.Seal(nil, nil, []byte("xchat-rooms/key-check/v1"), []byte("metadata/key-check"))
		if _, err = transaction.Exec("INSERT INTO metadata(key,value) VALUES ('schema_version','rooms-encrypted-1'),('instance_id',?),('key_check',?)", rand.Text(), check); err != nil {
			return nil, err
		}
		if err = transaction.Commit(); err != nil {
			return nil, err
		}
	}
	var check []byte
	if err = database.QueryRow("SELECT value FROM metadata WHERE key='key_check'").Scan(&check); err != nil {
		return nil, err
	}
	plaintext, err := encryption.Open(nil, nil, check, []byte("metadata/key-check"))
	if err != nil || string(plaintext) != "xchat-rooms/key-check/v1" {
		return nil, errors.New("encryption key does not match database or metadata is damaged")
	}
	var instance string
	if err = database.QueryRow("SELECT value FROM metadata WHERE key='instance_id'").Scan(&instance); err != nil {
		return nil, err
	}
	repository.instance.Store(instance)
	rows, err := database.Query("SELECT key,value FROM metadata WHERE key LIKE 'recall_epoch/%'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var name, epoch string
		if err := rows.Scan(&name, &epoch); err != nil {
			return nil, err
		}
		repository.epochs.Store(strings.TrimPrefix(name, "recall_epoch/"), epoch)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	success = true
	return repository, nil
}

func (repository *Store) Close() error  { return repository.database.Close() }
func (repository *Store) Health() error { return repository.database.Ping() }
func (repository *Store) Room(passphrase string) (*Room, error) {
	if err := accesskey.Validate(passphrase); err != nil {
		return nil, err
	}
	hash := hmac.New(sha256.New, repository.roomKey)
	hash.Write([]byte(passphrase))
	room := &Room{store: repository, id: hex.EncodeToString(hash.Sum(nil))}
	if _, err := repository.database.Exec("INSERT OR IGNORE INTO rooms(id) VALUES (?)", room.id); err != nil {
		return nil, err
	}
	return room, nil
}
func (repository *Store) ClearAll() (int64, error) {
	repository.mutations.Lock()
	defer repository.mutations.Unlock()
	transaction, err := repository.database.Begin()
	if err != nil {
		return 0, err
	}
	defer transaction.Rollback()
	result, err := transaction.Exec("DELETE FROM messages")
	if err != nil {
		return 0, err
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if _, err = transaction.Exec("DELETE FROM rooms"); err != nil {
		return 0, err
	}
	if _, err = transaction.Exec("DELETE FROM metadata WHERE key LIKE 'recall_epoch/%'"); err != nil {
		return 0, err
	}
	instance := rand.Text()
	if _, err = transaction.Exec("UPDATE metadata SET value=? WHERE key='instance_id'", instance); err != nil {
		return 0, err
	}
	if err = transaction.Commit(); err != nil {
		return 0, err
	}
	repository.instance.Store(instance)
	repository.epochs.Clear()
	return deleted, nil
}

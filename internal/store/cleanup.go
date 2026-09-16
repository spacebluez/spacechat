package store

import "crypto/rand"

func (repository *Store) Clear() (int64, error) {
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
	instance := rand.Text()
	if _, err = transaction.Exec("UPDATE metadata SET value=? WHERE key='instance_id'", instance); err != nil {
		return 0, err
	}
	if err = transaction.Commit(); err != nil {
		return 0, err
	}
	repository.instance.Store(instance)
	return deleted, nil
}

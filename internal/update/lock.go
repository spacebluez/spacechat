package update

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

var ErrUpdateLocked = errors.New("another SpaceChat update is in progress")

const staleLockAfter = 10 * time.Minute

type lockMetadata struct {
	PID       int       `json:"pid"`
	CreatedAt time.Time `json:"created_at"`
}

type Lock struct {
	path string
	info os.FileInfo
}

func Acquire(layout Layout, now time.Time) (*Lock, error) {
	if err := os.MkdirAll(layout.Root, 0700); err != nil {
		return nil, err
	}
	for attempt := 0; attempt < 2; attempt++ {
		file, err := os.OpenFile(layout.Lock, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err == nil {
			metadata := lockMetadata{PID: os.Getpid(), CreatedAt: now.UTC()}
			if encodeError := json.NewEncoder(file).Encode(metadata); encodeError != nil {
				file.Close()
				os.Remove(layout.Lock)
				return nil, encodeError
			}
			if syncError := file.Sync(); syncError != nil {
				file.Close()
				os.Remove(layout.Lock)
				return nil, syncError
			}
			if closeError := file.Close(); closeError != nil {
				os.Remove(layout.Lock)
				return nil, closeError
			}
			info, statError := os.Lstat(layout.Lock)
			if statError != nil {
				return nil, statError
			}
			return &Lock{path: layout.Lock, info: info}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		stale, inspectError := staleUpdateLock(layout.Lock, now)
		if inspectError != nil {
			return nil, inspectError
		}
		if !stale || attempt > 0 {
			return nil, ErrUpdateLocked
		}
		if err = os.Remove(layout.Lock); err != nil {
			return nil, fmt.Errorf("remove stale update lock: %w", err)
		}
	}
	return nil, ErrUpdateLocked
}

func staleUpdateLock(path string, now time.Time) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 1024 {
		return false, errors.New("update lock is not a safe regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()
	var metadata lockMetadata
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&metadata); err != nil || metadata.PID <= 0 || metadata.CreatedAt.IsZero() {
		return false, errors.New("update lock metadata is invalid")
	}
	age := now.Sub(metadata.CreatedAt)
	return age > staleLockAfter, nil
}

func (lock *Lock) Release() error {
	if lock == nil || lock.path == "" {
		return nil
	}
	info, err := os.Lstat(lock.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !os.SameFile(lock.info, info) {
		return errors.New("update lock ownership changed")
	}
	err = os.Remove(lock.path)
	if err == nil {
		lock.path = ""
	}
	return err
}

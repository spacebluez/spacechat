package securestore

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"xchat/internal/accesskey"
)

func ReadKey(path string) ([]byte, error) {
	if info, err := os.Lstat(path); err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return nil, errors.New("encryption key file must be a regular file, not a symlink")
	}
	encoded, err := accesskey.Read(path)
	if err != nil {
		return nil, err
	}
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(key) != 32 {
		return nil, errors.New("encryption key file must contain a base64-encoded random 32-byte key")
	}
	return key, nil
}

// EnsureKey creates a new encryption key only when the key and database are
// both absent. Existing key material is never replaced.
func EnsureKey(path, databasePath string) error {
	parent := filepath.Dir(path)
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("encryption key path must be a regular file")
		}
		parentInfo, parentErr := os.Lstat(parent)
		if parentErr != nil {
			return parentErr
		}
		if !parentInfo.IsDir() || parentInfo.Mode()&os.ModeSymlink != 0 {
			return errors.New("encryption key parent must be a real directory")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if _, err := os.Lstat(databasePath); err == nil {
		return errors.New("existing database requires its original encryption key")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(parent, 0750); err != nil {
		return err
	}
	parentInfo, err := os.Lstat(parent)
	if err != nil || !parentInfo.IsDir() || parentInfo.Mode()&os.ModeSymlink != 0 {
		return errors.New("encryption key parent must be a real directory")
	}
	temporary, err := os.CreateTemp(parent, ".encryption-key-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	closed := false
	defer func() {
		if !closed {
			_ = temporary.Close()
		}
	}()
	if err = temporary.Chmod(0600); err != nil {
		return err
	}
	material := make([]byte, 32)
	if _, err = rand.Read(material); err != nil {
		return err
	}
	if _, err = fmt.Fprintln(temporary, base64.StdEncoding.EncodeToString(material)); err != nil {
		return err
	}
	if err = temporary.Sync(); err != nil {
		return err
	}
	if err = temporary.Close(); err != nil {
		return err
	}
	closed = true
	if err = os.Link(temporaryPath, path); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return nil
		}
		return err
	}
	directory, err := os.Open(parent)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

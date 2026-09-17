package securestore

import (
	"encoding/base64"
	"errors"
	"xchat/internal/accesskey"
)

func ReadKey(path string) ([]byte, error) {
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

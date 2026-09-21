package clientconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"xchat/internal/client"
)

const maximumConfigSize = 64 << 10

type Config struct {
	Server string `json:"server"`
}

func Load(path, fallback string) (Config, error) {
	if err := client.ValidateAddress(fallback); err != nil {
		return Config{}, fmt.Errorf("invalid fallback server: %w", err)
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{Server: fallback}, nil
	}
	if err != nil {
		return Config{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > maximumConfigSize {
		return Config{}, errors.New("client config must be a regular JSON file no larger than 64 KiB")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	config, err := decode(raw)
	if err != nil {
		return Config{}, err
	}
	if err = client.ValidateAddress(config.Server); err != nil {
		return Config{}, fmt.Errorf("invalid configured server: %w", err)
	}
	return config, nil
}

func decode(raw []byte) (Config, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return Config{}, errors.New("client config must be one JSON object")
	}
	config := Config{}
	seenServer := false
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return Config{}, fmt.Errorf("decode client config: %w", err)
		}
		name, ok := token.(string)
		if !ok || name != "server" {
			return Config{}, errors.New("client config may contain only the server field")
		}
		if seenServer {
			return Config{}, errors.New("client config contains duplicate server fields")
		}
		seenServer = true
		if err = decoder.Decode(&config.Server); err != nil {
			return Config{}, fmt.Errorf("decode configured server: %w", err)
		}
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') || !seenServer {
		return Config{}, errors.New("client config must contain one server field")
	}
	var extra any
	if err = decoder.Decode(&extra); err != io.EOF {
		return Config{}, errors.New("client config contains trailing JSON")
	}
	return config, nil
}

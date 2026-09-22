package update

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

const InstallerServerModeRequest = "request"

type InstallerManifest struct {
	Schema     int                 `json:"schema"`
	Version    string              `json:"version"`
	Server     string              `json:"server,omitempty"`
	ServerMode string              `json:"server_mode,omitempty"`
	Packages   map[string]Artifact `json:"packages"`
}

func VerifyInstallerManifest(raw, signature []byte, publicKey ed25519.PublicKey) (InstallerManifest, error) {
	if len(publicKey) != ed25519.PublicKeySize {
		return InstallerManifest{}, errors.New("invalid installer manifest public key")
	}
	if len(signature) != ed25519.SignatureSize || !ed25519.Verify(publicKey, raw, signature) {
		return InstallerManifest{}, errors.New("invalid installer manifest signature")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var manifest InstallerManifest
	if err := decoder.Decode(&manifest); err != nil {
		return InstallerManifest{}, fmt.Errorf("decode installer manifest: %w", err)
	}
	if err := requireJSONEnd(decoder); err != nil {
		return InstallerManifest{}, err
	}
	if err := manifest.validate(); err != nil {
		return InstallerManifest{}, err
	}
	return manifest, nil
}

func (manifest InstallerManifest) validate() error {
	version, err := ParseVersion(manifest.Version)
	if err != nil {
		return fmt.Errorf("invalid installer version: %w", err)
	}
	switch manifest.Schema {
	case 1:
		if manifest.ServerMode != "" {
			return errors.New("schema 1 installer manifest cannot set server_mode")
		}
		server, err := url.Parse(manifest.Server)
		if err != nil || server.Host == "" || (server.Scheme != "ws" && server.Scheme != "wss") || server.User != nil || server.Fragment != "" {
			return errors.New("invalid installer server URL")
		}
	case 2:
		if manifest.Server != "" || manifest.ServerMode != InstallerServerModeRequest {
			return errors.New("schema 2 installer manifest requires request server mode")
		}
	default:
		return errors.New("unsupported installer manifest schema")
	}
	expectedFiles := map[string]string{
		"windows-amd64": "spacechat-windows-amd64-" + version.String() + ".zip",
		"linux-amd64":   "spacechat-linux-amd64-" + version.String() + ".zip",
	}
	if len(manifest.Packages) != len(expectedFiles) {
		return errors.New("installer manifest must contain exactly two supported packages")
	}
	for platform, expectedFile := range expectedFiles {
		item, exists := manifest.Packages[platform]
		if !exists {
			return fmt.Errorf("missing installer package for %s", platform)
		}
		if item.File != expectedFile || filepath.Base(item.File) != item.File || strings.ContainsAny(item.File, `/\`) {
			return fmt.Errorf("invalid installer package filename for %s", platform)
		}
		if item.Size <= 0 || item.Size > MaxArtifactSize {
			return fmt.Errorf("invalid installer package size for %s", platform)
		}
		if !digestPattern.MatchString(item.SHA256) {
			return fmt.Errorf("invalid installer package digest for %s", platform)
		}
	}
	return nil
}

func (manifest InstallerManifest) Package(goos, goarch string) (Artifact, error) {
	key := goos + "-" + goarch
	item, exists := manifest.Packages[key]
	if !exists || (key != "windows-amd64" && key != "linux-amd64") {
		return Artifact{}, fmt.Errorf("unsupported installer platform %s", key)
	}
	return item, nil
}

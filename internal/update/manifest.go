package update

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const MaxArtifactSize int64 = 128 << 20

var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type Artifact struct {
	File   string `json:"file"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type Manifest struct {
	Schema                  int                 `json:"schema"`
	LatestVersion           string              `json:"latest_version"`
	MinimumSupportedVersion string              `json:"minimum_supported_version"`
	ReleaseNotes            string              `json:"release_notes"`
	Artifacts               map[string]Artifact `json:"artifacts"`
}

func VerifyManifest(raw, signature []byte, publicKey ed25519.PublicKey) (Manifest, error) {
	if len(publicKey) != ed25519.PublicKeySize {
		return Manifest{}, errors.New("invalid manifest public key")
	}
	if len(signature) != ed25519.SignatureSize || !ed25519.Verify(publicKey, raw, signature) {
		return Manifest{}, errors.New("invalid manifest signature")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode manifest: %w", err)
	}
	if err := requireJSONEnd(decoder); err != nil {
		return Manifest{}, err
	}
	if err := manifest.validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func requireJSONEnd(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("manifest contains trailing JSON")
		}
		return fmt.Errorf("decode trailing manifest content: %w", err)
	}
	return nil
}

func (manifest Manifest) validate() error {
	if manifest.Schema != 1 {
		return errors.New("unsupported manifest schema")
	}
	latest, err := ParseVersion(manifest.LatestVersion)
	if err != nil {
		return fmt.Errorf("invalid latest version: %w", err)
	}
	minimum, err := ParseVersion(manifest.MinimumSupportedVersion)
	if err != nil {
		return fmt.Errorf("invalid minimum version: %w", err)
	}
	if minimum.Compare(latest) > 0 {
		return errors.New("minimum version exceeds latest version")
	}
	if !utf8.ValidString(manifest.ReleaseNotes) || utf8.RuneCountInString(manifest.ReleaseNotes) > 4000 {
		return errors.New("invalid release notes")
	}
	for _, character := range manifest.ReleaseNotes {
		if unicode.IsControl(character) {
			return errors.New("release notes contain control characters")
		}
	}
	expectedFiles := map[string]string{
		"windows-amd64": "spacechat-client-windows-amd64-" + latest.String() + ".exe",
		"linux-amd64":   "spacechat-client-linux-amd64-" + latest.String(),
	}
	if len(manifest.Artifacts) != len(expectedFiles) {
		return errors.New("manifest must contain exactly two supported artifacts")
	}
	for platform, expectedFile := range expectedFiles {
		artifact, exists := manifest.Artifacts[platform]
		if !exists {
			return fmt.Errorf("missing artifact for %s", platform)
		}
		if artifact.File != expectedFile || filepath.Base(artifact.File) != artifact.File || strings.ContainsAny(artifact.File, `/\`) {
			return fmt.Errorf("invalid artifact filename for %s", platform)
		}
		if artifact.Size <= 0 || artifact.Size > MaxArtifactSize {
			return fmt.Errorf("invalid artifact size for %s", platform)
		}
		if !digestPattern.MatchString(artifact.SHA256) {
			return fmt.Errorf("invalid artifact digest for %s", platform)
		}
	}
	return nil
}

func (manifest Manifest) Artifact(goos, goarch string) (Artifact, error) {
	key := goos + "-" + goarch
	artifact, exists := manifest.Artifacts[key]
	if !exists || (key != "windows-amd64" && key != "linux-amd64") {
		return Artifact{}, fmt.Errorf("unsupported client platform %s", key)
	}
	return artifact, nil
}

func (manifest Manifest) Latest() (Version, error) {
	return ParseVersion(manifest.LatestVersion)
}

func (manifest Manifest) Minimum() (Version, error) {
	return ParseVersion(manifest.MinimumSupportedVersion)
}

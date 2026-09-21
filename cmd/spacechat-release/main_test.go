package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"xchat/internal/update"
)

func writePrivateKey(t *testing.T, key []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "release.key")
	if err := os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(key)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPublicKeyPrintsPublicHalfForSeedAndPrivateKey(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	for name, material := range map[string][]byte{"seed": privateKey.Seed(), "private key": privateKey} {
		t.Run(name, func(t *testing.T) {
			stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
			code := run([]string{"public-key", "-private-key", writePrivateKey(t, material)}, stdout, stderr)
			if code != 0 {
				t.Fatalf("exit=%d stderr=%q", code, stderr.String())
			}
			if strings.TrimSpace(stdout.String()) != base64.StdEncoding.EncodeToString(publicKey) {
				t.Fatalf("public key = %q", stdout.String())
			}
		})
	}
}

func TestManifestProducesSignedExactArtifacts(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	windowsSource := filepath.Join(directory, "windows.exe")
	linuxSource := filepath.Join(directory, "linux")
	windowsContents := []byte("windows artifact")
	linuxContents := []byte("linux artifact")
	if err = os.WriteFile(windowsSource, windowsContents, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(linuxSource, linuxContents, 0600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(directory, "updates-0.4.0")
	stderr := new(bytes.Buffer)
	code := run([]string{
		"manifest", "-version", "0.4.0", "-minimum", "0.3.2",
		"-private-key", writePrivateKey(t, privateKey),
		"-windows", windowsSource, "-linux", linuxSource, "-out", out,
	}, new(bytes.Buffer), stderr)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
	raw, err := os.ReadFile(filepath.Join(out, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) == 0 || raw[len(raw)-1] != '\n' || bytes.HasSuffix(raw, []byte("\n\n")) {
		t.Fatalf("manifest must have exactly one trailing newline: %q", raw)
	}
	signature, err := os.ReadFile(filepath.Join(out, "manifest.sig"))
	if err != nil {
		t.Fatal(err)
	}
	if len(signature) != ed25519.SignatureSize {
		t.Fatalf("signature size = %d", len(signature))
	}
	manifest, err := update.VerifyManifest(raw, signature, publicKey)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.LatestVersion != "0.4.0" || manifest.MinimumSupportedVersion != "0.3.2" {
		t.Fatalf("manifest versions = %+v", manifest)
	}
	for platform, expected := range map[string][]byte{"windows-amd64": windowsContents, "linux-amd64": linuxContents} {
		artifact := manifest.Artifacts[platform]
		actual, readError := os.ReadFile(filepath.Join(out, artifact.File))
		if readError != nil {
			t.Fatal(readError)
		}
		if !bytes.Equal(actual, expected) || artifact.Size != int64(len(expected)) {
			t.Fatalf("%s artifact mismatch: %+v %q", platform, artifact, actual)
		}
	}
}

func TestManifestFailuresLeaveNoOutputDirectory(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	windowsSource := filepath.Join(directory, "windows.exe")
	linuxSource := filepath.Join(directory, "linux")
	if err = os.WriteFile(windowsSource, []byte("windows"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(linuxSource, []byte("linux"), 0600); err != nil {
		t.Fatal(err)
	}
	validKey := writePrivateKey(t, privateKey)
	badKey := writePrivateKey(t, []byte("bad key"))
	for name, values := range map[string]struct {
		minimum string
		key     string
	}{
		"minimum exceeds latest": {minimum: "0.5.0", key: validKey},
		"malformed private key":  {minimum: "0.3.2", key: badKey},
	} {
		t.Run(name, func(t *testing.T) {
			out := filepath.Join(directory, strings.ReplaceAll(name, " ", "-"))
			code := run([]string{
				"manifest", "-version", "0.4.0", "-minimum", values.minimum,
				"-private-key", values.key, "-windows", windowsSource, "-linux", linuxSource, "-out", out,
			}, new(bytes.Buffer), new(bytes.Buffer))
			if code == 0 {
				t.Fatal("invalid manifest inputs succeeded")
			}
			if _, statError := os.Lstat(out); !os.IsNotExist(statError) {
				t.Fatalf("partial output remains: %v", statError)
			}
		})
	}
}

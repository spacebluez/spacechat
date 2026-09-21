package update

import (
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"testing"
)

const validManifest = `{"schema":1,"latest_version":"0.4.0","minimum_supported_version":"0.3.2","release_notes":"online update","artifacts":{"windows-amd64":{"file":"spacechat-client-windows-amd64-0.4.0.exe","size":4,"sha256":"88d4266fd4e6338d13b845fcf289579d209c897823b9217da3e161936f031589"},"linux-amd64":{"file":"spacechat-client-linux-amd64-0.4.0","size":4,"sha256":"88d4266fd4e6338d13b845fcf289579d209c897823b9217da3e161936f031589"}}}`

func signManifest(t *testing.T, raw []byte) (ed25519.PublicKey, []byte) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return publicKey, ed25519.Sign(privateKey, raw)
}

func TestVerifyManifestAndSelectArtifact(t *testing.T) {
	raw := []byte(validManifest)
	publicKey, signature := signManifest(t, raw)
	manifest, err := VerifyManifest(raw, signature, publicKey)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := manifest.Artifact("windows", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if artifact.File != "spacechat-client-windows-amd64-0.4.0.exe" || artifact.Size != 4 {
		t.Fatalf("wrong artifact: %+v", artifact)
	}
	if _, err = manifest.Artifact("linux", "arm64"); err == nil {
		t.Fatal("accepted unsupported platform")
	}
}

func TestVerifyManifestRejectsInvalidSignatureBeforeJSON(t *testing.T) {
	raw := []byte(validManifest)
	publicKey, signature := signManifest(t, raw)
	raw[0] = '['
	if _, err := VerifyManifest(raw, signature, publicKey); err == nil || !strings.Contains(err.Error(), "signature") {
		t.Fatalf("expected signature error, got %v", err)
	}
}

func TestManifestValidation(t *testing.T) {
	cases := map[string]string{
		"unknown field":        strings.Replace(validManifest, `"schema":1`, `"schema":1,"extra":true`, 1),
		"trailing json":        validManifest + `{}`,
		"wrong schema":         strings.Replace(validManifest, `"schema":1`, `"schema":2`, 1),
		"minimum after latest": strings.Replace(validManifest, `"minimum_supported_version":"0.3.2"`, `"minimum_supported_version":"0.5.0"`, 1),
		"unsafe filename":      strings.Replace(validManifest, `spacechat-client-linux-amd64-0.4.0`, `../spacechat-client-linux-amd64-0.4.0`, 1),
		"uppercase digest":     strings.Replace(validManifest, `88d4266f`, `88D4266F`, 1),
		"zero size":            strings.Replace(validManifest, `"size":4`, `"size":0`, 1),
		"oversized":            strings.Replace(validManifest, `"size":4`, `"size":134217729`, 1),
		"missing platform":     strings.Replace(validManifest, `,"linux-amd64":{"file":"spacechat-client-linux-amd64-0.4.0","size":4,"sha256":"88d4266fd4e6338d13b845fcf289579d209c897823b9217da3e161936f031589"}`, ``, 1),
		"control in notes":     strings.Replace(validManifest, `online update`, `online\nupdate`, 1),
	}
	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			raw := []byte(text)
			publicKey, signature := signManifest(t, raw)
			if _, err := VerifyManifest(raw, signature, publicKey); err == nil {
				t.Fatal("invalid manifest accepted")
			}
		})
	}
}

func TestVerifyManifestRejectsInvalidKeyAndSignatureLengths(t *testing.T) {
	if _, err := VerifyManifest([]byte(validManifest), make([]byte, ed25519.SignatureSize), []byte("short")); err == nil {
		t.Fatal("accepted invalid public key length")
	}
	if _, err := VerifyManifest([]byte(validManifest), []byte("short"), make([]byte, ed25519.PublicKeySize)); err == nil {
		t.Fatal("accepted invalid signature length")
	}
}

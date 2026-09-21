package update

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"strings"
	"testing"
)

func signInstallerManifest(t *testing.T, manifest InstallerManifest) ([]byte, []byte, ed25519.PublicKey) {
	t.Helper()
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return raw, ed25519.Sign(privateKey, raw), publicKey
}

func validInstallerManifest() InstallerManifest {
	return InstallerManifest{
		Schema:  1,
		Version: "0.4.0",
		Server:  "ws://192.168.33.216:18081/ws",
		Packages: map[string]Artifact{
			"linux-amd64": {
				File:   "spacechat-linux-amd64-0.4.0.zip",
				Size:   123,
				SHA256: strings.Repeat("1", 64),
			},
			"windows-amd64": {
				File:   "spacechat-windows-amd64-0.4.0.zip",
				Size:   456,
				SHA256: strings.Repeat("2", 64),
			},
		},
	}
}

func TestVerifyInstallerManifestAcceptsExactSignedPackages(t *testing.T) {
	manifest := validInstallerManifest()
	raw, signature, publicKey := signInstallerManifest(t, manifest)
	verified, err := VerifyInstallerManifest(raw, signature, publicKey)
	if err != nil {
		t.Fatal(err)
	}
	if verified.Version != "0.4.0" || verified.Server != "ws://192.168.33.216:18081/ws" {
		t.Fatalf("verified manifest = %+v", verified)
	}
	linux, err := verified.Package("linux", "amd64")
	if err != nil || linux.File != "spacechat-linux-amd64-0.4.0.zip" {
		t.Fatalf("linux package=%+v err=%v", linux, err)
	}
	if _, err = verified.Package("darwin", "arm64"); err == nil {
		t.Fatal("unsupported installer platform accepted")
	}
}

func TestVerifyInstallerManifestRejectsUnsafeCatalogs(t *testing.T) {
	tests := map[string]func(*InstallerManifest){
		"unknown schema":       func(manifest *InstallerManifest) { manifest.Schema = 2 },
		"invalid version":      func(manifest *InstallerManifest) { manifest.Version = "latest" },
		"unsafe server scheme": func(manifest *InstallerManifest) { manifest.Server = "https://chat.invalid/ws" },
		"server credentials":   func(manifest *InstallerManifest) { manifest.Server = "ws://user@chat.invalid/ws" },
		"missing platform":     func(manifest *InstallerManifest) { delete(manifest.Packages, "linux-amd64") },
		"extra platform": func(manifest *InstallerManifest) {
			manifest.Packages["darwin-arm64"] = manifest.Packages["linux-amd64"]
		},
		"path traversal": func(manifest *InstallerManifest) {
			item := manifest.Packages["linux-amd64"]
			item.File = "../spacechat.zip"
			manifest.Packages["linux-amd64"] = item
		},
		"wrong filename": func(manifest *InstallerManifest) {
			item := manifest.Packages["linux-amd64"]
			item.File = "another.zip"
			manifest.Packages["linux-amd64"] = item
		},
		"invalid digest": func(manifest *InstallerManifest) {
			item := manifest.Packages["linux-amd64"]
			item.SHA256 = "ABC"
			manifest.Packages["linux-amd64"] = item
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			manifest := validInstallerManifest()
			mutate(&manifest)
			raw, signature, publicKey := signInstallerManifest(t, manifest)
			if _, err := VerifyInstallerManifest(raw, signature, publicKey); err == nil {
				t.Fatal("unsafe installer manifest accepted")
			}
		})
	}
}

func TestVerifyInstallerManifestRejectsInvalidSignatureAndUnknownFields(t *testing.T) {
	manifest := validInstallerManifest()
	raw, signature, publicKey := signInstallerManifest(t, manifest)
	signature[0] ^= 1
	if _, err := VerifyInstallerManifest(raw, signature, publicKey); err == nil {
		t.Fatal("invalid installer signature accepted")
	}

	raw = []byte(`{"schema":1,"version":"0.4.0","server":"ws://chat.invalid/ws","packages":{},"extra":true}`)
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = VerifyInstallerManifest(raw, ed25519.Sign(privateKey, raw), privateKey.Public().(ed25519.PublicKey)); err == nil {
		t.Fatal("unknown installer manifest field accepted")
	}
}

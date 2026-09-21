package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"xchat/internal/server"
)

func TestParseConfigRequiresPairedUpdateFlags(t *testing.T) {
	if _, err := parseConfig([]string{}); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"-update-dir", "/updates"},
		{"-update-public-key-file", "/key"},
		{"-installer-dir", "/installers"},
	} {
		if _, err := parseConfig(args); err == nil {
			t.Fatalf("unpaired update flags accepted: %v", args)
		}
	}
	config, err := parseConfig([]string{
		"-update-dir", "/updates", "-update-public-key-file", "/key", "-installer-dir", "/installers",
		"-tls-cert", "/tls.crt", "-tls-key", "/tls.key", "-kaomoji", "/kaomoji.json",
	})
	if err != nil {
		t.Fatal(err)
	}
	if config.updateDirectory != "/updates" || config.updatePublicKeyFile != "/key" || config.installerDirectory != "/installers" {
		t.Fatalf("update config = %+v", config)
	}
	if config.tlsCertificate != "/tls.crt" || config.tlsKey != "/tls.key" || config.kaomojiPath != "/kaomoji.json" {
		t.Fatalf("transport/catalog config = %+v", config)
	}
	config, err = parseConfig([]string{"-validate-updates", "-update-dir", "/updates", "-update-public-key-file", "/key"})
	if err != nil || !config.validateUpdates {
		t.Fatalf("validation config=%+v err=%v", config, err)
	}
	if _, err = parseConfig([]string{"-validate-updates"}); err == nil {
		t.Fatal("update validation without a catalog was accepted")
	}
}

func makeInstallerFixture(t *testing.T) (string, string) {
	t.Helper()
	directory := t.TempDir()
	windows, linux := []byte("windows package"), []byte("linux package")
	digest := func(data []byte) string {
		hash := sha256.Sum256(data)
		return hex.EncodeToString(hash[:])
	}
	raw := []byte(fmt.Sprintf(`{"schema":1,"version":"0.4.0","server":"ws://chat.invalid/ws","packages":{"linux-amd64":{"file":"spacechat-linux-amd64-0.4.0.zip","size":%d,"sha256":"%s"},"windows-amd64":{"file":"spacechat-windows-amd64-0.4.0.zip","size":%d,"sha256":"%s"}}}`,
		len(linux), digest(linux), len(windows), digest(windows)))
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{
		"installers.json":                   raw,
		"installers.sig":                    ed25519.Sign(privateKey, raw),
		"spacechat-windows-amd64-0.4.0.zip": windows,
		"spacechat-linux-amd64-0.4.0.zip":   linux,
	} {
		if err = os.WriteFile(filepath.Join(directory, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	keyPath := filepath.Join(t.TempDir(), "update-public.key")
	if err = os.WriteFile(keyPath, []byte(base64.StdEncoding.EncodeToString(publicKey)+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return directory, keyPath
}

func TestLoadInstallerOptionsWiresBootstrapRoutes(t *testing.T) {
	directory, keyPath := makeInstallerFixture(t)
	options, catalog, err := loadInstallerOptions(serverConfig{installerDirectory: directory, updatePublicKeyFile: keyPath})
	if err != nil {
		t.Fatal(err)
	}
	if catalog == nil || catalog.Manifest().Version != "0.4.0" || len(options) != 1 {
		t.Fatalf("catalog=%+v options=%d", catalog, len(options))
	}
	service := server.NewRooms(nil, options...)
	defer service.Close()
	recorder := httptest.NewRecorder()
	service.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "http://chat.invalid/install/linux", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "spacechat-linux-amd64-0.4.0.zip") {
		t.Fatalf("bootstrap response = %d %q", recorder.Code, recorder.Body.String())
	}
}

func makeUpdateFixture(t *testing.T) (string, string, []byte) {
	t.Helper()
	directory := t.TempDir()
	windows, linux := []byte("windows"), []byte("linux")
	digest := func(data []byte) string {
		hash := sha256.Sum256(data)
		return hex.EncodeToString(hash[:])
	}
	raw := []byte(fmt.Sprintf(`{"schema":1,"latest_version":"0.4.0","minimum_supported_version":"0.3.2","release_notes":"release","artifacts":{"linux-amd64":{"file":"spacechat-client-linux-amd64-0.4.0","size":%d,"sha256":"%s"},"windows-amd64":{"file":"spacechat-client-windows-amd64-0.4.0.exe","size":%d,"sha256":"%s"}}}`,
		len(linux), digest(linux), len(windows), digest(windows)))
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"manifest.json": raw,
		"manifest.sig":  ed25519.Sign(privateKey, raw),
		"spacechat-client-windows-amd64-0.4.0.exe": windows,
		"spacechat-client-linux-amd64-0.4.0":       linux,
	}
	for name, data := range files {
		if err = os.WriteFile(filepath.Join(directory, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	keyPath := filepath.Join(t.TempDir(), "update-public.key")
	if err = os.WriteFile(keyPath, []byte(base64.StdEncoding.EncodeToString(publicKey)+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return directory, keyPath, raw
}

func TestLoadUpdateOptionsValidatesKeyAndWiresCatalog(t *testing.T) {
	directory, keyPath, raw := makeUpdateFixture(t)
	options, catalog, err := loadUpdateOptions(serverConfig{updateDirectory: directory, updatePublicKeyFile: keyPath})
	if err != nil {
		t.Fatal(err)
	}
	if catalog == nil || catalog.MinimumVersion().String() != "0.3.2" || len(options) != 1 {
		t.Fatalf("catalog=%+v options=%d", catalog, len(options))
	}
	service := server.NewRooms(nil, options...)
	defer service.Close()
	recorder := httptest.NewRecorder()
	service.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/updates/v1/manifest", nil))
	if recorder.Code != http.StatusOK || recorder.Body.String() != string(raw) {
		t.Fatalf("manifest response = %d %q", recorder.Code, recorder.Body.String())
	}

	for name, data := range map[string][]byte{
		"invalid base64": []byte("not-base64\n"),
		"wrong size":     []byte(base64.StdEncoding.EncodeToString([]byte("short")) + "\n"),
	} {
		t.Run(name, func(t *testing.T) {
			invalidPath := filepath.Join(t.TempDir(), "key")
			if writeError := os.WriteFile(invalidPath, data, 0644); writeError != nil {
				t.Fatal(writeError)
			}
			if _, _, loadError := loadUpdateOptions(serverConfig{updateDirectory: directory, updatePublicKeyFile: invalidPath}); loadError == nil {
				t.Fatal("invalid update public key accepted")
			}
		})
	}
}

func TestLoadUpdateOptionsAllowsStagedMigrationWithoutCatalog(t *testing.T) {
	options, catalog, err := loadUpdateOptions(serverConfig{})
	if err != nil || catalog != nil || len(options) != 0 {
		t.Fatalf("options=%v catalog=%v err=%v", options, catalog, err)
	}
}

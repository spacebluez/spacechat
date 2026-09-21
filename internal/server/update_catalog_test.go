package server

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"xchat/internal/securestore"
)

type catalogFixture struct {
	directory string
	publicKey ed25519.PublicKey
	raw       []byte
	signature []byte
	files     map[string][]byte
}

func makeCatalogFixture(t *testing.T) catalogFixture {
	t.Helper()
	directory := t.TempDir()
	files := map[string][]byte{
		"spacechat-client-windows-amd64-0.4.0.exe": []byte("windows"),
		"spacechat-client-linux-amd64-0.4.0":       []byte("linux"),
	}
	digest := func(data []byte) string {
		hash := sha256.Sum256(data)
		return hex.EncodeToString(hash[:])
	}
	raw := []byte(fmt.Sprintf(`{"schema":1,"latest_version":"0.4.0","minimum_supported_version":"0.3.2","release_notes":"online update","artifacts":{"windows-amd64":{"file":"spacechat-client-windows-amd64-0.4.0.exe","size":%d,"sha256":"%s"},"linux-amd64":{"file":"spacechat-client-linux-amd64-0.4.0","size":%d,"sha256":"%s"}}}`,
		len(files["spacechat-client-windows-amd64-0.4.0.exe"]), digest(files["spacechat-client-windows-amd64-0.4.0.exe"]),
		len(files["spacechat-client-linux-amd64-0.4.0"]), digest(files["spacechat-client-linux-amd64-0.4.0"])))
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signature := ed25519.Sign(privateKey, raw)
	if err = os.WriteFile(filepath.Join(directory, "manifest.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(directory, "manifest.sig"), signature, 0600); err != nil {
		t.Fatal(err)
	}
	for name, data := range files {
		if err = os.WriteFile(filepath.Join(directory, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return catalogFixture{directory: directory, publicKey: publicKey, raw: raw, signature: signature, files: files}
}

func TestUpdateCatalogServesOnlyVerifiedFiles(t *testing.T) {
	fixture := makeCatalogFixture(t)
	catalog, err := LoadUpdateCatalog(fixture.directory, fixture.publicKey)
	if err != nil {
		t.Fatal(err)
	}
	if catalog.MinimumVersion().String() != "0.3.2" || catalog.Manifest().LatestVersion != "0.4.0" {
		t.Fatal("catalog versions not retained")
	}
	database, err := securestore.Open(filepath.Join(t.TempDir(), "rooms.db"), bytes.Repeat([]byte{3}, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := NewRooms(database, WithUpdateCatalog(catalog))
	defer service.Close()

	tests := map[string][]byte{
		"/updates/v1/manifest":                                     fixture.raw,
		"/updates/v1/manifest.sig":                                 fixture.signature,
		"/updates/v1/artifacts/spacechat-client-linux-amd64-0.4.0": fixture.files["spacechat-client-linux-amd64-0.4.0"],
	}
	for path, expected := range tests {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		service.Handler().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK || !bytes.Equal(recorder.Body.Bytes(), expected) {
			t.Fatalf("GET %s = %d %q", path, recorder.Code, recorder.Body.Bytes())
		}
		if recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("GET %s omitted nosniff", path)
		}
	}
	for _, path := range []string{
		"/updates/v1/artifacts/not-listed",
		"/updates/v1/artifacts/spacechat-client-linux-amd64-0.4.0/extra",
	} {
		recorder := httptest.NewRecorder()
		service.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("GET %s = %d, want 404", path, recorder.Code)
		}
	}
	recorder := httptest.NewRecorder()
	service.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/updates/v1/artifacts/../manifest.json", nil))
	if recorder.Code == http.StatusOK {
		t.Fatal("path traversal returned update content")
	}
}

func TestUpdateCatalogRejectsInvalidArtifacts(t *testing.T) {
	tests := map[string]func(*testing.T, catalogFixture){
		"missing": func(t *testing.T, fixture catalogFixture) {
			if err := os.Remove(filepath.Join(fixture.directory, "spacechat-client-linux-amd64-0.4.0")); err != nil {
				t.Fatal(err)
			}
		},
		"wrong size": func(t *testing.T, fixture catalogFixture) {
			if err := os.WriteFile(filepath.Join(fixture.directory, "spacechat-client-linux-amd64-0.4.0"), []byte("short"), 0600); err != nil {
				t.Fatal(err)
			}
		},
		"wrong digest": func(t *testing.T, fixture catalogFixture) {
			data := append([]byte(nil), fixture.files["spacechat-client-linux-amd64-0.4.0"]...)
			data[0] ^= 1
			if err := os.WriteFile(filepath.Join(fixture.directory, "spacechat-client-linux-amd64-0.4.0"), data, 0600); err != nil {
				t.Fatal(err)
			}
		},
		"symlink": func(t *testing.T, fixture catalogFixture) {
			path := filepath.Join(fixture.directory, "spacechat-client-linux-amd64-0.4.0")
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(fixture.directory, "spacechat-client-windows-amd64-0.4.0.exe"), path); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			fixture := makeCatalogFixture(t)
			mutate(t, fixture)
			if _, err := LoadUpdateCatalog(fixture.directory, fixture.publicKey); err == nil {
				t.Fatal("invalid catalog accepted")
			}
		})
	}
}

func TestUpdateCatalogRejectsSymlinkedManifest(t *testing.T) {
	fixture := makeCatalogFixture(t)
	manifest := filepath.Join(fixture.directory, "manifest.json")
	backup := filepath.Join(fixture.directory, "manifest.real")
	if err := os.Rename(manifest, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(backup, manifest); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadUpdateCatalog(fixture.directory, fixture.publicKey); err == nil {
		t.Fatal("symlinked manifest accepted")
	}
}

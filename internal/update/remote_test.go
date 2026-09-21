package update

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUpdateURLUsesSelectedServerOrigin(t *testing.T) {
	tests := map[string]string{
		"ws://chat.example:18081/ws":            "http://chat.example:18081/updates/v1/manifest",
		"wss://chat.example/company/socket?x=1": "https://chat.example/updates/v1/manifest",
	}
	for server, expected := range tests {
		actual, err := UpdateURL(server, "/updates/v1/manifest")
		if err != nil || actual.String() != expected {
			t.Fatalf("UpdateURL(%q) = %v, %v", server, actual, err)
		}
	}
	for _, server := range []string{"http://chat.example/ws", "ws://user@chat.example/ws", "ws:///missing", "ws://chat.example/ws#fragment"} {
		if _, err := UpdateURL(server, "/updates/v1/manifest"); err == nil {
			t.Fatalf("accepted server %q", server)
		}
	}
	if _, err := UpdateURL("ws://chat.example/ws", "https://evil.example/file"); err == nil {
		t.Fatal("accepted absolute update endpoint")
	}
}

func TestRemoteUsesSeparateMetadataAndArtifactTimeouts(t *testing.T) {
	remote := Remote{}
	if timeout := remote.httpClient(metadataRequestTimeout).Timeout; timeout != 10*time.Second {
		t.Fatalf("metadata timeout = %v", timeout)
	}
	if timeout := remote.httpClient(artifactDownloadTimeout).Timeout; timeout < 10*time.Minute {
		t.Fatalf("artifact timeout is too short for bounded streaming: %v", timeout)
	}
}

func signedRemoteServer(t *testing.T, windows, linux []byte) (*httptest.Server, ed25519.PublicKey) {
	t.Helper()
	digest := func(data []byte) string {
		hash := sha256.Sum256(data)
		return hex.EncodeToString(hash[:])
	}
	raw := []byte(fmt.Sprintf(`{"schema":1,"latest_version":"0.4.0","minimum_supported_version":"0.3.2","release_notes":"online update","artifacts":{"windows-amd64":{"file":"spacechat-client-windows-amd64-0.4.0.exe","size":%d,"sha256":"%s"},"linux-amd64":{"file":"spacechat-client-linux-amd64-0.4.0","size":%d,"sha256":"%s"}}}`,
		len(windows), digest(windows), len(linux), digest(linux)))
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signature := ed25519.Sign(privateKey, raw)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/updates/v1/manifest":
			writer.Write(raw)
		case "/updates/v1/manifest.sig":
			writer.Write(signature)
		case "/updates/v1/artifacts/spacechat-client-windows-amd64-0.4.0.exe":
			writer.Write(windows)
		case "/updates/v1/artifacts/spacechat-client-linux-amd64-0.4.0":
			writer.Write(linux)
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)
	return server, publicKey
}

func websocketAddress(server *httptest.Server) string {
	return "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
}

func TestRemoteCheckVerifiesManifestAndSelectsPlatform(t *testing.T) {
	server, publicKey := signedRemoteServer(t, []byte("windows"), []byte("linux"))
	current, _ := ParseVersion("0.3.2")
	check, err := (Remote{PublicKey: publicKey}).Check(context.Background(), websocketAddress(server), current, "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if check.Decision != DecisionOptional || check.Latest.String() != "0.4.0" || check.Minimum.String() != "0.3.2" {
		t.Fatalf("wrong check result: %+v", check)
	}
	if check.Artifact.File != "spacechat-client-linux-amd64-0.4.0" || check.Server != websocketAddress(server) {
		t.Fatalf("wrong artifact/source: %+v", check)
	}
}

func TestRemoteCheckRejectsRedirectAndInvalidSignature(t *testing.T) {
	redirect := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, "https://example.invalid/manifest", http.StatusFound)
	}))
	defer redirect.Close()
	publicKey, _, _ := ed25519.GenerateKey(rand.Reader)
	current, _ := ParseVersion("0.3.2")
	if _, err := (Remote{PublicKey: publicKey}).Check(context.Background(), websocketAddress(redirect), current, "linux", "amd64"); err == nil {
		t.Fatal("redirect accepted")
	}

	server, _ := signedRemoteServer(t, []byte("windows"), []byte("linux"))
	otherPublic, _, _ := ed25519.GenerateKey(rand.Reader)
	if _, err := (Remote{PublicKey: otherPublic}).Check(context.Background(), websocketAddress(server), current, "linux", "amd64"); err == nil || !strings.Contains(err.Error(), "signature") {
		t.Fatalf("expected signature failure, got %v", err)
	}
}

func TestRemoteDownloadChecksSizeDigestAndProgress(t *testing.T) {
	server, publicKey := signedRemoteServer(t, []byte("windows"), []byte("linux"))
	current, _ := ParseVersion("0.3.2")
	remote := Remote{PublicKey: publicKey}
	check, err := remote.Check(context.Background(), websocketAddress(server), current, "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "client")
	var received, total int64
	if err = remote.Download(context.Background(), check.Server, check.Artifact, destination, func(current, maximum int64) {
		received, total = current, maximum
	}); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(destination)
	if err != nil || string(contents) != "linux" {
		t.Fatalf("download = %q, %v", contents, err)
	}
	if received != 5 || total != 5 {
		t.Fatalf("progress = %d/%d", received, total)
	}
}

func TestRemoteDownloadRemovesInvalidPartialFile(t *testing.T) {
	tests := map[string]func([]byte) Artifact{
		"short": func(data []byte) Artifact {
			hash := sha256.Sum256(data)
			return Artifact{File: "spacechat-client-linux-amd64-0.4.0", Size: int64(len(data) + 1), SHA256: hex.EncodeToString(hash[:])}
		},
		"digest": func(data []byte) Artifact {
			return Artifact{File: "spacechat-client-linux-amd64-0.4.0", Size: int64(len(data)), SHA256: strings.Repeat("0", 64)}
		},
	}
	for name, artifactFor := range tests {
		t.Run(name, func(t *testing.T) {
			data := []byte("bad")
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) { writer.Write(data) }))
			defer server.Close()
			destination := filepath.Join(t.TempDir(), "partial")
			remote := Remote{}
			if err := remote.Download(context.Background(), websocketAddress(server), artifactFor(data), destination, nil); err == nil {
				t.Fatal("invalid download accepted")
			}
			if _, err := os.Stat(destination); !os.IsNotExist(err) {
				t.Fatalf("partial file remains: %v", err)
			}
		})
	}
}

func TestRemoteDownloadRejectsOversizedBody(t *testing.T) {
	data := []byte("toolong")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) { io.WriteString(writer, string(data)) }))
	defer server.Close()
	hash := sha256.Sum256(data[:3])
	artifact := Artifact{File: "spacechat-client-linux-amd64-0.4.0", Size: 3, SHA256: hex.EncodeToString(hash[:])}
	destination := filepath.Join(t.TempDir(), "partial")
	if err := (Remote{}).Download(context.Background(), websocketAddress(server), artifact, destination, nil); err == nil {
		t.Fatal("oversized response accepted")
	}
}

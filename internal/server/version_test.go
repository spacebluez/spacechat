package server

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"xchat/internal/protocol"
	"xchat/internal/securestore"
	"xchat/internal/update"
)

func versionedRoomService(t *testing.T, minimum string) *Server {
	t.Helper()
	database, err := securestore.Open(filepath.Join(t.TempDir(), "rooms.db"), bytes.Repeat([]byte{9}, 32))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	parsed, err := update.ParseVersion(minimum)
	if err != nil {
		t.Fatal(err)
	}
	service := NewRooms(database, WithUpdateCatalog(&UpdateCatalog{minimum: parsed}))
	t.Cleanup(service.Close)
	return service
}

func versionedSession(t *testing.T) *session {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return &session{ctx: ctx, cancel: cancel, outgoing: make(chan protocol.Frame, 256)}
}

func TestClientVersionRejectedBeforeRoomAuthentication(t *testing.T) {
	service := versionedRoomService(t, "0.3.2")
	failure := service.join(versionedSession(t), protocol.Join{
		Nickname:      "old",
		ClientVersion: "0.3.1",
		ClientOS:      "windows",
		ClientArch:    "amd64",
	})
	if failure.Code != "upgrade_required" || failure.MinimumVersion != "0.3.2" {
		t.Fatalf("wrong failure: %+v", failure)
	}
}

func TestClientVersionPolicy(t *testing.T) {
	tests := []struct {
		name        string
		minimum     string
		version     string
		goos        string
		arch        string
		wantFailure string
	}{
		{name: "missing", minimum: "0.3.2", goos: "windows", arch: "amd64", wantFailure: "upgrade_required"},
		{name: "malformed", minimum: "0.3.2", version: "v0.4.0", goos: "windows", arch: "amd64", wantFailure: "upgrade_required"},
		{name: "unsupported os", minimum: "0.3.2", version: "0.4.0", goos: "darwin", arch: "amd64", wantFailure: "unsupported_client"},
		{name: "unsupported arch", minimum: "0.3.2", version: "0.4.0", goos: "linux", arch: "arm64", wantFailure: "unsupported_client"},
		{name: "minimum", minimum: "0.3.2", version: "0.3.2", goos: "windows", arch: "amd64"},
		{name: "newer", minimum: "0.3.2", version: "0.5.0", goos: "linux", arch: "amd64"},
		{name: "migration missing metadata", minimum: "0.0.0"},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := versionedRoomService(t, test.minimum)
			failure := service.join(versionedSession(t), protocol.Join{
				Nickname:      fmt.Sprintf("user-%d", index),
				AccessKey:     "room-key",
				ClientVersion: test.version,
				ClientOS:      test.goos,
				ClientArch:    test.arch,
			})
			if failure.Code != test.wantFailure {
				t.Fatalf("failure = %+v, want code %q", failure, test.wantFailure)
			}
		})
	}
}

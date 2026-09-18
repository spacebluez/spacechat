package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"xchat/internal/protocol"
	"xchat/internal/store"
)

func TestAuthenticationBeforeHistoryOrPresence(t *testing.T) {
	repository, err := store.Open(filepath.Join(t.TempDir(), "chat.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	repository.Append("private", "must not leak")
	service := New(repository, "test-access-key")
	defer service.Close()
	httpServer := httptest.NewServer(service.Handler())
	defer httpServer.Close()
	for _, key := range []string{"", "wrong-key", "test-access-key"} {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		connection, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(httpServer.URL, "http")+"/ws", nil)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		err = wsjson.Write(ctx, connection, protocol.Encode("join", "join", protocol.Join{Nickname: "Alice", AccessKey: key}))
		if err != nil {
			t.Fatal(err)
		}
		var frame protocol.Frame
		if err = wsjson.Read(ctx, connection, &frame); err != nil {
			t.Fatal(err)
		}
		if key == "test-access-key" {
			if frame.Type != "welcome" {
				t.Fatalf("valid key rejected: %s", frame.Type)
			}
		} else {
			if frame.Type != "error" {
				t.Fatalf("unauthorized client received %s", frame.Type)
			}
			var failure protocol.Failure
			json.Unmarshal(frame.Payload, &failure)
			if failure.Code != "unauthorized" {
				t.Fatalf("wrong error %+v", failure)
			}
		}
		connection.CloseNow()
		cancel()
	}
}
func TestEmptyConfiguredKeyFailsClosed(t *testing.T) {
	repository, _, _ := fixture(t)
	service := New(repository, "")
	defer service.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := &session{ctx: ctx, cancel: cancel, outgoing: make(chan protocol.Frame, 10)}
	if failure := service.join(client, protocol.Join{Nickname: "Alice"}); failure.Code != "unauthorized" {
		t.Fatal("empty configured key allowed access")
	}
}

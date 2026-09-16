package client

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"xchat/internal/protocol"
	"xchat/internal/server"
	"xchat/internal/store"
)

func TestClientReceivesClearAndKeepsConnection(t *testing.T) {
	repository, err := store.Open(filepath.Join(t.TempDir(), "chat.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	repository.Append("seed", "old")
	service := server.New(repository, "test-access-key")
	defer service.Close()
	httpServer := httptest.NewServer(service.Handler())
	defer httpServer.Close()
	network := New("ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { network.Run(ctx, "Alice", "test-access-key"); close(done) }()
	defer func() { cancel(); <-done }()
	awaitEvent(t, network.Events(), func(event Event) bool { return event.State == "connected" })
	result, err := service.ClearHistory()
	if err != nil {
		t.Fatal(err)
	}
	received := awaitEvent(t, network.Events(), func(event Event) bool { return event.Frame != nil && event.Frame.Type == "history_cleared" })
	var cleared protocol.Cleared
	json.Unmarshal(received.Frame.Payload, &cleared)
	if cleared.InstanceID != result.InstanceID {
		t.Fatal("wrong clear generation")
	}
	if err = network.Send("new", "after"); err != nil {
		t.Fatal(err)
	}
	awaitEvent(t, network.Events(), func(event Event) bool { return event.Frame != nil && event.Frame.Type == "ack" })
}
func TestWrongKeyStopsRetrying(t *testing.T) {
	repository, err := store.Open(filepath.Join(t.TempDir(), "chat.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	service := server.New(repository, "test-access-key")
	defer service.Close()
	httpServer := httptest.NewServer(service.Handler())
	defer httpServer.Close()
	network := New("ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go network.Run(ctx, "Alice", "wrong-key")
	awaitEvent(t, network.Events(), func(event Event) bool {
		if event.Frame != nil {
			t.Fatalf("unauthorized data %s", event.Frame.Type)
		}
		return event.State == "unauthorized"
	})
	if _, open := <-network.Events(); open {
		t.Fatal("wrong key retried")
	}
}

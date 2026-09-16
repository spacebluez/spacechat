package client

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"xchat/internal/protocol"
	"xchat/internal/server"
	"xchat/internal/store"
)

func awaitEvent(t *testing.T, events <-chan Event, predicate func(Event) bool) Event {
	t.Helper()
	timeout := time.NewTimer(10 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case event, open := <-events:
			if !open {
				t.Fatal("closed before event")
			}
			if predicate(event) {
				return event
			}
		case <-timeout.C:
			t.Fatal("event timed out")
		}
	}
}
func TestConnectSendHistoryAndCancel(t *testing.T) {
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
	done := make(chan struct{})
	go func() { network.Run(ctx, "小明", "test-access-key"); close(done) }()
	awaitEvent(t, network.Events(), func(event Event) bool { return event.State == "connected" })
	if err = network.Send("send-1", "你好"); err != nil {
		t.Fatal(err)
	}
	event := awaitEvent(t, network.Events(), func(event Event) bool { return event.Frame != nil && event.Frame.Type == "ack" })
	if event.Frame.RequestID != "send-1" {
		t.Fatal("wrong acknowledgement")
	}
	if err = network.History(2); err != nil {
		t.Fatal(err)
	}
	awaitEvent(t, network.Events(), func(event Event) bool { return event.Frame != nil && event.Frame.Type == "history" })
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cancel did not terminate")
	}
	if network.Send("offline", "no") == nil {
		t.Fatal("sent offline")
	}
}
func TestResumeFromEmptyAndNameConflict(t *testing.T) {
	repository, err := store.Open(filepath.Join(t.TempDir(), "chat.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	service := server.New(repository, "test-access-key")
	defer service.Close()
	httpServer := httptest.NewServer(service.Handler())
	defer httpServer.Close()
	address := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws"
	network := New(address)
	network.instance = repository.InstanceID()
	for index := 0; index < 120; index++ {
		repository.Append("seed", "补齐")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go network.Run(ctx, "Alice", "test-access-key")
	count := 0
	awaitEvent(t, network.Events(), func(event Event) bool {
		if event.Frame != nil && event.Frame.Type == "sync" {
			page, err := decode[protocol.Page](*event.Frame)
			if err != nil {
				t.Fatal(err)
			}
			count += len(page.Messages)
		}
		return event.State == "connected"
	})
	if count != 120 {
		t.Fatalf("synced %d", count)
	}
	duplicate := New(address)
	go duplicate.Run(ctx, "Alice", "test-access-key")
	awaitEvent(t, duplicate.Events(), func(event Event) bool { return event.State == "name_taken" })
}
func TestBadURLRejected(t *testing.T) {
	if ValidateAddress("http://example.com") == nil {
		t.Fatal("accepted non websocket")
	}
	if ValidateAddress("ws://127.0.0.1:18080/ws") != nil {
		t.Fatal("rejected valid address")
	}
}

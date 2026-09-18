package client

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"xchat/internal/protocol"
	"xchat/internal/server"
	"xchat/internal/store"
)

func TestAutomaticReconnectAfterServerRestart(t *testing.T) {
	repository, err := store.Open(filepath.Join(t.TempDir(), "chat.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	service := server.New(repository, "test-access-key")
	httpServer := &http.Server{Handler: service.Handler()}
	go httpServer.Serve(listener)
	defer func() { service.Close(); httpServer.Close() }()
	network := New("ws://" + address + "/ws")
	network.retryMin = 50 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	defer func() { cancel(); <-done }()
	go func() { network.Run(ctx, "小明", "test-access-key"); close(done) }()
	awaitEvent(t, network.Events(), func(event Event) bool { return event.State == "connected" })
	service.Close()
	httpServer.Close()
	awaitEvent(t, network.Events(), func(event Event) bool { return event.State == "disconnected" })
	for index := 0; index < 205; index++ {
		if _, err = repository.Append("seed", "离线消息"); err != nil {
			t.Fatal(err)
		}
	}
	listener, err = net.Listen("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	service = server.New(repository, "test-access-key")
	httpServer = &http.Server{Handler: service.Handler()}
	go httpServer.Serve(listener)
	seen := make(map[int64]bool)
	awaitEvent(t, network.Events(), func(event Event) bool {
		if event.Frame != nil && event.Frame.Type == "sync" {
			var page protocol.Page
			if err := json.Unmarshal(event.Frame.Payload, &page); err != nil {
				t.Fatal(err)
			}
			for _, message := range page.Messages {
				if seen[message.ID] {
					t.Fatal("duplicate sync")
				}
				seen[message.ID] = true
			}
		}
		return event.State == "connected"
	})
	if len(seen) != 205 || !seen[1] || !seen[205] {
		t.Fatalf("missed messages: %d", len(seen))
	}
	if err = network.Send("after-restart", "恢复"); err != nil {
		t.Fatal(err)
	}
	awaitEvent(t, network.Events(), func(event Event) bool { return event.Frame != nil && event.Frame.Type == "ack" })
}
func TestMaximumChineseMessagePage(t *testing.T) {
	repository, err := store.Open(filepath.Join(t.TempDir(), "chat.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	for index := 0; index < 100; index++ {
		repository.Append("中文", strings.Repeat("中", 2000))
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	service := server.New(repository, "test-access-key")
	httpServer := &http.Server{Handler: service.Handler()}
	go httpServer.Serve(listener)
	defer httpServer.Close()
	defer service.Close()
	network := New("ws://" + listener.Addr().String() + "/ws")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	defer func() { cancel(); <-done }()
	go func() { network.Run(ctx, "viewer", "test-access-key"); close(done) }()
	count := 0
	awaitEvent(t, network.Events(), func(event Event) bool {
		if event.Frame != nil && event.Frame.Type == "history" {
			var page protocol.Page
			json.Unmarshal(event.Frame.Payload, &page)
			count += len(page.Messages)
		}
		return event.State == "connected"
	})
	if count != 100 {
		t.Fatalf("received %d", count)
	}
}

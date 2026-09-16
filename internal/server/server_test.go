package server

import (
	"context"
	"encoding/json"
	"errors"
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

type peer struct {
	connection *websocket.Conn
	ctx        context.Context
}

func connect(t *testing.T, address string, join protocol.Join) *peer {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	connection, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(address, "http")+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { connection.CloseNow() })
	client := &peer{connection, ctx}
	join.AccessKey = "test-access-key"
	client.send(t, "join", "join", join)
	return client
}
func (client *peer) send(t *testing.T, kind, id string, payload any) {
	t.Helper()
	if err := wsjson.Write(client.ctx, client.connection, protocol.Encode(kind, id, payload)); err != nil {
		t.Fatal(err)
	}
}
func (client *peer) receive(t *testing.T, kind string) protocol.Frame {
	t.Helper()
	for {
		var frame protocol.Frame
		if err := wsjson.Read(client.ctx, client.connection, &frame); err != nil {
			t.Fatal(err)
		}
		if frame.Type == kind {
			return frame
		}
	}
}
func fixture(t *testing.T) (*store.Store, *Server, *httptest.Server) {
	t.Helper()
	repository, err := store.Open(filepath.Join(t.TempDir(), "chat.db"))
	if err != nil {
		t.Fatal(err)
	}
	service := New(repository, "test-access-key")
	httpServer := httptest.NewServer(service.Handler())
	t.Cleanup(func() { service.Close(); httpServer.Close(); repository.Close() })
	return repository, service, httpServer
}
func TestJoinBroadcastDuplicateAndHistory(t *testing.T) {
	_, _, httpServer := fixture(t)
	alice := connect(t, httpServer.URL, protocol.Join{Nickname: "小明"})
	alice.receive(t, "sync_complete")
	duplicate := connect(t, httpServer.URL, protocol.Join{Nickname: "小明"})
	var failure protocol.Failure
	json.Unmarshal(duplicate.receive(t, "error").Payload, &failure)
	if failure.Code != "name_taken" {
		t.Fatalf("wrong error %+v", failure)
	}
	bob := connect(t, httpServer.URL, protocol.Join{Nickname: "Bob"})
	bob.receive(t, "sync_complete")
	alice.send(t, "send", "message-1", protocol.Send{Body: "你好"})
	var message protocol.Message
	json.Unmarshal(bob.receive(t, "message").Payload, &message)
	if message.ID != 1 || message.Nickname != "小明" || message.Body != "你好" {
		t.Fatalf("bad message %+v", message)
	}
	if alice.receive(t, "ack").RequestID != "message-1" {
		t.Fatal("lost request id")
	}
	alice.send(t, "send", "invalid", protocol.Send{Body: "\x1b[31m"})
	if alice.receive(t, "error").RequestID != "invalid" {
		t.Fatal("missing validation error")
	}
	bob.send(t, "history", "older", protocol.Query{BeforeID: 2})
	var page protocol.Page
	json.Unmarshal(bob.receive(t, "history").Payload, &page)
	if len(page.Messages) != 1 {
		t.Fatalf("history %+v", page)
	}
}
func TestResumeMultiplePagesWithConcurrentLiveMessage(t *testing.T) {
	repository, _, httpServer := fixture(t)
	for index := 0; index < 206; index++ {
		if _, err := repository.Append("seed", "历史"); err != nil {
			t.Fatal(err)
		}
	}
	alice := connect(t, httpServer.URL, protocol.Join{Nickname: "Alice", InstanceID: repository.InstanceID(), AfterID: 1})
	alice.receive(t, "welcome")
	var page protocol.Page
	json.Unmarshal(alice.receive(t, "sync").Payload, &page)
	if len(page.Messages) != 100 || page.Messages[0].ID != 2 || !page.HasMore {
		t.Fatalf("first sync %+v", page)
	}
	bob := connect(t, httpServer.URL, protocol.Join{Nickname: "Bob"})
	bob.receive(t, "sync_complete")
	bob.send(t, "send", "live", protocol.Send{Body: "同步期间"})
	bob.receive(t, "ack")
	alice.send(t, "sync", "next", protocol.Query{AfterID: 101})
	json.Unmarshal(alice.receive(t, "sync").Payload, &page)
	if len(page.Messages) != 100 || page.Messages[0].ID != 102 || !page.HasMore {
		t.Fatal("second sync")
	}
	alice.send(t, "sync", "last", protocol.Query{AfterID: 201})
	json.Unmarshal(alice.receive(t, "sync").Payload, &page)
	if len(page.Messages) != 5 || page.Messages[4].ID != 206 || page.HasMore {
		t.Fatalf("last sync %+v", page)
	}
	alice.receive(t, "sync_complete")
	var live protocol.Message
	json.Unmarshal(alice.receive(t, "message").Payload, &live)
	if live.ID != 207 {
		t.Fatalf("lost live message %+v", live)
	}
}
func TestResumeFromEmptyRoom(t *testing.T) {
	repository, _, httpServer := fixture(t)
	for index := 0; index < 120; index++ {
		repository.Append("seed", "消息")
	}
	client := connect(t, httpServer.URL, protocol.Join{Nickname: "Alice", InstanceID: repository.InstanceID(), AfterID: 0})
	var page protocol.Page
	json.Unmarshal(client.receive(t, "sync").Payload, &page)
	if len(page.Messages) != 100 || page.Messages[0].ID != 1 || !page.HasMore {
		t.Fatalf("empty cursor skipped history %+v", page)
	}
}

type brokenStore struct{ *store.Store }

func (repository brokenStore) Append(name, body string) (protocol.Message, error) {
	return protocol.Message{}, errors.New("disk full")
}
func TestWriteFailureNeverBroadcasts(t *testing.T) {
	repository, err := store.Open(filepath.Join(t.TempDir(), "chat.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	service := New(brokenStore{repository}, "test-access-key")
	httpServer := httptest.NewServer(service.Handler())
	defer httpServer.Close()
	defer service.Close()
	client := connect(t, httpServer.URL, protocol.Join{Nickname: "Alice"})
	client.receive(t, "sync_complete")
	client.send(t, "send", "failed", protocol.Send{Body: "失败"})
	frame := client.receive(t, "error")
	var failure protocol.Failure
	json.Unmarshal(frame.Payload, &failure)
	if failure.Code != "storage_error" || frame.RequestID != "failed" {
		t.Fatal("not reported")
	}
	latest, _ := repository.LatestID()
	if latest != 0 {
		t.Fatal("unexpected persistence")
	}
}
func TestSlowQueueIsDisconnected(t *testing.T) {
	_, service, _ := fixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	slow := &session{ctx: ctx, cancel: cancel, outgoing: make(chan protocol.Frame, 1)}
	slow.enqueue(protocol.Encode("presence", "", protocol.Presence{}))
	slow.enqueue(protocol.Encode("presence", "", protocol.Presence{}))
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("slow peer not stopped")
	}
	service.mu.Lock()
	service.mu.Unlock()
}

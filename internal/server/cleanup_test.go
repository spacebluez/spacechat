package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"xchat/internal/protocol"
	"xchat/internal/store"
)

func TestClearNotifiesSyncingAndOnlineClients(t *testing.T) {
	repository, service, httpServer := fixture(t)
	oldInstance := repository.InstanceID()
	for index := 0; index < 205; index++ {
		repository.Append("seed", "old")
	}
	online := connect(t, httpServer.URL, protocol.Join{Nickname: "online"})
	online.receive(t, "sync_complete")
	syncing := connect(t, httpServer.URL, protocol.Join{Nickname: "syncing", InstanceID: oldInstance, AfterID: 1})
	syncing.receive(t, "sync")
	result, err := service.ClearHistory()
	if err != nil || result.Deleted != 205 {
		t.Fatalf("clear %+v %v", result, err)
	}
	for _, client := range []*peer{online, syncing} {
		var event protocol.Cleared
		json.Unmarshal(client.receive(t, "history_cleared").Payload, &event)
		if event.InstanceID == oldInstance || event.Deleted != 205 {
			t.Fatal("wrong cleanup event")
		}
	}
	syncing.send(t, "sync", "stale", protocol.Query{AfterID: 101})
	online.send(t, "send", "after-clear", protocol.Send{Body: "new"})
	online.receive(t, "ack")
	var message protocol.Message
	json.Unmarshal(syncing.receive(t, "message").Payload, &message)
	if message.Body != "new" || message.ID <= 205 {
		t.Fatal("new message lost or ID reused")
	}
	returning := connect(t, httpServer.URL, protocol.Join{Nickname: "returning", InstanceID: oldInstance, AfterID: 205})
	var welcome protocol.Welcome
	json.Unmarshal(returning.receive(t, "welcome").Payload, &welcome)
	if welcome.Resumed || welcome.InstanceID == oldInstance {
		t.Fatal("stale cursor reused")
	}
	var page protocol.Page
	json.Unmarshal(returning.receive(t, "history").Payload, &page)
	if len(page.Messages) != 1 || page.Messages[0].Body != "new" {
		t.Fatal("old history restored")
	}
	response, err := http.Post(httpServer.URL+"/clear-history", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatal("public clear endpoint exposed")
	}
}

func TestCleanupRefreshesPresenceForSyncingClients(t *testing.T) {
	_, service, _ := fixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	syncing := &session{name: "Alice", ctx: ctx, cancel: cancel, syncing: true, outgoing: make(chan protocol.Frame, 8)}
	service.sessions["Alice"] = syncing
	if _, err := service.ClearHistory(); err != nil {
		t.Fatal(err)
	}
	if len(syncing.outgoing) != 2 {
		t.Fatal("clear must refresh online list after discarding deferred events")
	}
	<-syncing.outgoing
	frame := <-syncing.outgoing
	if frame.Type != "presence" {
		t.Fatal("missing refreshed presence")
	}
}

type failingClearStore struct{ *store.Store }

func (repository failingClearStore) Clear() (int64, error) {
	return 0, errors.New("injected cleanup failure")
}
func TestFailedClearDoesNotNotifyClients(t *testing.T) {
	repository, _, _ := fixture(t)
	repository.Append("seed", "keep")
	service := New(failingClearStore{repository}, "test-access-key")
	defer service.Close()
	httpServer := httptest.NewServer(service.Handler())
	defer httpServer.Close()
	client := connect(t, httpServer.URL, protocol.Join{Nickname: "Alice"})
	client.receive(t, "sync_complete")
	if _, err := service.ClearHistory(); err == nil {
		t.Fatal("failed clear accepted")
	}
	client.send(t, "send", "new", protocol.Send{Body: "continue"})
	frame := client.receive(t, "message")
	var message protocol.Message
	json.Unmarshal(frame.Payload, &message)
	if message.ID != 2 {
		t.Fatal("history corrupted")
	}
}

package server

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"xchat/internal/protocol"
	"xchat/internal/securestore"
)

func TestEncryptedRoomsWebSocketResumeAndIsolation(t *testing.T) {
	database, err := securestore.Open(filepath.Join(t.TempDir(), "rooms.db"), bytes.Repeat([]byte{6}, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	first, _ := database.Room("first")
	second, _ := database.Room("second")
	for index := 0; index < 205; index++ {
		if _, err := first.Append("seed", "private"); err != nil {
			t.Fatal(err)
		}
	}
	second.Append("other", "different")
	service := NewRooms(database)
	defer service.Close()
	httpServer := httptest.NewServer(service.Handler())
	defer httpServer.Close()
	alice := connect(t, httpServer.URL, protocol.Join{Nickname: "same", AccessKey: "first", InstanceID: first.InstanceID(), AfterID: 1})
	var page protocol.Page
	json.Unmarshal(alice.receive(t, "sync").Payload, &page)
	if len(page.Messages) != 100 || !page.HasMore || page.Messages[0].Body != "private" {
		t.Fatal("room sync incorrect")
	}
	other := connect(t, httpServer.URL, protocol.Join{Nickname: "same", AccessKey: "second", InstanceID: first.InstanceID(), AfterID: 205})
	var welcome protocol.Welcome
	json.Unmarshal(other.receive(t, "welcome").Payload, &welcome)
	if welcome.Resumed || len(welcome.Users) != 1 {
		t.Fatal("foreign cursor/presence accepted")
	}
	json.Unmarshal(other.receive(t, "history").Payload, &page)
	if len(page.Messages) != 1 || page.Messages[0].Body != "different" {
		t.Fatal("cross-room initial history")
	}
	other.receive(t, "sync_complete")
	alice.send(t, "sync", "next", protocol.Query{AfterID: 101})
	json.Unmarshal(alice.receive(t, "sync").Payload, &page)
	if len(page.Messages) != 100 || !page.HasMore {
		t.Fatal("second page")
	}
	alice.send(t, "sync", "last", protocol.Query{AfterID: 201})
	json.Unmarshal(alice.receive(t, "sync").Payload, &page)
	if len(page.Messages) != 4 || page.HasMore {
		t.Fatal("last page leaked other room")
	}
	alice.receive(t, "sync_complete")
	alice.send(t, "send", "multiline", protocol.Send{Body: "line one\nline two"})
	alice.receive(t, "ack")
	other.send(t, "history", "older", protocol.Query{BeforeID: 99999})
	json.Unmarshal(other.receive(t, "history").Payload, &page)
	if len(page.Messages) != 1 {
		t.Fatal("cross-room history query")
	}
}

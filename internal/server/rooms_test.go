package server

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"xchat/internal/protocol"
	"xchat/internal/securestore"
)

func TestRoomIsolationAndGlobalCleanup(t *testing.T) {
	database, err := securestore.Open(filepath.Join(t.TempDir(), "rooms.db"), bytes.Repeat([]byte{3}, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := NewRooms(database)
	defer service.Close()
	makeSession := func() *session {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		return &session{ctx: ctx, cancel: cancel, outgoing: make(chan protocol.Frame, 256)}
	}
	join := func(name, key string) *session {
		client := makeSession()
		if failure := service.join(client, protocol.Join{Nickname: name, AccessKey: key}); failure.Code != "" {
			t.Fatal(failure)
		}
		return client
	}
	first := join("same-name", "room-one")
	second := join("same-name", "room-two")
	friend := join("friend", "room-one")
	for _, current := range []*session{first, second, friend} {
		for len(current.outgoing) > 0 {
			frame := <-current.outgoing
			if frame.Type == "presence" && current == second {
				var presence protocol.Presence
				json.Unmarshal(frame.Payload, &presence)
				if len(presence.Users) != 1 || presence.Users[0] != "same-name" {
					t.Fatal("cross-room presence")
				}
			}
		}
	}
	if failure := service.join(makeSession(), protocol.Join{Nickname: "same-name", AccessKey: "room-one"}); failure.Code != "name_taken" {
		t.Fatal("duplicate allowed", failure)
	}
	if failure := service.join(makeSession(), protocol.Join{Nickname: "invalid"}); failure.Code != "unauthorized" {
		t.Fatal("empty key accepted")
	}
	service.handle(first, protocol.Encode("send", "first", protocol.Send{Body: "private"}))
	if len(second.outgoing) != 0 {
		t.Fatal("cross-room broadcast")
	}
	if len(friend.outgoing) != 1 {
		t.Fatal("same-room broadcast missing")
	}
	service.handle(second, protocol.Encode("history", "query", protocol.Query{BeforeID: 99999}))
	var page protocol.Page
	json.Unmarshal((<-second.outgoing).Payload, &page)
	if len(page.Messages) != 0 {
		t.Fatal("cross-room history")
	}
	oldInstance := first.repository.InstanceID()
	result, err := service.ClearHistory()
	if err != nil || result.Deleted != 1 {
		t.Fatal(result, err)
	}
	for _, current := range []*session{first, second, friend} {
		seen := false
		for len(current.outgoing) > 0 {
			frame := <-current.outgoing
			if frame.Type == "history_cleared" {
				var cleared protocol.Cleared
				json.Unmarshal(frame.Payload, &cleared)
				if cleared.InstanceID != current.repository.InstanceID() {
					t.Fatal("wrong room epoch")
				}
				seen = true
			}
		}
		if !seen {
			t.Fatal("cleanup notification missing")
		}
	}
	if first.repository.InstanceID() == oldInstance {
		t.Fatal("epoch unchanged")
	}
	service.handle(first, protocol.Encode("send", "next", protocol.Send{Body: "new"}))
	if len(second.outgoing) != 0 || len(friend.outgoing) != 1 {
		t.Fatal("post-cleanup room isolation")
	}
}

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"xchat/internal/protocol"
	"xchat/internal/securestore"
)

func TestRecallBroadcastResumeAndNicknameImpersonation(t *testing.T) {
	db, err := securestore.Open(filepath.Join(t.TempDir(), "rooms.db"), bytes.Repeat([]byte{8}, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service := NewRooms(db)
	defer service.Close()
	join := func(name, key, token, epoch string, cursor int64) *session {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		peer := &session{ctx: ctx, cancel: cancel, outgoing: make(chan protocol.Frame, 256)}
		if failure := service.join(peer, protocol.Join{Nickname: name, AccessKey: key, ClientToken: token, InstanceID: epoch, AfterID: cursor}); failure.Code != "" {
			t.Fatal(failure)
		}
		return peer
	}
	flush := func(peers ...*session) {
		for _, peer := range peers {
			for len(peer.outgoing) > 0 {
				<-peer.outgoing
			}
		}
	}
	find := func(peer *session, kind string) protocol.Frame {
		for len(peer.outgoing) > 0 {
			frame := <-peer.outgoing
			if frame.Type == kind {
				return frame
			}
		}
		t.Fatalf("missing %s", kind)
		return protocol.Frame{}
	}
	alice := join("Alice", "one", strings.Repeat("a1", 32), "", 0)
	bob := join("Bob", "one", strings.Repeat("b2", 32), "", 0)
	other := join("Elsewhere", "two", strings.Repeat("c3", 32), "", 0)
	flush(alice, bob, other)
	service.handle(alice, protocol.Encode("send", "m1", protocol.Send{Body: "hello @Bob @Elsewhere"}))
	var sent, received protocol.Message
	json.Unmarshal(find(alice, "ack").Payload, &sent)
	json.Unmarshal(find(bob, "message").Payload, &received)
	if !sent.CanRecall || received.CanRecall || len(received.Mentions) != 1 || received.Mentions[0] != "Bob" {
		t.Fatal("wrong permissions or mentions", sent, received)
	}
	if bytes.Contains(protocol.Encode("message", "", sent).Payload, []byte(alice.owner)) {
		t.Fatal("owner leaked on wire")
	}
	epoch := alice.repository.InstanceID()
	service.handle(bob, protocol.Encode("recall", "bad", protocol.Recall{MessageID: sent.ID}))
	var failure protocol.Failure
	json.Unmarshal(find(bob, "error").Payload, &failure)
	if failure.Code != "recall_denied" {
		t.Fatal(failure)
	}
	service.handle(alice, protocol.Encode("recall", "r1", protocol.Recall{MessageID: sent.ID}))
	var recalled protocol.Recalled
	json.Unmarshal(find(bob, "recalled").Payload, &recalled)
	if recalled.Message.Body != "" || !recalled.Message.Recalled || recalled.InstanceID == epoch {
		t.Fatal(recalled)
	}
	if len(other.outgoing) != 0 {
		t.Fatal("cross-room event leak")
	}
	resume := join("Offline", "one", strings.Repeat("d4", 32), epoch, sent.ID)
	var welcome protocol.Welcome
	json.Unmarshal(find(resume, "welcome").Payload, &welcome)
	if welcome.Resumed {
		t.Fatal("offline client would retain recalled body")
	}
	var page protocol.Page
	json.Unmarshal(find(resume, "history").Payload, &page)
	if len(page.Messages) != 1 || !page.Messages[0].Recalled || page.Messages[0].Body != "" {
		t.Fatal(page)
	}
	service.handle(alice, protocol.Encode("send", "m2", protocol.Send{Body: "another"}))
	json.Unmarshal(find(alice, "ack").Payload, &sent)
	service.leave(alice)
	impostor := join("Alice", "one", strings.Repeat("e5", 32), "", 0)
	flush(impostor)
	service.handle(impostor, protocol.Encode("recall", "bad", protocol.Recall{MessageID: sent.ID}))
	json.Unmarshal(find(impostor, "error").Payload, &failure)
	if failure.Code != "recall_denied" {
		t.Fatal("nickname impersonation can recall", failure)
	}
}

func TestRecallDuringPagedSyncArrivesAfterSyncComplete(t *testing.T) {
	db, err := securestore.Open(filepath.Join(t.TempDir(), "rooms.db"), bytes.Repeat([]byte{9}, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service := NewRooms(db)
	defer service.Close()
	join := func(request protocol.Join) *session {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		peer := &session{ctx: ctx, cancel: cancel, outgoing: make(chan protocol.Frame, 256)}
		if failure := service.join(peer, request); failure.Code != "" {
			t.Fatal(failure)
		}
		return peer
	}
	alice := join(protocol.Join{Nickname: "Alice", AccessKey: "one", ClientToken: strings.Repeat("ab", 32)})
	room, _ := db.Room("one")
	for i := 0; i < 205; i++ {
		if _, err := room.AppendChat("Alice", "body", alice.owner, nil); err != nil {
			t.Fatal(err)
		}
	}
	bob := join(protocol.Join{Nickname: "Bob", AccessKey: "one", InstanceID: room.InstanceID(), AfterID: 1})
	if !bob.syncing || bob.cursor != 101 {
		t.Fatal("paged sync did not start")
	}
	service.handle(alice, protocol.Encode("recall", "r", protocol.Recall{MessageID: 50}))
	for len(bob.outgoing) > 0 {
		if frame := <-bob.outgoing; frame.Type == "recalled" {
			t.Fatal("recall overtook history")
		}
	}
	service.handle(bob, protocol.Encode("sync", "next", protocol.Query{AfterID: 101}))
	service.handle(bob, protocol.Encode("sync", "last", protocol.Query{AfterID: 201}))
	complete, recalled := false, false
	for len(bob.outgoing) > 0 {
		frame := <-bob.outgoing
		if frame.Type == "sync_complete" {
			complete = true
		}
		if frame.Type == "recalled" {
			if !complete {
				t.Fatal("recall before sync completion")
			}
			var result protocol.Recalled
			if json.Unmarshal(frame.Payload, &result) != nil || result.Message.ID != 50 || result.Message.Body != "" || result.InstanceID != room.InstanceID() {
				t.Fatal("bad deferred recall")
			}
			recalled = true
		}
	}
	if !complete || !recalled {
		t.Fatal("recall lost during sync")
	}
}

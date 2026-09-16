package tui

import (
	"testing"
	"xchat/internal/protocol"
)

func TestClearResetsHistoryButPreservesDraft(t *testing.T) {
	model := New("ws://localhost:18080/ws")
	model.joined = true
	model.connected = true
	model.input.SetValue("草稿")
	model.accessKey.SetValue("test-access-key")
	model.hasMore = true
	model.loading = true
	model.pending["old"] = "old"
	model.issues = []string{"old failure"}
	model.Update(event("message", "", protocol.Message{ID: 1, Nickname: "A", Body: "old"}))
	model.Update(event("history_cleared", "", protocol.Cleared{InstanceID: "new", Deleted: 1}))
	if len(model.messages) != 0 || len(model.pending) != 0 || len(model.issues) != 0 || model.hasMore || model.loading {
		t.Fatal("old history state remained")
	}
	if model.input.Value() != "草稿" || model.accessKey.Value() != "test-access-key" || !model.connected {
		t.Fatal("session or draft lost")
	}
	model.Update(event("message", "", protocol.Message{ID: 2, Nickname: "A", Body: "new"}))
	if len(model.messages) != 1 {
		t.Fatal("cannot show post-clear message")
	}
}

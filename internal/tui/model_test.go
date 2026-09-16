package tui

import (
	"encoding/json"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"xchat/internal/client"
	"xchat/internal/protocol"
)

func event(kind, id string, payload any) networkEvent {
	frame := protocol.Encode(kind, id, payload)
	return networkEvent{event: client.Event{Frame: &frame}}
}
func TestDeduplicateAndUnknownSend(t *testing.T) {
	model := New("ws://localhost:18080/ws")
	model.joined = true
	model.connected = true
	model.pending["send-1"] = "待确认"
	model.Update(networkEvent{event: client.Event{State: "disconnected"}})
	if model.connected || len(model.pending) != 0 || !strings.Contains(model.notice, "结果未知") {
		t.Fatal("pending send not marked unknown")
	}
	message := protocol.Message{ID: 1, Nickname: "小明", Body: "你好", CreatedAt: "2026-09-16T01:00:00Z"}
	model.Update(event("message", "", message))
	model.Update(event("message", "", message))
	if len(model.messages) != 1 {
		t.Fatal("duplicate message")
	}
}
func TestNarrowScreenAndNewInstance(t *testing.T) {
	model := New("ws://localhost:18080/ws")
	model.joined = true
	model.Update(tea.WindowSizeMsg{Width: 35, Height: 15})
	model.Update(event("message", "", protocol.Message{ID: 9, Nickname: "Alice", Body: "中文"}))
	model.Update(event("welcome", "", protocol.Welcome{InstanceID: "new", Resumed: false, Users: []string{"Alice"}}))
	if len(model.messages) != 0 {
		t.Fatal("old instance history retained")
	}
	if strings.Contains(model.View(), "在线成员") {
		t.Fatal("sidebar should be hidden")
	}
	model.Update(networkEvent{event: client.Event{State: "name_taken", Detail: "昵称已占用"}})
	if model.joined {
		t.Fatal("cannot change nickname")
	}
	_, err := json.Marshal(model.messages)
	if err != nil {
		t.Fatal(err)
	}
}

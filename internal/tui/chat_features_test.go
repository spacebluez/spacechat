package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"xchat/internal/protocol"
)

func TestMemberPickerHandlesNamesAndPresence(t *testing.T) {
	model := kaomojiModel()
	model.name = "Alice"
	model.users = []string{"Alice", "Bob Smith", "小明"}
	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("@")})
	if model.members == nil {
		t.Fatal("typed @ did not open members")
	}
	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Bob")})
	kaomojiKey(model, tea.KeyEnter)
	if model.input.Value() != protocol.MentionText("Bob Smith")+" " {
		t.Fatal(model.input.Value())
	}
	model.input.SetValue("mail")
	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("@")})
	if model.members != nil || model.input.Value() != "mail@" {
		t.Fatal("email interrupted")
	}
	kaomojiKey(model, tea.KeyF4)
	model.Update(event("presence", "", protocol.Presence{Users: []string{"Alice"}}))
	kaomojiKey(model, tea.KeyEnter)
	if model.members == nil || model.input.Value() != "mail@" {
		t.Fatal("offline member inserted")
	}
	kaomojiKey(model, tea.KeyEsc)
}

func TestRecallReplacesBodyAndSurvivesLateHistory(t *testing.T) {
	model := kaomojiModel()
	model.name = "Bob"
	message := protocol.Message{ID: 4, Nickname: "Alice", Body: "private @Bob", Mentions: []string{"Bob"}, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	model.Update(event("message", "", message))
	if !strings.Contains(model.notice, "提到了你") || !strings.Contains(model.viewport.View(), "@你") {
		t.Fatal("mention not highlighted")
	}
	recalled := message
	recalled.Body, recalled.Mentions, recalled.Recalled = "", nil, true
	model.Update(event("recalled", "r1", protocol.Recalled{Message: recalled, InstanceID: "new"}))
	model.Update(event("history", "older", protocol.Page{Messages: []protocol.Message{message}}))
	if model.messages[0].Body != "" || !model.messages[0].Recalled || strings.Contains(model.View(), "private") {
		t.Fatal("recalled body resurrected")
	}
}

func TestRecallPickerUsesServerOwnershipAndConfirmation(t *testing.T) {
	model := kaomojiModel()
	model.name = "Alice"
	stamp := time.Now().UTC().Format(time.RFC3339Nano)
	model.messages = []protocol.Message{{ID: 1, Nickname: "Alice", Body: "same-name-not-mine", CreatedAt: stamp}, {ID: 2, Nickname: "Alice", Body: "mine", CreatedAt: stamp, CanRecall: true}}
	model.input.SetValue("draft")
	kaomojiKey(model, tea.KeyF5)
	if model.recaller == nil || len(model.recallable()) != 1 {
		t.Fatal("ownership ignored")
	}
	kaomojiKey(model, tea.KeyEnter)
	if model.recaller.confirmID != 2 || len(model.recalls) != 0 {
		t.Fatal("missing confirmation")
	}
	model.messages = append(model.messages, protocol.Message{ID: 3, Body: "new", CreatedAt: stamp, CanRecall: true})
	model.View()
	if model.recaller.confirmID != 2 || model.recaller.selected != 1 {
		t.Fatal("confirmation moved to new message")
	}
	kaomojiKey(model, tea.KeyEsc)
	if model.recaller != nil || model.input.Value() != "draft" {
		t.Fatal("cancel changed draft")
	}
}

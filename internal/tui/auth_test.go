package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"xchat/internal/client"
)

func TestAccessKeyMaskedAndRequired(t *testing.T) {
	model := New("ws://localhost:18080/ws")
	model.nickname.SetValue("Alice")
	model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if model.joined || !model.accessKey.Focused() {
		t.Fatal("entered before key input")
	}
	model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if model.joined {
		t.Fatal("empty key accepted")
	}
	model.accessKey.SetValue("test-access-key")
	if model.accessKey.EchoMode != textinput.EchoPassword || strings.Contains(model.View(), "test-access-key") {
		t.Fatal("key not masked")
	}
	model.applyEvent(client.Event{State: "unauthorized", Detail: "密钥错误"})
	if model.joined || !model.accessKey.Focused() || model.accessKey.Value() != "" {
		t.Fatal("cannot retry key")
	}
}

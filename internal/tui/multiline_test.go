package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"strings"
	"testing"
)

func TestMultilineDraftAndNewlineKey(t *testing.T) {
	model := New("ws://localhost/ws")
	model.joined = true
	model.input.Focus()
	model.input.SetValue("first")
	model.Update(tea.KeyMsg{Type: tea.KeyEnter, Alt: true})
	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("second")})
	if model.input.Value() != "first\nsecond" {
		t.Fatalf("newline lost: %q", model.input.Value())
	}
	model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if model.input.Value() != "first\nsecond" || !strings.Contains(model.notice, "草稿") {
		t.Fatal("disconnected Enter lost draft")
	}
	model.input.Reset()
	model.Update(pasteTextMsg{text: "粘贴第一行\r\n粘贴第二行"})
	if model.input.Value() != "粘贴第一行\n粘贴第二行" {
		t.Fatal("multiline paste flattened")
	}
}

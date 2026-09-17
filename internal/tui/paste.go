package tui

import (
	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"strings"
)

type pasteTextMsg struct {
	text string
	err  error
}

func readClipboard() tea.Msg {
	text, err := clipboard.ReadAll()
	return pasteTextMsg{text: text, err: err}
}

func normalizeNewlines(text string) string {
	return strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
}

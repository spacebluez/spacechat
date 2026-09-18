package tui

import (
	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"strings"
	"xchat/internal/client"
)

type pasteTextMsg struct {
	source *client.Client
	text   string
	err    error
}

func readClipboard() tea.Msg {
	text, err := clipboard.ReadAll()
	return pasteTextMsg{text: text, err: err}
}

func readRoomClipboard(source *client.Client) tea.Cmd {
	return func() tea.Msg { result := readClipboard().(pasteTextMsg); result.source = source; return result }
}

func normalizeNewlines(text string) string {
	return strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
}

type roomSwitchPaste struct {
	dialog   *roomSwitch
	keyField bool
	text     string
	err      error
}

func readSwitchClipboard(dialog *roomSwitch) tea.Cmd {
	keyField := dialog.key.Focused()
	return func() tea.Msg {
		text, err := clipboard.ReadAll()
		return roomSwitchPaste{dialog: dialog, keyField: keyField, text: text, err: err}
	}
}

func (model *Model) applySwitchPaste(message roomSwitchPaste) tea.Cmd {
	dialog := model.switcher
	if dialog == nil || dialog != message.dialog || dialog.candidate != nil || dialog.waiting || dialog.key.Focused() != message.keyField {
		return nil
	}
	if message.err != nil {
		dialog.notice = "无法读取剪贴板"
		return nil
	}
	return model.updateRoomSwitch(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(message.text), Paste: true})
}

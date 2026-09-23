package tui

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"xchat/internal/protocol"
)

type memberPicker struct {
	search   textinput.Model
	selected int
	notice   string
	typedAt  bool
}

func (model *Model) mentionBoundary() bool {
	lines := strings.Split(model.input.Value(), "\n")
	line := []rune(lines[model.input.Line()])
	info := model.input.LineInfo()
	column := info.StartColumn + info.ColumnOffset
	if column == 0 {
		return true
	}
	previous := line[column-1]
	return !unicode.IsLetter(previous) && !unicode.IsNumber(previous) && previous != '_' && previous != '@'
}

func (model *Model) openMembers() tea.Cmd {
	search := textinput.New()
	search.Placeholder = "搜索成员"
	search.CharLimit = 20
	search.Width = max(1, model.width-6)
	styleTextInput(&search)
	model.members = &memberPicker{search: search}
	model.input.Blur()
	return model.members.search.Focus()
}

func (model *Model) memberNames() []string {
	var names []string
	query := strings.ToLower(model.members.search.Value())
	for _, name := range model.users {
		if name != model.name && strings.Contains(strings.ToLower(name), query) {
			names = append(names, name)
		}
	}
	model.members.selected = min(model.members.selected, max(0, len(names)-1))
	return names
}

func (model *Model) updateMembers(message tea.Msg) tea.Cmd {
	picker := model.members
	names := model.memberNames()
	if key, ok := message.(tea.KeyMsg); ok && !key.Paste {
		switch key.String() {
		case "esc", "f4":
			model.members = nil
			return model.input.Focus()
		case "up":
			picker.selected = max(0, picker.selected-1)
			return nil
		case "down":
			picker.selected = min(max(0, len(names)-1), picker.selected+1)
			return nil
		case "enter":
			if len(names) == 0 {
				return nil
			}
			text := protocol.MentionText(names[picker.selected]) + " "
			if picker.typedAt {
				text = strings.TrimPrefix(text, "@")
			} else if !model.mentionBoundary() {
				text = " " + text
			}
			if !model.insertDraft(text) {
				picker.notice = model.notice
				return nil
			}
			model.members = nil
			return model.input.Focus()
		}
	}
	previous := picker.search.Value()
	var command tea.Cmd
	picker.search, command = picker.search.Update(message)
	if previous != picker.search.Value() {
		picker.selected = 0
		picker.notice = ""
	}
	return command
}

func (model *Model) insertDraft(text string) bool {
	if utf8.RuneCountInString(model.input.Value())+utf8.RuneCountInString(text) > model.input.CharLimit {
		model.notice = "插入后超过 2000 字符，草稿已保留"
		return false
	}
	// The textarea counts display columns; the protocol limit counts Unicode runes.
	limit := model.input.CharLimit
	model.input.CharLimit = 0
	model.input.InsertString(text)
	model.input.CharLimit = limit
	return true
}

func (model *Model) membersView() string {
	model.members.search.Width = max(1, model.width-6)
	return model.selectionView("@成员", model.members.search.View(), model.memberNames(), model.members.selected, model.members.notice)
}

package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"xchat/internal/kaomoji"
)

func kaomojiModel() *Model {
	model := New("ws://localhost/ws")
	model.SetKaomojiOverride(kaomoji.Default())
	model.joined, model.connected = true, true
	model.input.Focus()
	return model
}

func kaomojiKey(model *Model, kind tea.KeyType) { model.Update(tea.KeyMsg{Type: kind}) }

func TestKaomojiSearchInsertCancelAndCapacity(t *testing.T) {
	model := kaomojiModel()
	model.input.SetValue("前后")
	model.input.SetCursor(1)
	kaomojiKey(model, tea.KeyF3)
	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hello")})
	items := model.picker.matches()
	if len(items) != 1 || model.input.Focused() {
		t.Fatal("search or focus failed", items)
	}
	text := items[0].Text
	kaomojiKey(model, tea.KeyEnter)
	if model.input.Value() != "前"+text+"后" || model.picker != nil || !model.input.Focused() {
		t.Fatal("cursor insertion failed", model.input.Value())
	}
	draft := strings.Repeat("x", 1999)
	model.input.SetValue(draft)
	kaomojiKey(model, tea.KeyF3)
	kaomojiKey(model, tea.KeyEnter)
	if model.picker == nil || model.input.Value() != draft || model.picker.notice == "" {
		t.Fatal("partial kaomoji inserted at limit")
	}
	kaomojiKey(model, tea.KeyEsc)
	if model.picker != nil || model.input.Value() != draft {
		t.Fatal("cancel changed draft")
	}
}

func TestKaomojiCategoriesAndNoResults(t *testing.T) {
	model := kaomojiModel()
	kaomojiKey(model, tea.KeyF3)
	kaomojiKey(model, tea.KeyTab)
	if model.picker.category != 0 || len(model.picker.matches()) != len(model.catalog[0].Items) {
		t.Fatal("category filter failed")
	}
	kaomojiKey(model, tea.KeyShiftTab)
	if model.picker.category != -1 {
		t.Fatal("all category missing")
	}
	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("no-such-expression")})
	kaomojiKey(model, tea.KeyEnter)
	if model.picker == nil || model.input.Value() != "" || len(model.picker.matches()) != 0 {
		t.Fatal("empty search sent something")
	}
	kaomojiKey(model, tea.KeyEsc)
	model.connected = false
	kaomojiKey(model, tea.KeyF3)
	kaomojiKey(model, tea.KeyCtrlS)
	if model.picker == nil || model.picker.notice == "" {
		t.Fatal("offline quick send lost selection")
	}
}

func TestKaomojiInsertionCountsRunesInsteadOfTerminalColumns(t *testing.T) {
	model := kaomojiModel()
	draft := strings.Repeat("中", 1500)
	model.input.SetValue(draft)
	kaomojiKey(model, tea.KeyF3)
	text := model.picker.matches()[0].Text
	kaomojiKey(model, tea.KeyEnter)
	if model.input.Value() != draft+text {
		t.Fatal("wide draft prevented atomic insertion")
	}
}

func TestChatPickersStayWithinTerminal(t *testing.T) {
	for _, size := range [][2]int{{24, 10}, {40, 18}, {80, 24}, {120, 40}} {
		model := kaomojiModel()
		model.users = []string{"成员一", "very long member name"}
		model.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for _, key := range []tea.KeyType{tea.KeyF3, tea.KeyF4} {
			kaomojiKey(model, key)
			lines := strings.Split(model.View(), "\n")
			if len(lines) > size[1] {
				t.Fatalf("height overflow at %v", size)
			}
			for _, line := range lines {
				if ansi.StringWidth(line) > size[0] {
					t.Fatalf("width overflow at %v: %q", size, line)
				}
			}
			kaomojiKey(model, tea.KeyEsc)
		}
	}
}

func TestPickersAreUnavailableDuringLoginAndRoomSwitch(t *testing.T) {
	login := New("ws://localhost/ws")
	for _, key := range []tea.KeyType{tea.KeyF3, tea.KeyF4, tea.KeyF5} {
		kaomojiKey(login, key)
	}
	if login.picker != nil || login.members != nil || login.recaller != nil {
		t.Fatal("picker on login")
	}
	model := switchingModel()
	kaomojiKey(model, tea.KeyF2)
	kaomojiKey(model, tea.KeyF3)
	kaomojiKey(model, tea.KeyF4)
	if model.picker != nil || model.members != nil || model.switcher == nil {
		t.Fatal("nested modal")
	}
}

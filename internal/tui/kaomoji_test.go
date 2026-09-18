package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"xchat/internal/kaomoji"
	"xchat/internal/protocol"
)

func init() {
	if err := kaomoji.LoadFile("../../config/kaomoji.json"); err != nil {
		panic(err)
	}
}

func kaomojiModel() *Model {
	model := New("ws://localhost/ws")
	model.joined = true
	model.connected = true
	model.input.Focus()
	return model
}

func kaomojiKey(model *Model, kind tea.KeyType) { model.Update(tea.KeyMsg{Type: kind}) }

func TestKaomojiCatalogPassesProtocolValidation(t *testing.T) {
	for _, category := range kaomoji.Categories() {
		for _, item := range category.Items {
			if err := protocol.ValidateBody(item.Text); err != nil {
				t.Fatalf("category %q item %q fails message validation: %v", category.ID, item.Text, err)
			}
		}
	}
}

func TestKaomojiPickerInsertAtCursor(t *testing.T) {
	model := kaomojiModel()
	first := kaomoji.Categories()[0].Items[0].Text
	model.input.SetValue("abc")
	model.input.SetCursor(1)
	kaomojiKey(model, tea.KeyF3)
	if model.picker == nil || model.input.Focused() {
		t.Fatal("picker did not open or input kept focus")
	}
	kaomojiKey(model, tea.KeyEnter)
	want := "a" + first + "bc"
	if model.picker != nil || model.input.Value() != want || !model.input.Focused() {
		t.Fatalf("insert failed: got %q, want %q", model.input.Value(), want)
	}
}

func TestKaomojiPickerCancel(t *testing.T) {
	model := kaomojiModel()
	model.input.SetValue("draft")
	kaomojiKey(model, tea.KeyF3)
	kaomojiKey(model, tea.KeyEsc)
	if model.picker != nil || model.input.Value() != "draft" || !model.input.Focused() {
		t.Fatal("cancel lost draft or focus")
	}
}

func TestKaomojiPickerOnlyInChat(t *testing.T) {
	login := New("ws://localhost/ws")
	kaomojiKey(login, tea.KeyF3)
	if login.picker != nil {
		t.Fatal("picker opened on login")
	}

	model := switchingModel()
	switchKey(model, tea.KeyF2)
	kaomojiKey(model, tea.KeyF3)
	if model.picker != nil {
		t.Fatal("picker opened while room switch dialog is open")
	}
	if model.switcher == nil {
		t.Fatal("F3 disturbed the room switch dialog")
	}
}

func TestKaomojiPickerNavigation(t *testing.T) {
	model := kaomojiModel()
	kaomojiKey(model, tea.KeyF3)
	kaomojiKey(model, tea.KeyDown)
	kaomojiKey(model, tea.KeyRight)
	kaomojiKey(model, tea.KeyDown)
	if model.picker == nil || model.picker.catIndex != 1 || model.picker.itemIndex != 1 || !model.picker.focusItems {
		t.Fatalf("unexpected picker state: %+v", model.picker)
	}
	want := kaomoji.Categories()[1].Items[1].Text
	kaomojiKey(model, tea.KeyEnter)
	if model.input.Value() != want {
		t.Fatalf("inserted wrong item: got %q, want %q", model.input.Value(), want)
	}
}

func TestKaomojiPickerCompactEnterEntersCategory(t *testing.T) {
	model := kaomojiModel()
	model.Update(tea.WindowSizeMsg{Width: 40, Height: 18})
	kaomojiKey(model, tea.KeyF3)
	kaomojiKey(model, tea.KeyEnter)
	if model.picker == nil || !model.picker.focusItems || model.picker.itemIndex != 0 {
		t.Fatal("compact Enter did not enter the selected category")
	}
	want := kaomoji.Categories()[0].Items[0].Text
	kaomojiKey(model, tea.KeyEnter)
	if model.picker != nil || model.input.Value() != want {
		t.Fatalf("compact insert failed: got %q, want %q", model.input.Value(), want)
	}
}

func TestKaomojiPickerCompactEscReturnsToCategories(t *testing.T) {
	model := kaomojiModel()
	model.Update(tea.WindowSizeMsg{Width: 40, Height: 18})
	kaomojiKey(model, tea.KeyF3)
	kaomojiKey(model, tea.KeyEnter)
	if model.picker == nil || !model.picker.focusItems {
		t.Fatal("compact Enter did not enter the category")
	}
	kaomojiKey(model, tea.KeyEsc)
	if model.picker == nil || model.picker.focusItems {
		t.Fatal("first Esc did not return to the category list")
	}
	kaomojiKey(model, tea.KeyEsc)
	if model.picker != nil || !model.input.Focused() {
		t.Fatal("second Esc did not close the picker")
	}
}

func TestKaomojiPickerFitsSmallTerminals(t *testing.T) {
	for _, size := range [][2]int{{24, 10}, {40, 18}, {80, 24}} {
		model := kaomojiModel()
		model.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		kaomojiKey(model, tea.KeyF3)
		lines := strings.Split(model.View(), "\n")
		if len(lines) > size[1] {
			t.Fatalf("picker exceeds height at %v: %d lines", size, len(lines))
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("picker exceeds width at %v: %q", size, line)
			}
		}
	}
}

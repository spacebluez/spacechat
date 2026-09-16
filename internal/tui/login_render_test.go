package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestLoginDoesNotEmitNULOrDuplicateKeyField(t *testing.T) {
	model := New("ws://192.168.33.216:18080/ws")
	model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	for _, stage := range []string{"initial", "nickname", "key-focused", "key-typed"} {
		switch stage {
		case "nickname":
			model.nickname.SetValue("zmz")
		case "key-focused":
			model.Update(tea.KeyMsg{Type: tea.KeyEnter})
		case "key-typed":
			model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("test")})
		}
		view := model.View()
		if strings.ContainsRune(view, 0) {
			t.Errorf("%s: terminal frame contains %d NUL characters", stage, strings.Count(view, "\x00"))
		}
		if strings.Count(view, "输入聊天室密钥") > 1 {
			t.Errorf("%s: duplicated key placeholder", stage)
		}
	}
}
func TestLoginFrameFitsSmallTerminal(t *testing.T) {
	for _, size := range [][2]int{{24, 10}, {40, 18}, {80, 20}, {100, 30}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			model := New("ws://192.168.33.216:18080/ws")
			model.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			model.nickname.SetValue("zmz")
			before := model.View()
			model.Update(tea.KeyMsg{Type: tea.KeyEnter})
			after := model.View()
			for _, frame := range []string{before, after} {
				lines := strings.Split(frame, "\n")
				if len(lines) > size[1] {
					t.Errorf("frame height %d exceeds terminal height %d", len(lines), size[1])
				}
				for _, line := range lines {
					if ansi.StringWidth(line) > size[0] {
						t.Errorf("line width %d exceeds %d", ansi.StringWidth(line), size[0])
					}
				}
			}
			if strings.Count(before, "\n") != strings.Count(after, "\n") {
				t.Error("focus switch changed frame height")
			}
		})
	}
}

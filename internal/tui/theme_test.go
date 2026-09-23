package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/cellbuf"
	"github.com/muesli/termenv"
	"github.com/rivo/uniseg"
	"xchat/internal/protocol"
)

func TestWideAmbiguousCharactersFitChatFrame(t *testing.T) {
	previous := uniseg.EastAsianAmbiguousWidth
	uniseg.EastAsianAmbiguousWidth = 2
	t.Cleanup(func() { uniseg.EastAsianAmbiguousWidth = previous })
	for _, width := range []int{24, 40, 60, 90, 120, 238} {
		model := kaomojiModel()
		model.name, model.state = "小明·测试", "已连接"
		model.users = []string{model.name, "其他成员"}
		model.input.SetValue(strings.Repeat("·", 12) + " 中文草稿")
		body := strings.Repeat("中文·", 30) + "\n末行"
		message := protocol.Message{Nickname: model.name, Body: body, CreatedAt: "2026-09-23T06:32:00Z"}
		model.messages = []protocol.Message{message}
		model.Update(tea.WindowSizeMsg{Width: width, Height: 30})
		lines := strings.Split(model.chatView(), "\n")
		if len(lines) != 30 {
			t.Fatalf("%d-column chat has %d rows", width, len(lines))
		}
		for row, line := range lines {
			if ansi.StringWidth(line) > width-4 {
				t.Fatalf("%d-column chat overflows row %d: %q", width, row, ansi.Strip(line))
			}
		}
		if model.viewport.Width >= 58 {
			var reconstructed strings.Builder
			for _, line := range model.messageLines(message) {
				reconstructed.WriteString(ansi.Cut(ansi.Strip(line), 25, model.viewport.Width))
			}
			if reconstructed.String() != strings.ReplaceAll(body, "\n", "") {
				t.Fatal("wide ambiguous characters were lost during wrapping")
			}
		}
	}
}

func TestChatLayoutKeepsComposerAndSidebarWithinFrame(t *testing.T) {
	for _, size := range [][2]int{{24, 10}, {35, 15}, {60, 20}, {89, 24}, {90, 10}, {100, 30}, {140, 40}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			model := kaomojiModel()
			model.name, model.state = "小明", "已连接"
			for index := 0; index < 30; index++ {
				model.users = append(model.users, fmt.Sprintf("很长的中文成员昵称%02d", index))
			}
			model.input.SetValue("保留多行草稿\n第二行\n第三行")
			model.messages = []protocol.Message{{ID: 1, Nickname: "很长的中文成员昵称", Body: strings.Repeat("中", 80), CreatedAt: "2026-09-23T06:32:00Z", Mentions: []string{"小明"}}}
			model.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			// Check the composition before the screen's final clipping guard.
			lines := strings.Split(ansi.Strip(model.chatView()), "\n")
			if len(lines) != size[1] {
				t.Fatalf("chat height = %d, want %d", len(lines), size[1])
			}
			for row, line := range lines {
				if ansi.StringWidth(line) > size[0]-4 {
					t.Fatalf("row %d overflows: %q", row, line)
				}
			}
			if strings.Contains(model.chatView(), "在线成员") != (size[0] >= 90) {
				t.Fatal("sidebar did not follow available width")
			}
			if size[0] >= 90 && size[1] < 35 && !strings.Contains(model.chatView(), "另有") {
				t.Fatal("hidden members were not counted")
			}
			model.Update(tea.WindowSizeMsg{Width: 24, Height: 10})
			model.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			if model.input.Value() != "保留多行草稿\n第二行\n第三行" {
				t.Fatal("resize changed the draft")
			}
		})
	}
}

func TestMessageColumnsPreserveWrappedChineseAndNewlines(t *testing.T) {
	model := New("wss://localhost/ws")
	model.name = "小明"
	message := protocol.Message{Nickname: "小明", Body: strings.Repeat("中文", 30) + "\n第二行\n\n末行", CreatedAt: "2026-09-23T06:32:00Z"}
	for _, width := range []int{58, 74, 117} {
		model.viewport.Width = width
		lines := model.messageLines(message)
		var body strings.Builder
		for _, line := range lines[:len(lines)-1] {
			plain := ansi.Strip(line)
			if ansi.StringWidth(plain) > width {
				t.Fatalf("message exceeds %d columns", width)
			}
			body.WriteString(ansi.Cut(plain, 25, width))
		}
		if body.String() != strings.ReplaceAll(message.Body, "\n", "") {
			t.Fatalf("wrapped body lost content at width %d: %q", width, body.String())
		}
		if !strings.Contains(ansi.Strip(lines[0]), "小明") || !strings.Contains(ansi.Strip(lines[0]), "中文") {
			t.Fatal("wide view did not put name and body on the same row")
		}
	}
	model.viewport.Width = 30
	lines := model.messageLines(message)
	if strings.Contains(ansi.Strip(lines[0]), "中文") || !strings.Contains(ansi.Strip(lines[1]), "中文") {
		t.Fatal("narrow view did not move the body below the name")
	}
}

func TestDarkThemeCoversAllViews(t *testing.T) {
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })
	model := kaomojiModel()
	model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model.name = "小明"
	model.users = []string{"小明", "同事"}
	model.refresh(false)
	check := func(name string) {
		t.Helper()
		frame := model.View()
		for _, line := range strings.Split(frame, "\n") {
			if ansi.StringWidth(line) != 98 {
				t.Fatalf("%s has an unpainted row: width %d", name, ansi.StringWidth(line))
			}
		}
		buffer := cellbuf.NewBuffer(98, 30)
		cellbuf.SetContent(buffer, frame)
		for row := 0; row < 30; row++ {
			for column := 0; column < 98; column++ {
				cell := buffer.Cell(column, row)
				if cell != nil && cell.Width == 0 {
					continue
				}
				if cell == nil || cell.Style.Bg == nil {
					t.Fatalf("%s has no background at %d,%d", name, column, row)
				}
			}
		}
	}
	check("chat")
	model.openMembers()
	check("members")
	buffer := cellbuf.NewBuffer(98, 30)
	cellbuf.SetContent(buffer, model.View())
	if buffer.Cell(1, 2).Style.Bg == buffer.Cell(0, 2).Style.Bg {
		t.Fatal("selected member has no distinct background")
	}
	model.members = nil
	model.openKaomojiPicker()
	check("kaomoji")
	model.picker = nil
	model.openRoomSwitch()
	model.switcher.key.SetValue("hidden-room-secret")
	check("room switch")
	if strings.Contains(model.View(), "hidden-room-secret") {
		t.Fatal("room key was exposed")
	}
	model.switcher = nil
	model.recaller = &recallPicker{}
	check("recall")
	model.recaller = nil
	model.joined = false
	check("login")
}

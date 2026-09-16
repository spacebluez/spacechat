package tui

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"xchat/internal/protocol"
)

func TestRenderedWidthAndChinese(t *testing.T) {
	for _, width := range []int{24, 35, 80, 100, 140} {
		model := New("ws://localhost:18080/ws")
		model.joined = true
		model.name = "小明"
		model.Update(tea.WindowSizeMsg{Width: width, Height: 26})
		model.Update(event("welcome", "", protocol.Welcome{Users: []string{"小明", "同事"}}))
		model.Update(event("message", "", protocol.Message{ID: 1, Nickname: "小明", Body: strings.Repeat("中文", 80), CreatedAt: "2026-09-16T01:00:00Z"}))
		for _, line := range strings.Split(model.View(), "\n") {
			if ansi.StringWidth(line) > width {
				t.Fatalf("width %d overflow %d: %s", width, ansi.StringWidth(line), line)
			}
		}
	}
}
func TestProgramStartsAndQuits(t *testing.T) {
	model := New("ws://127.0.0.1:18080/ws")
	input, writer := io.Pipe()
	defer writer.Close()
	defer input.Close()
	var output bytes.Buffer
	program := tea.NewProgram(model, tea.WithInput(input), tea.WithOutput(&output), tea.WithoutRenderer(), tea.WithoutSignalHandler())
	done := make(chan error, 1)
	go func() { _, err := program.Run(); done <- err }()
	program.Send(tea.WindowSizeMsg{Width: 100, Height: 26})
	program.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		program.Kill()
		t.Fatal("program did not quit")
	}
}
func TestHistoryPrependKeepsReadingPosition(t *testing.T) {
	model := New("ws://localhost:18080/ws")
	model.joined = true
	var messages []protocol.Message
	for index := int64(101); index <= 200; index++ {
		messages = append(messages, protocol.Message{ID: index, Nickname: "A", Body: fmt.Sprint(index)})
	}
	model.Update(event("history", "initial", protocol.Page{Messages: messages, HasMore: true}))
	model.viewport.GotoTop()
	before := model.viewport.TotalLineCount()
	messages = nil
	for index := int64(1); index <= 100; index++ {
		messages = append(messages, protocol.Message{ID: index, Nickname: "A", Body: fmt.Sprint(index)})
	}
	model.Update(event("history", "older", protocol.Page{Messages: messages}))
	if model.viewport.YOffset != model.viewport.TotalLineCount()-before {
		t.Fatal("reading position moved")
	}
}

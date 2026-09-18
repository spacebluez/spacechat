package tui

import (
	"bytes"
	"context"
	tea "github.com/charmbracelet/bubbletea"
	"io"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"xchat/internal/client"
	"xchat/internal/securestore"
	"xchat/internal/server"
)

type modelProbe struct {
	inspect func(*Model) bool
	result  chan bool
}
type probedModel struct{ model *Model }

func (probe *probedModel) Init() tea.Cmd { return probe.model.Init() }
func (probe *probedModel) View() string  { return probe.model.View() }
func (probe *probedModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if request, ok := message.(modelProbe); ok {
		request.result <- request.inspect(probe.model)
		return probe, nil
	}
	_, command := probe.model.Update(message)
	return probe, command
}
func awaitModel(t *testing.T, program *tea.Program, description string, inspect func(*Model) bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		result := make(chan bool, 1)
		program.Send(modelProbe{inspect: inspect, result: result})
		select {
		case passed := <-result:
			if passed {
				return
			}
		case <-time.After(time.Second):
			t.Fatal("model event loop stalled")
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal(description)
}
func TestRoomSwitchWithRealConnectionsAndNameConflict(t *testing.T) {
	database, err := securestore.Open(filepath.Join(t.TempDir(), "rooms.db"), bytes.Repeat([]byte{4}, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	first, _ := database.Room("first-key")
	second, _ := database.Room("second-key")
	first.Append("old-user", "old-history")
	second.Append("new-user", "new-history")
	service := server.NewRooms(database)
	defer service.Close()
	httpServer := httptest.NewServer(service.Handler())
	defer httpServer.Close()
	address := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws"
	occupant := client.New(address)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	occupantDone := make(chan struct{})
	go func() { occupant.Run(ctx, "Alice", "second-key"); close(occupantDone) }()
	defer func() { cancel(); <-occupantDone }()
	ready := false
	timeout := time.After(5 * time.Second)
	for !ready {
		select {
		case event := <-occupant.Events():
			ready = event.State == "connected"
		case <-timeout:
			t.Fatal("target occupant failed to join")
		}
	}
	model := New(address)
	probe := &probedModel{model: model}
	program := tea.NewProgram(probe, tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithoutRenderer())
	stopped := make(chan error, 1)
	go func() { _, err := program.Run(); stopped <- err }()
	defer func() {
		program.Quit()
		select {
		case err := <-stopped:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("program did not stop")
		}
		model.Close()
	}()
	typeText := func(text string) { program.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)}) }
	press := func(key tea.KeyType) { program.Send(tea.KeyMsg{Type: key}) }
	typeText("Alice")
	press(tea.KeyEnter)
	typeText("first-key")
	press(tea.KeyEnter)
	awaitModel(t, program, "initial room not ready", func(model *Model) bool {
		return model.connected && len(model.messages) == 1 && model.messages[0].Body == "old-history"
	})
	typeText("unsent draft")
	press(tea.KeyF2)
	typeText("second-key")
	press(tea.KeyEnter)
	press(tea.KeyEnter)
	awaitModel(t, program, "name conflict did not preserve old room", func(model *Model) bool {
		return model.switcher != nil && model.switcher.candidate == nil && strings.Contains(model.switcher.notice, "昵称") && model.connected && model.input.Value() == "unsent draft" && model.messages[0].Body == "old-history"
	})
	press(tea.KeyEsc)
	awaitModel(t, program, "cancel failed", func(model *Model) bool { return model.switcher == nil && model.input.Value() == "unsent draft" })
	press(tea.KeyF2)
	typeText("second-key")
	press(tea.KeyTab)
	press(tea.KeyCtrlU)
	typeText("Alice2")
	press(tea.KeyEnter)
	press(tea.KeyEnter)
	awaitModel(t, program, "target not committed", func(model *Model) bool {
		return model.switcher == nil && model.connected && model.name == "Alice2" && model.input.Value() == "" && len(model.messages) == 1 && model.messages[0].Body == "new-history"
	})
	typeText("new-room-message")
	press(tea.KeyEnter)
	awaitModel(t, program, "target send missing", func(model *Model) bool {
		return len(model.pending) == 0 && len(model.messages) == 2 && model.messages[1].Body == "new-room-message"
	})
	latest, _ := first.LatestID()
	page, err := first.Page(0, 0, latest)
	if err != nil || len(page.Messages) != 1 {
		t.Fatal("new message leaked into old room")
	}
	latest, _ = second.LatestID()
	page, err = second.Page(0, 0, latest)
	if err != nil || len(page.Messages) != 2 || page.Messages[1].Nickname != "Alice2" {
		t.Fatal("target room did not persist new nickname")
	}
}

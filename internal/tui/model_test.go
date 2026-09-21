package tui

import (
	"encoding/json"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"xchat/internal/client"
	"xchat/internal/protocol"
)

func event(kind, id string, payload any) networkEvent {
	frame := protocol.Encode(kind, id, payload)
	return networkEvent{event: client.Event{Frame: &frame}}
}
func TestDeduplicateAndUnknownSend(t *testing.T) {
	model := New("ws://localhost:18080/ws")
	model.joined = true
	model.connected = true
	model.pending["send-1"] = "待确认"
	model.Update(networkEvent{event: client.Event{State: "disconnected"}})
	if model.connected || len(model.pending) != 0 || !strings.Contains(model.notice, "结果未知") {
		t.Fatal("pending send not marked unknown")
	}
	message := protocol.Message{ID: 1, Nickname: "小明", Body: "你好", CreatedAt: "2026-09-16T01:00:00Z"}
	model.Update(event("message", "", message))
	model.Update(event("message", "", message))
	if len(model.messages) != 1 {
		t.Fatal("duplicate message")
	}
}
func TestNarrowScreenAndNewInstance(t *testing.T) {
	model := New("ws://localhost:18080/ws")
	model.joined = true
	model.Update(tea.WindowSizeMsg{Width: 35, Height: 15})
	model.Update(event("message", "", protocol.Message{ID: 9, Nickname: "Alice", Body: "中文"}))
	model.Update(event("welcome", "", protocol.Welcome{InstanceID: "new", Resumed: false, Users: []string{"Alice"}}))
	if len(model.messages) != 0 {
		t.Fatal("old instance history retained")
	}
	if strings.Contains(model.View(), "在线成员") {
		t.Fatal("sidebar should be hidden")
	}
	model.Update(networkEvent{event: client.Event{State: "name_taken", Detail: "昵称已占用"}})
	if model.joined {
		t.Fatal("cannot change nickname")
	}
	_, err := json.Marshal(model.messages)
	if err != nil {
		t.Fatal(err)
	}
}

func TestClientInfoIsUsedForInitialAndRoomSwitchConnections(t *testing.T) {
	info := client.Info{Version: "0.4.0", OS: "linux", Arch: "amd64"}
	model := NewWithClientInfo("ws://localhost:18080/ws", info)
	var received []client.Info
	model.clientFactory = func(address string, actual client.Info, peer *client.Client) *client.Client {
		if address != model.address {
			t.Fatalf("address = %q", address)
		}
		received = append(received, actual)
		if peer != nil {
			return peer.NewPeer()
		}
		return client.NewWithInfo(address, actual)
	}
	model.nickname.SetValue("Alice")
	model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model.accessKey.SetValue("first-room")
	model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model.connected = true
	model.Update(tea.KeyMsg{Type: tea.KeyF2})
	model.switcher.key.SetValue("second-room")
	model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if len(received) != 2 || received[0] != info || received[1] != info {
		t.Fatalf("client metadata = %+v", received)
	}
	model.Close()
}

func TestUpgradeRequiredQuitsBeforeHistoryIsShown(t *testing.T) {
	model := NewWithClientInfo("ws://localhost:18080/ws", client.Info{Version: "0.3.2", OS: "linux", Arch: "amd64"})
	model.joined = true
	model.network = client.New(model.address)
	cancelled := false
	model.cancel = func() { cancelled = true }
	_, command := model.Update(networkEvent{source: model.network, event: client.Event{State: "upgrade_required", Detail: "必须升级"}})
	if !model.UpgradeRequired() || !cancelled {
		t.Fatalf("upgradeRequired=%v cancelled=%v", model.UpgradeRequired(), cancelled)
	}
	if command == nil {
		t.Fatal("upgrade-required event did not quit")
	}
	if _, ok := command().(tea.QuitMsg); !ok {
		t.Fatal("upgrade-required command is not tea.Quit")
	}
	if len(model.messages) != 0 || strings.Contains(model.View(), "聊天记录") {
		t.Fatal("history was shown after upgrade requirement")
	}
}

func TestUnsupportedClientReturnsToLogin(t *testing.T) {
	model := New("ws://localhost:18080/ws")
	model.joined = true
	model.connected = false
	model.Update(networkEvent{event: client.Event{State: "unsupported_client", Detail: "当前平台不受支持"}})
	if model.joined || model.notice != "当前平台不受支持" || !model.nickname.Focused() {
		t.Fatalf("unsupported client state was not recovered: joined=%v notice=%q", model.joined, model.notice)
	}
}

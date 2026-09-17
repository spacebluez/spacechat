package tui

import (
	"context"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
	"xchat/internal/client"
	"xchat/internal/protocol"
)

func switchingModel() *Model {
	model := New("ws://localhost/ws")
	model.joined = true
	model.connected = true
	model.name = "Alice"
	model.nickname.SetValue("Alice")
	model.accessKey.SetValue("old-room")
	model.network = client.New(model.address)
	model.input.Focus()
	model.messages = []protocol.Message{{ID: 1, Nickname: "old-user", Body: "old-body"}}
	return model
}
func switchKey(model *Model, kind tea.KeyType) { model.Update(tea.KeyMsg{Type: kind}) }

func TestSwitchModalCancelAndMask(t *testing.T) {
	model := switchingModel()
	model.input.SetValue("draft")
	switchKey(model, tea.KeyF2)
	if model.switcher == nil || model.switcher.nickname.Value() != "Alice" || model.switcher.key.Value() != "" {
		t.Fatal("bad initial modal")
	}
	model.switcher.key.SetValue("new-secret")
	if strings.Contains(model.View(), "new-secret") {
		t.Fatal("room key visible")
	}
	switchKey(model, tea.KeyEsc)
	if model.switcher != nil || model.input.Value() != "draft" || !model.input.Focused() {
		t.Fatal("cancel lost draft/focus")
	}
}
func TestSwitchDraftConfirmationAndSameRoomNoop(t *testing.T) {
	model := switchingModel()
	model.input.SetValue("draft")
	switchKey(model, tea.KeyF2)
	model.switcher.key.SetValue("old-room")
	switchKey(model, tea.KeyEnter)
	if model.switcher == nil || model.switcher.candidate != nil || !strings.Contains(model.switcher.notice, "当前房间") {
		t.Fatal("same room reconnects")
	}
	model.switcher.key.SetValue("new-room")
	switchKey(model, tea.KeyEnter)
	if !model.switcher.confirm || model.switcher.candidate != nil {
		t.Fatal("missing draft confirmation")
	}
	switchKey(model, tea.KeyEnter)
	if model.switcher.candidate == nil || model.input.Value() != "draft" {
		t.Fatal("draft cleared before success")
	}
	model.Close()
}
func TestSwitchFailureAndSuccessIsolateState(t *testing.T) {
	model := switchingModel()
	old := model.network
	oldCancelled := false
	model.cancel = func() { oldCancelled = true }
	model.input.SetValue("draft")
	switchKey(model, tea.KeyF2)
	model.switcher.key.SetValue("new-room")
	switchKey(model, tea.KeyEnter)
	switchKey(model, tea.KeyEnter)
	candidate := model.switcher.candidate
	incoming := protocol.Encode("history", "initial", protocol.Page{Messages: []protocol.Message{{ID: 99, Body: "new-body"}}})
	model.Update(networkEvent{source: candidate.network, event: client.Event{Frame: &incoming}})
	if len(model.messages) != 1 || model.messages[0].Body != "old-body" {
		t.Fatal("candidate leaked before ready")
	}
	model.Update(networkEvent{source: candidate.network, event: client.Event{State: "name_taken", Detail: "昵称占用"}})
	if oldCancelled || model.network != old || model.input.Value() != "draft" || model.switcher.candidate != nil {
		t.Fatal("failed switch lost old session")
	}
	switchKey(model, tea.KeyEnter)
	switchKey(model, tea.KeyEnter)
	candidate = model.switcher.candidate
	model.Update(networkEvent{source: candidate.network, event: client.Event{Frame: &incoming}})
	model.Update(networkEvent{source: candidate.network, event: client.Event{State: "connected"}})
	if !oldCancelled || model.network != candidate.network || model.switcher != nil || model.input.Value() != "" || len(model.messages) != 1 || model.messages[0].Body != "new-body" {
		t.Fatal("bad commit")
	}
	stale := protocol.Encode("history_cleared", "", protocol.Cleared{})
	model.Update(networkEvent{source: old, event: client.Event{Frame: &stale}})
	model.Update(networkEvent{source: old, event: client.Event{State: "unauthorized"}})
	if len(model.messages) != 1 || !model.joined {
		t.Fatal("old events polluted new room")
	}
	model.Close()
}
func TestSwitchWaitsForPendingAndTimeoutPreservesOld(t *testing.T) {
	model := switchingModel()
	model.pending["send-1"] = "message"
	switchKey(model, tea.KeyF2)
	model.switcher.key.SetValue("target")
	switchKey(model, tea.KeyEnter)
	if !model.switcher.waiting || model.switcher.candidate != nil {
		t.Fatal("did not wait")
	}
	ack := protocol.Encode("ack", "send-1", protocol.Message{ID: 2, Body: "message"})
	model.Update(networkEvent{source: model.network, event: client.Event{Frame: &ack}})
	if model.switcher.candidate == nil {
		t.Fatal("did not continue after ack")
	}
	candidate := model.switcher.candidate
	model.Update(roomSwitchTimeout{source: candidate.network})
	if model.switcher.candidate != nil || !model.connected || len(model.messages) != 2 {
		t.Fatal("timeout damaged old room")
	}
}
func TestSwitchUnknownSendRequiresAnotherConfirmation(t *testing.T) {
	model := switchingModel()
	model.pending["send-1"] = "message"
	switchKey(model, tea.KeyF2)
	model.switcher.key.SetValue("target")
	switchKey(model, tea.KeyEnter)
	model.Update(networkEvent{source: model.network, event: client.Event{State: "disconnected"}})
	if model.switcher.candidate != nil || model.switcher.waiting || !strings.Contains(model.switcher.notice, "未知") {
		t.Fatal("unknown result was silently discarded")
	}
}
func TestCloseCancelsBothConnections(t *testing.T) {
	model := switchingModel()
	ctx, cancel := context.WithCancel(context.Background())
	model.cancel = cancel
	switchKey(model, tea.KeyF2)
	model.switcher.key.SetValue("target")
	switchKey(model, tea.KeyEnter)
	targetCancelled := false
	model.switcher.candidate.cancel = func() { targetCancelled = true }
	model.Close()
	if ctx.Err() == nil || !targetCancelled {
		t.Fatal("connections leaked on quit")
	}
}

func TestSwitchCancelledCandidateAndClipboardCannotPollute(t *testing.T) {
	model := switchingModel()
	switchKey(model, tea.KeyF2)
	model.Update(pasteTextMsg{text: "old clipboard", source: model.network})
	if model.switcher.key.Value() != "" {
		t.Fatal("asynchronous draft paste exposed as room key")
	}
	model.switcher.key.SetValue("target")
	switchKey(model, tea.KeyEnter)
	abandoned := model.switcher.candidate.network
	switchKey(model, tea.KeyEsc)
	model.Update(networkEvent{source: abandoned, event: client.Event{State: "connected"}})
	model.Update(pasteTextMsg{source: abandoned, text: "stale draft"})
	if model.switcher != nil || model.accessKey.Value() != "old-room" || model.input.Value() != "" {
		t.Fatal("cancelled candidate or old clipboard replaced active room")
	}
}
func TestSwitchMixedSendResultsRequireConfirmation(t *testing.T) {
	model := switchingModel()
	model.pending["first"] = "first"
	model.pending["second"] = "second"
	switchKey(model, tea.KeyF2)
	model.switcher.key.SetValue("target")
	switchKey(model, tea.KeyEnter)
	failure := protocol.Encode("error", "first", protocol.Failure{Code: "storage_error", Message: "failed"})
	model.Update(networkEvent{source: model.network, event: client.Event{Frame: &failure}})
	ack := protocol.Encode("ack", "second", protocol.Message{ID: 2, Body: "second"})
	model.Update(networkEvent{source: model.network, event: client.Event{Frame: &ack}})
	if model.switcher.candidate != nil || !strings.Contains(model.switcher.notice, "失败") {
		t.Fatal("earlier failed send was hidden by last successful ack")
	}
}

func TestSwitchModalFitsSmallTerminals(t *testing.T) {
	for _, size := range [][2]int{{24, 10}, {40, 18}, {80, 24}} {
		model := switchingModel()
		model.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		switchKey(model, tea.KeyF2)
		model.switcher.key.SetValue("秘密-room-passphrase")
		model.switcher.notice = "成功后丢弃草稿？Enter 继续，Esc 取消"
		lines := strings.Split(model.View(), "\n")
		if len(lines) > size[1] {
			t.Fatalf("modal exceeds height: %v", size)
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("modal exceeds width: %v", size)
			}
		}
		if strings.Contains(model.View(), "秘密") {
			t.Fatal("secret exposed")
		}
	}
}

func TestSwitchClipboardCannotMoveBetweenFieldsOrDialogs(t *testing.T) {
	model := switchingModel()
	switchKey(model, tea.KeyF2)
	dialog := model.switcher
	switchKey(model, tea.KeyTab)
	model.Update(roomSwitchPaste{dialog: dialog, keyField: true, text: "secret"})
	if model.switcher.nickname.Value() != "Alice" || model.switcher.key.Value() != "" {
		t.Fatal("key clipboard crossed fields")
	}
	switchKey(model, tea.KeyEsc)
	switchKey(model, tea.KeyF2)
	model.Update(roomSwitchPaste{dialog: dialog, keyField: true, text: "secret"})
	if model.switcher.key.Value() != "" {
		t.Fatal("clipboard crossed dialogs")
	}
}

func TestOldRoomShortcutIsNotUsed(t *testing.T) {
	model := switchingModel()
	switchKey(model, tea.KeyCtrlR)
	if model.switcher != nil {
		t.Fatal("old Ctrl+R shortcut still opens room switch")
	}
	switchKey(model, tea.KeyF2)
	if model.switcher == nil {
		t.Fatal("F2 does not open room switch")
	}
}

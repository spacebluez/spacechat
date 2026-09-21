package tui

import (
	"context"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"strings"
	"time"
	"xchat/internal/accesskey"
	"xchat/internal/client"
	"xchat/internal/protocol"
)

type roomSwitch struct {
	key           textinput.Model
	nickname      textinput.Model
	notice        string
	confirm       bool
	waiting       bool
	waitingFailed bool
	candidate     *Model
}

type roomSwitchTimeout struct{ source *client.Client }

func (model *Model) openRoomSwitch() tea.Cmd {
	key := textinput.New()
	key.Placeholder = "输入目标房间口令"
	key.CharLimit = 256
	key.EchoMode = textinput.EchoPassword
	key.EchoCharacter = '*'
	key.Prompt = "> "
	nickname := textinput.New()
	nickname.CharLimit = 20
	nickname.Prompt = "> "
	nickname.SetValue(model.name)
	model.switcher = &roomSwitch{key: key, nickname: nickname}
	model.input.Blur()
	model.resize()
	return model.switcher.key.Focus()
}

func (model *Model) cancelRoomSwitch() tea.Cmd {
	if model.switcher != nil && model.switcher.candidate != nil {
		model.switcher.candidate.Close()
	}
	model.switcher = nil
	return model.input.Focus()
}

func (model *Model) failRoomSwitch(notice string) tea.Cmd {
	dialog := model.switcher
	if dialog.candidate != nil {
		dialog.candidate.Close()
	}
	dialog.candidate = nil
	dialog.waiting = false
	dialog.confirm = false
	dialog.notice = notice
	if dialog.nickname.Focused() {
		return dialog.nickname.Focus()
	}
	return dialog.key.Focus()
}

func (model *Model) submitRoomSwitch() tea.Cmd {
	dialog := model.switcher
	name := strings.TrimSpace(dialog.nickname.Value())
	key := dialog.key.Value()
	if err := accesskey.Validate(key); err != nil {
		dialog.notice = err.Error()
		return nil
	}
	if err := protocol.ValidateName(name); err != nil {
		dialog.notice = err.Error()
		return nil
	}
	if key == model.accessKey.Value() && name == model.name {
		dialog.notice = "已在当前房间，无需切换"
		return nil
	}
	if len(model.pending) > 0 {
		dialog.waiting = true
		dialog.waitingFailed = false
		dialog.notice = "等待原房间发送结果，Esc 可取消"
		return nil
	}
	if model.input.Value() != "" && !dialog.confirm {
		dialog.confirm = true
		dialog.notice = "成功后丢弃草稿？Enter 继续，Esc 取消"
		return nil
	}
	dialog.waiting = false
	dialog.confirm = false
	dialog.notice = "正在进入目标房间…"
	target := newWithClientFactory(model.address, model.clientInfo, model.clientFactory)
	target.width, target.height = model.width, model.height
	target.resize()
	target.name = name
	target.nickname.SetValue(name)
	target.nickname.Blur()
	target.accessKey.SetValue(key)
	target.joined = true
	target.state = "连接中"
	target.network = target.clientFactory(model.address, target.clientInfo)
	ctx, cancel := context.WithCancel(context.Background())
	target.cancel = cancel
	dialog.candidate = target
	network := target.network
	return tea.Batch(func() tea.Msg { go network.Run(ctx, name, key); return waitEvent(network)() }, tea.Tick(15*time.Second, func(time.Time) tea.Msg { return roomSwitchTimeout{source: network} }))
}

func (model *Model) updateRoomSwitch(message tea.Msg) tea.Cmd {
	dialog := model.switcher
	if value, ok := message.(tea.KeyMsg); ok {
		switch value.String() {
		case "esc":
			return model.cancelRoomSwitch()
		case "f2":
			return nil
		}
		if dialog.candidate != nil || dialog.waiting {
			return nil
		}
		switch value.String() {
		case "ctrl+v", "shift+insert":
			return readSwitchClipboard(dialog)
		case "enter":
			return model.submitRoomSwitch()
		case "tab", "shift+tab":
			dialog.confirm = false
			if dialog.key.Focused() {
				dialog.key.Blur()
				return dialog.nickname.Focus()
			}
			dialog.nickname.Blur()
			return dialog.key.Focus()
		}
		dialog.confirm = false
	}
	if dialog.candidate != nil || dialog.waiting {
		return nil
	}
	oldKey, oldName := dialog.key.Value(), dialog.nickname.Value()
	var command tea.Cmd
	if dialog.key.Focused() {
		dialog.key, command = dialog.key.Update(message)
	} else {
		dialog.nickname, command = dialog.nickname.Update(message)
	}
	if oldKey != dialog.key.Value() || oldName != dialog.nickname.Value() {
		dialog.confirm = false
		dialog.notice = ""
	}
	return command
}

func (model *Model) candidateEvent(event networkEvent) tea.Cmd {
	target := model.switcher.candidate
	if event.closed {
		return model.failRoomSwitch("目标连接已关闭，请重试；原房间保留")
	}
	switch event.event.State {
	case "unauthorized", "name_taken", "invalid_name", "invalid_address":
		return model.failRoomSwitch(event.event.Detail)
	case "disconnected":
		return model.failRoomSwitch("进入目标房间失败，请重试；原房间保留")
	}
	target.applyEvent(event.event)
	if event.event.State == "connected" {
		if model.cancel != nil {
			model.cancel()
		}
		target.width, target.height = model.width, model.height
		target.resize()
		target.refresh(false)
		target.viewport.GotoBottom()
		*model = *target
		model.notice = "已切换聊天室"
		return tea.Batch(waitEvent(model.network), model.input.Focus())
	}
	return waitEvent(target.network)
}

func (model *Model) continueRoomSwitch(event client.Event) tea.Cmd {
	if model.switcher == nil || !model.switcher.waiting {
		return nil
	}
	if event.Frame != nil && event.Frame.Type == "error" {
		model.switcher.waitingFailed = true
	}
	if len(model.pending) > 0 {
		return nil
	}
	model.switcher.waiting = false
	if event.Frame != nil && event.Frame.Type == "history_cleared" {
		model.switcher.notice = "原记录已清理，发送结果需核对；Enter 继续"
		return nil
	}
	if event.State == "disconnected" {
		model.switcher.notice = "原消息结果未知，不会重发；Enter 继续切换"
		return nil
	}
	if model.switcher.waitingFailed {
		model.switcher.notice = "原消息发送失败；Enter 继续切换，Esc 返回"
		return nil
	}
	return model.submitRoomSwitch()
}

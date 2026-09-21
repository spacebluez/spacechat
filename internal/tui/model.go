package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"xchat/internal/accesskey"
	"xchat/internal/client"
	"xchat/internal/protocol"
)

type networkEvent struct {
	event  client.Event
	source *client.Client
	closed bool
}

type clientFactory func(address string, info client.Info) *client.Client

type Model struct {
	switcher          *roomSwitch
	address           string
	clientInfo        client.Info
	clientFactory     clientFactory
	name              string
	nickname          textinput.Model
	accessKey         textinput.Model
	input             textarea.Model
	viewport          viewport.Model
	network           *client.Client
	cancel            context.CancelFunc
	width, height     int
	joined, connected bool
	state, notice     string
	messages          []protocol.Message
	users             []string
	pending           map[string]string
	issues            []string
	hasMore, loading  bool
	upgradeRequired   bool
	sequence          uint64
}

func New(address string) *Model {
	return NewWithClientInfo(address, client.Info{})
}

func NewWithClientInfo(address string, info client.Info) *Model {
	return newWithClientFactory(address, info, client.NewWithInfo)
}

func newWithClientFactory(address string, info client.Info, factory clientFactory) *Model {
	nickname := textinput.New()
	nickname.Placeholder = "输入昵称（1–20 字符）"
	nickname.CharLimit = 20
	nickname.Prompt = "> "
	nickname.Focus()
	keyInput := textinput.New()
	keyInput.Placeholder = "输入房间口令"
	keyInput.CharLimit = 256
	keyInput.EchoMode = textinput.EchoPassword
	keyInput.EchoCharacter = '*'
	keyInput.Prompt = "> "
	input := textarea.New()
	input.ShowLineNumbers = false
	input.SetHeight(3)
	input.MaxHeight = 2000
	input.KeyMap.Paste = key.NewBinding(key.WithKeys("ctrl+v", "shift+insert"))
	input.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("alt+enter", "ctrl+j"))
	input.Placeholder = "输入消息，Enter 发送，Shift+Enter 换行"
	input.CharLimit = 2000
	input.Prompt = "> "
	model := &Model{address: address, clientInfo: info, clientFactory: factory, nickname: nickname, accessKey: keyInput, input: input, viewport: viewport.New(70, 15), width: 100, height: 26, pending: make(map[string]string), state: "未连接"}
	model.resize()
	return model
}
func (model *Model) Init() tea.Cmd { return textinput.Blink }
func (model *Model) UpgradeRequired() bool {
	return model.upgradeRequired
}
func (model *Model) Close() {
	if model.switcher != nil && model.switcher.candidate != nil {
		model.switcher.candidate.Close()
	}
	if model.cancel != nil {
		model.cancel()
	}
}
func waitEvent(network *client.Client) tea.Cmd {
	if network == nil {
		return nil
	}
	return func() tea.Msg {
		event, open := <-network.Events()
		return networkEvent{event: event, source: network, closed: !open}
	}
}
func (model *Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch value := message.(type) {
	case roomSwitchPaste:
		return model, model.applySwitchPaste(value)
	case pasteTextMsg:
		if !model.joined || model.switcher != nil || (value.source != nil && value.source != model.network) {
			return model, nil
		}
		if value.err != nil {
			model.notice = "无法读取剪贴板"
			return model, nil
		}
		return model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(normalizeNewlines(value.text)), Paste: true})
	case roomSwitchTimeout:
		if model.switcher != nil && model.switcher.candidate != nil && model.switcher.candidate.network == value.source {
			return model, model.failRoomSwitch("进入目标房间超时；原房间和草稿保留")
		}
		return model, nil
	case tea.WindowSizeMsg:
		model.width = value.Width
		model.height = value.Height
		model.resize()
		model.refresh(false)
		return model, nil
	case networkEvent:
		if model.switcher != nil && model.switcher.candidate != nil && value.source == model.switcher.candidate.network {
			if value.event.State == "upgrade_required" {
				model.upgradeRequired = true
				model.Close()
				return model, tea.Quit
			}
			return model, model.candidateEvent(value)
		}
		if value.source != nil && value.source != model.network {
			return model, nil
		}
		if value.closed {
			return model, nil
		}
		if value.event.State == "upgrade_required" {
			model.upgradeRequired = true
			model.Close()
			return model, tea.Quit
		}
		model.applyEvent(value.event)
		return model, tea.Batch(waitEvent(model.network), model.continueRoomSwitch(value.event))
	case tea.KeyMsg:
		if value.Paste {
			value.Runes = []rune(normalizeNewlines(string(value.Runes)))
			message = value
		}
		if value.String() == "ctrl+c" {
			model.Close()
			return model, tea.Quit
		}
		if model.switcher != nil {
			return model, model.updateRoomSwitch(message)
		}
		if model.joined && value.String() == "f2" {
			return model, model.openRoomSwitch()
		}
		if !model.joined {
			if value.String() == "tab" || value.String() == "shift+tab" {
				if model.nickname.Focused() {
					model.nickname.Blur()
					return model, model.accessKey.Focus()
				}
				model.accessKey.Blur()
				return model, model.nickname.Focus()
			}
			if value.String() == "enter" {
				name := strings.TrimSpace(model.nickname.Value())
				if err := protocol.ValidateName(name); err != nil {
					model.notice = err.Error()
					return model, nil
				}
				if model.nickname.Focused() {
					model.nickname.Blur()
					return model, model.accessKey.Focus()
				}
				key := model.accessKey.Value()
				if err := accesskey.Validate(key); err != nil {
					model.notice = err.Error()
					return model, nil
				}
				model.Close()
				model.name = name
				model.joined = true
				model.state = "连接中"
				model.notice = ""
				model.nickname.Blur()
				model.accessKey.Blur()
				model.input.Focus()
				model.network = model.clientFactory(model.address, model.clientInfo)
				ctx, cancel := context.WithCancel(context.Background())
				model.cancel = cancel
				network := model.network
				return model, tea.Batch(textinput.Blink, func() tea.Msg { go network.Run(ctx, name, key); return waitEvent(network)() })
			}
			var command tea.Cmd
			if model.nickname.Focused() {
				model.nickname, command = model.nickname.Update(message)
			} else {
				model.accessKey, command = model.accessKey.Update(message)
			}
			return model, command
		}
		switch value.String() {
		case "ctrl+v", "shift+insert":
			return model, readRoomClipboard(model.network)
		case "enter":
			if !model.connected {
				model.notice = "连接恢复后才能发送，草稿已保留"
				return model, nil
			}
			body := model.input.Value()
			if err := protocol.ValidateBody(body); err != nil {
				model.notice = err.Error()
				return model, nil
			}
			model.sequence++
			requestID := fmt.Sprintf("send-%d", model.sequence)
			if err := model.network.Send(requestID, body); err != nil {
				model.notice = err.Error()
				return model, nil
			}
			model.pending[requestID] = body
			model.input.Reset()
			model.notice = "正在发送…"
			return model, nil
		case "pgup":
			model.viewport.PageUp()
			model.loadOlder()
			return model, nil
		case "pgdown":
			model.viewport.PageDown()
			return model, nil
		case "ctrl+end":
			model.viewport.GotoBottom()
			return model, nil
		case "ctrl+home":
			model.viewport.GotoTop()
			model.loadOlder()
			return model, nil
		}
	case tea.MouseMsg:
		if model.switcher != nil {
			return model, nil
		}
		var command tea.Cmd
		model.viewport, command = model.viewport.Update(message)
		model.loadOlder()
		return model, command
	}
	if model.switcher != nil {
		return model, model.updateRoomSwitch(message)
	}
	var command tea.Cmd
	if model.joined {
		model.input, command = model.input.Update(message)
	} else if model.accessKey.Focused() {
		model.accessKey, command = model.accessKey.Update(message)
	} else {
		model.nickname, command = model.nickname.Update(message)
	}
	return model, command
}
func (model *Model) loadOlder() {
	if !model.viewport.AtTop() || !model.hasMore || model.loading || !model.connected || len(model.messages) == 0 || model.network == nil {
		return
	}
	if err := model.network.History(model.messages[0].ID); err != nil {
		model.notice = err.Error()
		return
	}
	model.loading = true
	model.notice = "正在加载更早的消息…"
}
func (model *Model) applyEvent(event client.Event) {
	if event.State != "" {
		switch event.State {
		case "connected":
			model.connected = true
			model.state = "已连接"
			if len(model.issues) == 0 {
				model.notice = ""
			}
		case "connecting":
			model.connected = false
			model.state = "连接中"
		case "disconnected":
			model.connected = false
			model.loading = false
			model.state = "重连中"
			model.notice = "连接断开，正在重连…"
			if len(model.pending) > 0 {
				model.notice = "部分消息结果未知，不会自动重发；请在重连后核对历史"
				for _, body := range model.pending {
					model.issues = append(model.issues, "结果未知 · "+body)
				}
				clear(model.pending)
				model.refresh(false)
			}
		case "unauthorized":
			model.connected = false
			model.joined = false
			model.notice = event.Detail
			model.accessKey.Reset()
			model.nickname.Blur()
			model.accessKey.Focus()
			model.input.Blur()
		case "name_taken", "invalid_name", "invalid_address":
			model.connected = false
			model.joined = false
			model.notice = event.Detail
			model.nickname.Focus()
			model.accessKey.Blur()
			model.input.Blur()
		}
		return
	}
	if event.Frame == nil {
		return
	}
	frame := *event.Frame
	switch frame.Type {
	case "history_cleared":
		model.messages = nil
		model.issues = nil
		clear(model.pending)
		model.hasMore = false
		model.loading = false
		model.connected = true
		model.state = "已连接"
		model.notice = "聊天记录已由服务端清空，可以继续聊天"
		model.refresh(false)
	case "welcome":
		var welcome protocol.Welcome
		if json.Unmarshal(frame.Payload, &welcome) != nil {
			return
		}
		model.users = welcome.Users
		if !welcome.Resumed {
			model.issues = nil
			clear(model.pending)
			model.messages = nil
			model.hasMore = false
			model.loading = false
		}
		model.refresh(false)
	case "presence":
		var presence protocol.Presence
		if json.Unmarshal(frame.Payload, &presence) == nil {
			model.users = presence.Users
		}
	case "history", "sync":
		var page protocol.Page
		if json.Unmarshal(frame.Payload, &page) != nil {
			return
		}
		prepend := frame.Type == "history" && frame.RequestID != "initial"
		if frame.Type == "history" {
			model.hasMore = page.HasMore
			model.loading = false
			model.notice = ""
		}
		model.merge(page.Messages)
		model.refresh(prepend)
		if frame.RequestID == "initial" && frame.Type == "history" {
			model.viewport.GotoBottom()
		}
	case "message", "ack":
		var received protocol.Message
		if json.Unmarshal(frame.Payload, &received) != nil {
			return
		}
		delete(model.pending, frame.RequestID)
		model.merge([]protocol.Message{received})
		model.refresh(false)
		if frame.RequestID != "" && len(model.pending) == 0 {
			model.notice = ""
		}
	case "error":
		var failure protocol.Failure
		if json.Unmarshal(frame.Payload, &failure) != nil {
			return
		}
		model.notice = failure.Message
		if body, exists := model.pending[frame.RequestID]; exists {
			model.issues = append(model.issues, "发送失败 · "+body+" · "+failure.Message)
			delete(model.pending, frame.RequestID)
			model.refresh(false)
		}
		if frame.RequestID == "older" {
			model.loading = false
		}
	}
}
func (model *Model) merge(incoming []protocol.Message) {
	known := make(map[int64]bool, len(model.messages))
	for _, message := range model.messages {
		known[message.ID] = true
	}
	for _, message := range incoming {
		if !known[message.ID] {
			model.messages = append(model.messages, message)
			known[message.ID] = true
		}
	}
	sort.Slice(model.messages, func(left, right int) bool { return model.messages[left].ID < model.messages[right].ID })
}
func (model *Model) resize() {
	width := model.width - 4
	if model.width >= 90 {
		width -= 24
	}
	model.viewport.Width = max(10, width)
	inputHeight := min(3, max(1, model.height-9))
	model.input.SetHeight(inputHeight)
	model.viewport.Height = max(1, model.height-7-inputHeight)
	model.input.SetWidth(max(5, model.width-4))
	loginWidth := max(1, model.width-4)
	if !model.compactLogin() {
		loginWidth -= 8
	}
	model.nickname.Width = max(1, min(36, loginWidth-3))
	model.accessKey.Width = model.nickname.Width
	if model.switcher != nil {
		model.switcher.key.Width = model.nickname.Width
		model.switcher.nickname.Width = model.nickname.Width
	}
}

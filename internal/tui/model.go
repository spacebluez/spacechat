package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"xchat/internal/accesskey"
	"xchat/internal/client"
	"xchat/internal/kaomoji"
	"xchat/internal/protocol"
)

type networkEvent struct {
	event  client.Event
	source *client.Client
	closed bool
}

type clientFactory func(address string, info client.Info, peer *client.Client) *client.Client

type Model struct {
	switcher          *roomSwitch
	picker            *kaomojiPicker
	catalog           []kaomoji.Category
	catalogLoading    bool
	catalogNotice     string
	kaomojiOverride   bool
	members           *memberPicker
	recaller          *recallPicker
	recalls           map[string]int64
	networkOptions    client.Options
	address           string
	clientInfo        client.Info
	clientFactory     clientFactory
	name              string
	nickname          textinput.Model
	accessKey         textinput.Model
	input             textarea.Model
	viewport          viewport.Model
	network           *client.Client
	ctx               context.Context
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

func New(address string, configuration ...client.Options) *Model {
	return NewWithClientInfo(address, client.Info{}, configuration...)
}

func NewWithClientInfo(address string, info client.Info, configuration ...client.Options) *Model {
	var options client.Options
	if len(configuration) > 0 {
		options = configuration[0]
	}
	factory := func(address string, info client.Info, peer *client.Client) *client.Client {
		if peer != nil {
			return peer.NewPeer()
		}
		return client.NewWithInfo(address, info, options)
	}
	return newWithClientFactory(address, info, factory, options)
}

func newWithClientFactory(address string, info client.Info, factory clientFactory, configuration ...client.Options) *Model {
	nickname := textinput.New()
	nickname.Placeholder = "输入昵称（1-20 字符）"
	nickname.CharLimit = 20
	nickname.Prompt = "> "
	nickname.Focus()
	keyInput := textinput.New()
	keyInput.Placeholder = "输入房间口令"
	keyInput.CharLimit = 256
	keyInput.EchoMode = textinput.EchoPassword
	keyInput.EchoCharacter = '*'
	keyInput.Prompt = "> "
	styleTextInput(&nickname)
	styleTextInput(&keyInput)
	input := textarea.New()
	input.ShowLineNumbers = false
	input.SetHeight(3)
	input.MaxHeight = 2000
	input.KeyMap.Paste = key.NewBinding(key.WithKeys("ctrl+v", "shift+insert"))
	input.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("alt+enter", "ctrl+j"))
	input.Placeholder = "输入消息..."
	input.CharLimit = 2000
	input.Prompt = "> "
	styleComposer(&input)
	model := &Model{address: address, clientInfo: info, clientFactory: factory, nickname: nickname, accessKey: keyInput, input: input, viewport: viewport.New(70, 15), width: 100, height: 26, pending: make(map[string]string), state: "未连接"}
	model.recalls = make(map[string]int64)
	if len(configuration) > 0 {
		model.networkOptions = configuration[0]
	}
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
	case kaomojiResult:
		return model, model.applyKaomoji(value)
	case roomSwitchPaste:
		return model, model.applySwitchPaste(value)
	case pasteTextMsg:
		if !model.joined || model.switcher != nil || model.picker != nil || model.members != nil || model.recaller != nil || (value.source != nil && value.source != model.network) {
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
		var catalogCommand tea.Cmd
		if value.event.State == "connected" {
			catalogCommand = model.refreshKaomoji()
		}
		return model, tea.Batch(waitEvent(model.network), model.continueRoomSwitch(value.event), catalogCommand)
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
		if model.picker != nil {
			return model, model.updateKaomojiPicker(message)
		}
		if model.members != nil {
			return model, model.updateMembers(message)
		}
		if model.recaller != nil {
			return model, model.updateRecall(message)
		}
		if model.joined && value.String() == "f2" {
			return model, model.openRoomSwitch()
		}
		if model.joined && value.String() == "f3" {
			return model, model.openKaomojiPicker()
		}
		if model.joined && value.String() == "f4" {
			return model, model.openMembers()
		}
		if model.joined && value.String() == "f5" {
			return model, model.openRecall()
		}
		if model.joined && !value.Paste && value.Type == tea.KeyRunes && string(value.Runes) == "@" && model.mentionBoundary() {
			if !model.insertDraft("@") {
				return model, nil
			}
			command := model.openMembers()
			model.members.typedAt = true
			return model, command
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
				model.network = model.clientFactory(model.address, model.clientInfo, nil)
				ctx, cancel := context.WithCancel(context.Background())
				model.cancel = cancel
				model.ctx = ctx
				model.catalogLoading = false
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
			body := model.input.Value()
			if model.sendBody(body) {
				model.input.Reset()
			}
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
		if model.switcher != nil || model.picker != nil || model.members != nil || model.recaller != nil {
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
	if model.picker != nil {
		return model, model.updateKaomojiPicker(message)
	}
	if model.members != nil {
		return model, model.updateMembers(message)
	}
	if model.recaller != nil {
		return model, nil
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
func (model *Model) sendBody(body string) bool {
	if !model.connected || model.network == nil {
		model.notice = "连接恢复后才能发送，草稿已保留"
		return false
	}
	if err := protocol.ValidateBody(body); err != nil {
		model.notice = err.Error()
		return false
	}
	model.sequence++
	requestID := fmt.Sprintf("send-%d", model.sequence)
	if err := model.network.Send(requestID, body); err != nil {
		model.notice = err.Error()
		return false
	}
	model.pending[requestID] = body
	model.notice = "正在发送…"
	return true
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
			if len(model.recalls) > 0 {
				model.notice = "撤回结果未知，重连后将核对历史"
				clear(model.recalls)
			}
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
		case "name_taken", "invalid_name", "invalid_address", "tls_error", "unsupported_client":
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
		clear(model.recalls)
		model.recaller = nil
		if model.switcher == nil && model.picker == nil && model.members == nil {
			model.input.Focus()
		}
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
			clear(model.recalls)
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
		if frame.Type == "message" && !received.Recalled && slices.Contains(received.Mentions, model.name) && received.Nickname != model.name {
			model.notice = received.Nickname + " 提到了你"
		}
	case "recalled":
		var recalled protocol.Recalled
		if json.Unmarshal(frame.Payload, &recalled) != nil {
			return
		}
		delete(model.recalls, frame.RequestID)
		for i := range model.messages {
			if model.messages[i].ID == recalled.Message.ID {
				model.messages[i] = recalled.Message
			}
		}
		if frame.RequestID != "" {
			model.notice = "消息已撤回"
		}
		model.refresh(false)
	case "error":
		var failure protocol.Failure
		if json.Unmarshal(frame.Payload, &failure) != nil {
			return
		}
		model.notice = failure.Message
		delete(model.recalls, frame.RequestID)
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
	known := make(map[int64]int, len(model.messages))
	for i, message := range model.messages {
		known[message.ID] = i
	}
	for _, message := range incoming {
		if i, exists := known[message.ID]; exists {
			if !model.messages[i].Recalled {
				model.messages[i] = message
			}
		} else {
			known[message.ID] = len(model.messages)
			model.messages = append(model.messages, message)
		}
	}
	sort.Slice(model.messages, func(left, right int) bool { return model.messages[left].ID < model.messages[right].ID })
}
func (model *Model) resize() {
	width := model.contentWidth()
	if model.width >= 90 {
		width -= 23
	}
	model.viewport.Width = max(1, width)
	inputHeight := min(3, max(1, model.height-9))
	model.input.SetHeight(inputHeight)
	model.viewport.Height = max(1, model.height-5-inputHeight)
	model.input.SetWidth(model.viewport.Width)
	model.nickname.Width = max(1, model.formWidth()-3)
	model.accessKey.Width = model.nickname.Width
	if model.switcher != nil {
		model.switcher.key.Width = model.nickname.Width
		model.switcher.nickname.Width = model.nickname.Width
	}
}

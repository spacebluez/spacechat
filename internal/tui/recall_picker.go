package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"xchat/internal/protocol"
)

type recallPicker struct {
	selected  int
	confirmID int64
	notice    string
}

func (model *Model) recallable() []protocol.Message {
	var messages []protocol.Message
	for i := len(model.messages) - 1; i >= 0; i-- {
		message := model.messages[i]
		created, err := time.Parse(time.RFC3339Nano, message.CreatedAt)
		if message.CanRecall && !message.Recalled && err == nil && time.Since(created) <= protocol.RecallWindow {
			messages = append(messages, message)
		}
	}
	return messages
}

func (model *Model) openRecall() tea.Cmd {
	if len(model.recallable()) == 0 {
		model.notice = "没有两分钟内可撤回的本人消息"
		return nil
	}
	model.recaller = &recallPicker{}
	model.input.Blur()
	return nil
}

func (model *Model) updateRecall(message tea.Msg) tea.Cmd {
	key, ok := message.(tea.KeyMsg)
	if !ok || key.Paste {
		return nil
	}
	picker := model.recaller
	messages := model.recallable()
	picker.selected = min(picker.selected, max(0, len(messages)-1))
	switch key.String() {
	case "esc", "f5":
		model.recaller = nil
		return model.input.Focus()
	case "up":
		picker.selected = max(0, picker.selected-1)
		picker.confirmID = 0
		picker.notice = ""
	case "down":
		picker.selected = min(max(0, len(messages)-1), picker.selected+1)
		picker.confirmID = 0
		picker.notice = ""
	case "enter":
		if len(messages) == 0 {
			picker.notice = "消息已过期或已撤回"
			return nil
		}
		id := messages[picker.selected].ID
		if picker.confirmID == 0 {
			picker.confirmID = id
			picker.notice = "确认撤回这条消息？"
			return nil
		}
		// Confirmation stays bound to the message even if new messages arrive.
		id = picker.confirmID
		found := false
		for _, candidate := range messages {
			if candidate.ID == id {
				found = true
				break
			}
		}
		if !found {
			picker.confirmID = 0
			picker.notice = "消息已过期或已撤回"
			return nil
		}
		if !model.connected || model.network == nil {
			picker.notice = "连接恢复后才能撤回"
			return nil
		}
		model.sequence++
		requestID := fmt.Sprintf("recall-%d", model.sequence)
		if err := model.network.Recall(requestID, id); err != nil {
			picker.notice = err.Error()
			return nil
		}
		model.recalls[requestID] = id
		model.notice = "正在撤回…"
		model.recaller = nil
		return model.input.Focus()
	}
	return nil
}

func (model *Model) recallView() string {
	var labels []string
	for i, message := range model.recallable() {
		labels = append(labels, fmt.Sprintf("#%d  %s", message.ID, strings.ReplaceAll(message.Body, "\n", " ")))
		if message.ID == model.recaller.confirmID {
			model.recaller.selected = i
		}
	}
	return model.selectionView("撤回消息", "", labels, model.recaller.selected, model.recaller.notice)
}

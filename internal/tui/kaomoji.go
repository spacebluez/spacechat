package tui

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"xchat/internal/client"
	"xchat/internal/kaomoji"
)

type kaomojiPicker struct {
	search     textinput.Model
	category   int
	selected   int
	categories []kaomoji.Category
	notice     string
}

type kaomojiResult struct {
	source  *client.Client
	catalog kaomoji.Catalog
	err     error
}

func (model *Model) SetKaomojiOverride(catalog kaomoji.Catalog) {
	model.catalog = catalog.Clone().Categories
	model.kaomojiOverride = true
}

func (model *Model) refreshKaomoji() tea.Cmd {
	if model.kaomojiOverride || model.catalogLoading || !model.connected || model.network == nil {
		return nil
	}
	model.catalogLoading = true
	model.catalogNotice = ""
	network, ctx := model.network, model.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return func() tea.Msg {
		catalog, err := network.FetchKaomoji(ctx)
		return kaomojiResult{source: network, catalog: catalog, err: err}
	}
}

func (model *Model) applyKaomoji(result kaomojiResult) tea.Cmd {
	if result.source != model.network || model.kaomojiOverride {
		return nil
	}
	model.catalogLoading = false
	if result.err != nil {
		model.catalogNotice = "暂时无法获取表情"
		if len(model.catalog) > 0 {
			model.catalogNotice = "表情更新失败，保留已有表情"
		}
		return nil
	}
	model.catalog, model.catalogNotice = result.catalog.Categories, ""
	if model.picker != nil {
		picker := model.picker
		categoryID, selectedText := "", ""
		if picker.category >= 0 {
			categoryID = picker.categories[picker.category].ID
		}
		items := picker.matches()
		if len(items) > 0 {
			selectedText = items[picker.selected].Text
		}
		picker.categories, picker.category, picker.selected = model.catalog, -1, 0
		for index, category := range picker.categories {
			if category.ID == categoryID {
				picker.category = index
				break
			}
		}
		for index, item := range picker.matches() {
			if item.Text == selectedText {
				picker.selected = index
				break
			}
		}
	}
	return nil
}

func (model *Model) openKaomojiPicker() tea.Cmd {
	search := textinput.New()
	search.Placeholder = "搜索颜文字"
	search.CharLimit = 40
	search.Width = max(1, model.width-6)
	styleTextInput(&search)
	model.picker = &kaomojiPicker{search: search, category: -1, categories: model.catalog}
	model.input.Blur()
	return tea.Batch(model.picker.search.Focus(), model.refreshKaomoji())
}

func (picker *kaomojiPicker) matches() []kaomoji.Item {
	query := strings.ToLower(strings.TrimSpace(picker.search.Value()))
	var items []kaomoji.Item
	for index, category := range picker.categories {
		if picker.category >= 0 && picker.category != index {
			continue
		}
		for _, item := range category.Items {
			search := strings.ToLower(category.Name + " " + item.Text + " " + strings.Join(item.Keywords, " "))
			if strings.Contains(search, query) {
				items = append(items, item)
			}
		}
	}
	picker.selected = min(picker.selected, max(0, len(items)-1))
	return items
}

func (model *Model) updateKaomojiPicker(message tea.Msg) tea.Cmd {
	picker := model.picker
	items := picker.matches()
	if key, ok := message.(tea.KeyMsg); ok && !key.Paste {
		switch key.String() {
		case "esc", "f3":
			model.picker = nil
			return model.input.Focus()
		case "up":
			picker.selected = max(0, picker.selected-1)
			return nil
		case "down":
			picker.selected = min(max(0, len(items)-1), picker.selected+1)
			return nil
		case "pgup":
			picker.selected = max(0, picker.selected-max(1, model.height-4))
			return nil
		case "pgdown":
			picker.selected = min(max(0, len(items)-1), picker.selected+max(1, model.height-4))
			return nil
		case "tab", "shift+tab":
			delta := 1
			if key.String() == "shift+tab" {
				delta = -1
			}
			count := len(picker.categories) + 1
			picker.category = (picker.category+1+delta+count)%count - 1
			picker.selected = 0
			return nil
		case "enter", "ctrl+s":
			if len(items) == 0 {
				return nil
			}
			text := items[picker.selected].Text
			accepted := false
			if key.String() == "ctrl+s" {
				accepted = model.sendBody(text)
			} else {
				accepted = model.insertDraft(text)
			}
			if !accepted {
				picker.notice = model.notice
				return nil
			}
			model.picker = nil
			return model.input.Focus()
		}
	}
	previous := picker.search.Value()
	var command tea.Cmd
	picker.search, command = picker.search.Update(message)
	if previous != picker.search.Value() {
		picker.selected = 0
		picker.notice = ""
	}
	return command
}

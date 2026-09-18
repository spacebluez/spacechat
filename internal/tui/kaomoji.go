package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"xchat/internal/kaomoji"
)

// kaomojiPicker is the state for the F3 kaomoji selection overlay. It follows
// the same lightweight state pattern as roomSwitch and is owned by Model.
type kaomojiPicker struct {
	categories []kaomoji.Category
	catIndex   int
	itemIndex  int
	focusItems bool
}

func (picker *kaomojiPicker) clampItem() {
	if picker.catIndex < 0 || picker.catIndex >= len(picker.categories) {
		picker.catIndex = 0
	}
	items := picker.categories[picker.catIndex].Items
	if len(items) == 0 {
		picker.itemIndex = 0
		return
	}
	if picker.itemIndex < 0 {
		picker.itemIndex = 0
	}
	if picker.itemIndex >= len(items) {
		picker.itemIndex = len(items) - 1
	}
}

func (picker *kaomojiPicker) selected() kaomoji.Item {
	picker.clampItem()
	items := picker.categories[picker.catIndex].Items
	if len(items) == 0 {
		return kaomoji.Item{}
	}
	return items[picker.itemIndex]
}

func (model *Model) openKaomojiPicker() tea.Cmd {
	if model.picker != nil {
		return nil
	}
	categories := kaomoji.Categories()
	if len(categories) == 0 {
		return nil
	}
	model.picker = &kaomojiPicker{categories: categories}
	model.input.Blur()
	return nil
}

func (model *Model) closeKaomojiPicker() tea.Cmd {
	model.picker = nil
	return model.input.Focus()
}

func (model *Model) insertSelectedKaomoji() tea.Cmd {
	item := model.picker.selected()
	model.input.InsertString(item.Text)
	return model.closeKaomojiPicker()
}

func (model *Model) updateKaomojiPicker(message tea.Msg) tea.Cmd {
	picker := model.picker
	if picker == nil {
		return nil
	}
	key, ok := message.(tea.KeyMsg)
	if !ok {
		return nil
	}
	switch key.String() {
	case "esc":
		if model.compactPicker() && picker.focusItems {
			picker.focusItems = false
			return nil
		}
		return model.closeKaomojiPicker()
	case "f3":
		return model.closeKaomojiPicker()
	case "enter":
		if model.compactPicker() && !picker.focusItems {
			picker.focusItems = true
			picker.itemIndex = 0
			return nil
		}
		return model.insertSelectedKaomoji()
	case "tab", "shift+tab", "left", "right":
		picker.focusItems = !picker.focusItems
		picker.clampItem()
		return nil
	case "up":
		model.movePickerSelection(false)
		return nil
	case "down":
		model.movePickerSelection(true)
		return nil
	case "pgup":
		model.pagePicker(false)
		return nil
	case "pgdown":
		model.pagePicker(true)
		return nil
	case "home":
		model.pickerHome(false)
		return nil
	case "end":
		model.pickerHome(true)
		return nil
	}
	return nil
}

func (model *Model) movePickerSelection(forward bool) {
	picker := model.picker
	if picker.focusItems {
		items := picker.categories[picker.catIndex].Items
		if len(items) == 0 {
			return
		}
		if forward {
			picker.itemIndex = (picker.itemIndex + 1) % len(items)
		} else {
			picker.itemIndex = (picker.itemIndex - 1 + len(items)) % len(items)
		}
		return
	}
	if len(picker.categories) == 0 {
		return
	}
	if forward {
		picker.catIndex = (picker.catIndex + 1) % len(picker.categories)
	} else {
		picker.catIndex = (picker.catIndex - 1 + len(picker.categories)) % len(picker.categories)
	}
	picker.itemIndex = 0
}

func (model *Model) pagePicker(forward bool) {
	picker := model.picker
	if !picker.focusItems {
		return
	}
	items := picker.categories[picker.catIndex].Items
	if len(items) == 0 {
		return
	}
	page := max(1, model.height-6)
	if forward {
		picker.itemIndex = min(len(items)-1, picker.itemIndex+page)
	} else {
		picker.itemIndex = max(0, picker.itemIndex-page)
	}
}

func (model *Model) pickerHome(end bool) {
	picker := model.picker
	if picker.focusItems {
		items := picker.categories[picker.catIndex].Items
		if len(items) == 0 {
			picker.itemIndex = 0
			return
		}
		if end {
			picker.itemIndex = len(items) - 1
		} else {
			picker.itemIndex = 0
		}
		return
	}
	if end {
		picker.catIndex = len(picker.categories) - 1
	} else {
		picker.catIndex = 0
	}
	picker.itemIndex = 0
}

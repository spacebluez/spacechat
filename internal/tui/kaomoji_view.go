package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

func (model *Model) compactPicker() bool { return model.width < 60 }

func (model *Model) kaomojiView() string {
	picker := model.picker
	if picker == nil {
		return ""
	}
	picker.clampItem()
	contentWidth := max(1, model.width-2)
	bodyHeight := max(1, model.height-3)

	header := accent.Render("选择颜文字")
	if model.compactPicker() && picker.focusItems {
		header += " · " + ansi.Truncate(picker.categories[picker.catIndex].Name, 12, "…")
	}

	preview := "预览：" + picker.selected().Text
	footer := muted.Render("↑↓ 选择 · ←→/Tab 切换 · Enter 插入 · Esc 取消")
	if model.compactPicker() {
		if picker.focusItems {
			preview = "预览：" + picker.selected().Text
			footer = muted.Render("↑↓ 选择 · Enter 插入 · Esc 返回 · ←→ 切换")
		} else {
			preview = muted.Render("Enter 进入分类 · Esc 取消")
			footer = muted.Render("↑↓ 选择分类 · Enter 进入 · Esc 取消")
		}
	}

	var body string
	if model.compactPicker() {
		body = model.compactKaomojiBody(contentWidth, bodyHeight)
	} else {
		body = model.wideKaomojiBody(contentWidth, bodyHeight)
	}

	lines := []string{
		ansi.Truncate(header, contentWidth, "…"),
		body,
		ansi.Truncate(preview, contentWidth, "…"),
		ansi.Truncate(footer, contentWidth, "…"),
	}
	return strings.Join(lines, "\n")
}

func (model *Model) compactKaomojiBody(contentWidth, bodyHeight int) string {
	picker := model.picker
	var labels []string
	selected := 0
	if picker.focusItems {
		items := picker.categories[picker.catIndex].Items
		labels = make([]string, len(items))
		for i, item := range items {
			labels[i] = ansi.Truncate(item.Text, contentWidth-3, "…")
		}
		selected = picker.itemIndex
	} else {
		labels = make([]string, len(picker.categories))
		for i, category := range picker.categories {
			labels[i] = ansi.Truncate(category.Name, contentWidth-3, "…")
		}
		selected = picker.catIndex
	}
	return strings.Join(listWindow(labels, selected, bodyHeight), "\n")
}

func (model *Model) wideKaomojiBody(contentWidth, bodyHeight int) string {
	picker := model.picker
	leftWidth := min(10, max(6, contentWidth/5))
	rightWidth := max(1, contentWidth-leftWidth-1)

	categoryNames := make([]string, len(picker.categories))
	for i, category := range picker.categories {
		categoryNames[i] = ansi.Truncate(category.Name, leftWidth-3, "…")
	}
	items := picker.categories[picker.catIndex].Items
	itemTexts := make([]string, len(items))
	for i, item := range items {
		itemTexts[i] = ansi.Truncate(item.Text, rightWidth-3, "…")
	}

	leftLines := listWindow(categoryNames, picker.catIndex, bodyHeight)
	rightLines := listWindow(itemTexts, picker.itemIndex, bodyHeight)
	return strings.Join(joinColumns(leftLines, rightLines, leftWidth, rightWidth), "\n")
}

func listWindow(items []string, selected, height int) []string {
	if height <= 0 {
		height = 1
	}
	start := 0
	if len(items) > height {
		start = selected - height/2
		if start < 0 {
			start = 0
		}
		if start > len(items)-height {
			start = len(items) - height
		}
	}
	end := min(len(items), start+height)
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		marker := "  "
		if i == selected {
			marker = "▸ "
		}
		lines = append(lines, marker+items[i])
	}
	return lines
}

func joinColumns(left, right []string, leftWidth, rightWidth int) []string {
	height := max(len(left), len(right))
	lines := make([]string, 0, height)
	for i := 0; i < height; i++ {
		var leftLine, rightLine string
		if i < len(left) {
			leftLine = left[i]
		}
		if i < len(right) {
			rightLine = right[i]
		}
		leftLine = ansi.Truncate(leftLine, leftWidth, "")
		rightLine = ansi.Truncate(rightLine, rightWidth, "")
		leftLine += strings.Repeat(" ", max(0, leftWidth-ansi.StringWidth(leftLine)))
		rightLine += strings.Repeat(" ", max(0, rightWidth-ansi.StringWidth(rightLine)))
		lines = append(lines, leftLine+" "+rightLine)
	}
	return lines
}

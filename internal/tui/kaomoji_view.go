package tui

func (model *Model) kaomojiView() string {
	picker := model.picker
	picker.search.Width = max(1, model.width-6)
	category := "全部"
	if picker.category >= 0 {
		category = picker.categories[picker.category].Name
	}
	var labels []string
	for _, item := range picker.matches() {
		labels = append(labels, item.Text)
	}
	detail := picker.notice
	if detail == "" {
		detail = model.catalogNotice
		if model.catalogLoading {
			detail = "正在更新表情…"
		}
	}
	return model.selectionView("颜文字 · "+category, picker.search.View(), labels, picker.selected, detail)
}

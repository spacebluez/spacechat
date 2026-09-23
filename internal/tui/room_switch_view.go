package tui

func (model *Model) roomSwitchView() string {
	dialog := model.switcher
	lines := []string{
		accent.Render("切换聊天室"),
		fieldLabel("房间口令", dialog.key),
		renderTextInput(dialog.key),
		"",
		fieldLabel("昵称", dialog.nickname),
		renderTextInput(dialog.nickname),
		warning.Render(dialog.notice),
		muted.Render("Enter 确认 · Esc 取消"),
		muted.Render("Tab 切换 · Ctrl+C 退出"),
		muted.Render("同口令进同房间，新口令自动建房"),
	}
	if !model.compactLogin() {
		lines = append(lines[:1], append([]string{"", rule(model.formWidth()), ""}, lines[1:]...)...)
	}
	return model.formView(lines)
}

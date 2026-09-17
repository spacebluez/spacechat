package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"strings"
)

func (model *Model) roomSwitchView() string {
	dialog := model.switcher
	width := max(1, model.width-4)
	if !model.compactLogin() {
		width -= 8
	}
	fit := func(text string) string { return ansi.Truncate(text, width, "…") }
	lines := []string{
		fit(accent.Render("切换聊天室")),
		"房间口令",
		fit(renderTextInput(dialog.key)),
		"",
		"昵称",
		fit(renderTextInput(dialog.nickname)),
		fit(warning.Render(dialog.notice)),
		fit(muted.Render("Enter 确认 · Esc 取消")),
		fit(muted.Render("Tab 切换 · Ctrl+C 退出")),
		fit(muted.Render("同口令进同房间，新口令自动建房")),
	}
	content := strings.Join(lines, "\n")
	if model.compactLogin() {
		return content
	}
	return lipgloss.NewStyle().Padding(1, 1).Render(panel.Width(model.width-8).Padding(1, 1).Render(content))
}

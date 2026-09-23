package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/x/ansi"
)

func renderTextInput(field textinput.Model) string {
	if field.Value() != "" || field.Placeholder == "" {
		return field.View()
	}
	placeholder := field.Placeholder
	if field.Width > 0 {
		placeholder = ansi.Truncate(placeholder, field.Width+1, "")
	}
	characters := []rune(placeholder)
	if len(characters) == 0 {
		field.Placeholder = ""
		return field.View()
	}
	field.Cursor.TextStyle = field.PlaceholderStyle
	field.Cursor.SetChar(string(characters[0]))
	padding := 0
	if field.Width > 0 {
		padding = max(0, field.Width+1-ansi.StringWidth(placeholder))
	}
	suffix := string(characters[1:]) + strings.Repeat(" ", padding)
	return field.PromptStyle.Render(field.Prompt) + field.Cursor.View() + field.PlaceholderStyle.Inline(true).Render(suffix)
}
func (model *Model) compactLogin() bool { return model.width < 60 || model.height < 20 }
func (model *Model) loginView() string {
	lines := []string{
		accent.Render("SpaceChat / 进入房间"),
		muted.Render(model.address),
		fieldLabel("昵称", model.nickname),
		renderTextInput(model.nickname),
		"",
		fieldLabel("房间口令", model.accessKey),
		renderTextInput(model.accessKey),
		warning.Render(model.notice),
		muted.Render("Tab 切换 · Enter 进入 · Ctrl+C 退出"),
		muted.Render("同口令进入同房间；昵称不是身份凭证"),
	}
	if !model.compactLogin() {
		lines = append(lines[:2], append([]string{"", rule(model.formWidth()), ""}, lines[2:]...)...)
	}
	return model.formView(lines)
}

package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
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
	width := model.width
	if !model.compactLogin() {
		width -= 8
	}
	fit := func(text string) string { return ansi.Truncate(text, width, "…") }
	lines := []string{
		fit(accent.Render("XCHAT / 内网聊天室")),
		fit(muted.Render(model.address)),
		"昵称",
		fit(renderTextInput(model.nickname)),
		"密钥",
		fit(renderTextInput(model.accessKey)),
		fit(warning.Render(model.notice)),
		fit(muted.Render("Tab 切换 · Enter 继续/进入 · Ctrl+C 退出")),
		fit(muted.Render("需要共享密钥；昵称不是身份凭证")),
	}
	content := strings.Join(lines, "\n")
	if model.compactLogin() {
		return content
	}
	return lipgloss.NewStyle().Padding(1, 1).Render(panel.Width(model.width-4).Padding(1, 1).Render(content))
}

package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var accent = lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true)
var muted = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
var warning = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
var panel = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("238"))

func (model *Model) refresh(prepend bool) {
	atBottom := model.viewport.AtBottom()
	previousLines := model.viewport.TotalLineCount()
	previousOffset := model.viewport.YOffset
	var lines []string
	if model.hasMore {
		lines = append(lines, muted.Render("↑ PgUp 加载更早消息"))
	}
	if len(model.messages) == 0 {
		lines = append(lines, muted.Render("还没有消息，来打个招呼吧。"))
	}
	for _, message := range model.messages {
		timestamp := message.CreatedAt
		if parsed, err := time.Parse(time.RFC3339Nano, timestamp); err == nil {
			timestamp = parsed.Local().Format("01-02 15:04:05")
		}
		nameStyle := accent
		if message.Nickname == model.name {
			nameStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("121")).Bold(true)
		}
		heading := muted.Render(timestamp) + "  " + nameStyle.Render(message.Nickname)
		lines = append(lines, ansi.Hardwrap(heading, model.viewport.Width, true), ansi.Hardwrap(message.Body, model.viewport.Width, true), "")
	}
	for _, issue := range model.issues {
		lines = append(lines, warning.Render(ansi.Hardwrap(issue, model.viewport.Width, true)))
	}
	model.viewport.SetContent(strings.Join(lines, "\n"))
	if prepend {
		model.viewport.SetYOffset(previousOffset + model.viewport.TotalLineCount() - previousLines)
	} else if atBottom {
		model.viewport.GotoBottom()
	} else {
		model.viewport.SetYOffset(previousOffset)
	}
}
func (model *Model) View() string {
	if model.width < 24 || model.height < 10 {
		return "请扩大终端窗口（至少 24×10）\nCtrl+C 退出"
	}
	width := max(20, model.width-4)
	if !model.joined {
		return model.loginView()
	}
	header := accent.Render(" XCHAT ") + muted.Render("口令房间") + "  " + model.state + fmt.Sprintf(" · %d 人", len(model.users))
	if len(model.pending) > 0 {
		header += fmt.Sprintf(" · 待确认 %d", len(model.pending))
	}
	body := panel.Width(model.viewport.Width).Height(model.viewport.Height).Render(model.viewport.View())
	if model.width >= 90 {
		names := []string{accent.Render("在线成员"), ""}
		maximum := max(1, model.viewport.Height-3)
		for index, name := range model.users {
			if index >= maximum {
				names = append(names, muted.Render(fmt.Sprintf("另有 %d 人", len(model.users)-index)))
				break
			}
			if name == model.name {
				name += " (你)"
			}
			names = append(names, ansi.Truncate(name, 20, "…"))
		}
		sidebar := panel.Width(20).Height(model.viewport.Height).Padding(0, 1).Render(strings.Join(names, "\n"))
		body = lipgloss.JoinHorizontal(lipgloss.Top, body, sidebar)
	}
	footer := muted.Render("Enter 发送 · Shift+Enter 换行 · PgUp/PgDn 翻页 · Ctrl+End 最新 · Ctrl+C 退出")
	notice := warning.Render(ansi.Truncate(model.notice, width, "…"))
	return strings.Join([]string{ansi.Truncate(header, model.width, "…"), body, model.input.View(), notice, ansi.Truncate(footer, model.width, "…")}, "\n")
}

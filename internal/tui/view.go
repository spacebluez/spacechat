package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"xchat/internal/protocol"
)

func (model *Model) messageLines(message protocol.Message) []string {
	width := model.viewport.Width
	stamp := "--:--"
	if parsed, err := time.Parse(time.RFC3339Nano, message.CreatedAt); err == nil {
		stamp = parsed.Local().Format("15:04")
	}
	nameStyle := accent
	if message.Nickname == model.name {
		nameStyle = self
	}
	body := strings.ReplaceAll(message.Body, "\t", "    ")
	bodyStyle := surface
	if message.Recalled {
		body, bodyStyle = "消息已撤回", muted
	} else if slices.Contains(message.Mentions, model.name) {
		body = warning.Render("@你") + " " + body
	}
	if width < 58 {
		heading := muted.Render(stamp) + "  " + nameStyle.Render(message.Nickname)
		return []string{ansi.Hardwrap(heading, width, true), bodyStyle.Render(ansi.Hardwrap(body, width, true)), ""}
	}
	const nameWidth = 16
	name := ansi.Truncate(message.Nickname, nameWidth, "...")
	prefix := muted.Render(stamp) + "  " + nameStyle.Render(name+strings.Repeat(" ", nameWidth-ansi.StringWidth(name))) + "  "
	indent := ansi.StringWidth(prefix)
	wrapped := strings.Split(ansi.Hardwrap(body, width-indent, true), "\n")
	for index, line := range wrapped {
		if index == 0 {
			wrapped[index] = prefix + bodyStyle.Render(line)
		} else {
			wrapped[index] = strings.Repeat(" ", indent) + bodyStyle.Render(line)
		}
	}
	return append(wrapped, "")
}

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
	previousDay := ""
	for _, message := range model.messages {
		if parsed, err := time.Parse(time.RFC3339Nano, message.CreatedAt); err == nil {
			day := parsed.Local().Format("2006-01-02")
			if day != previousDay {
				lines = append(lines, muted.Render(day)+"  "+rule(model.viewport.Width-12))
				previousDay = day
			}
		}
		lines = append(lines, model.messageLines(message)...)
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
func (model *Model) chatHeader() string {
	width := model.contentWidth()
	brand := accent.Render("SpaceChat")
	status := warning.Render(model.state)
	if model.connected {
		status = self.Render(model.state)
	}
	transport := "明文"
	if strings.HasPrefix(model.address, "wss://") {
		transport = "TLS"
	}
	if width < 40 {
		return alignedLine(brand, status, width)
	}
	status += muted.Render(fmt.Sprintf(" | %s | %d 人", transport, len(model.users)))
	if width >= 65 {
		brand += muted.Render(" / 口令房间")
	}
	return alignedLine(brand, status, width)
}

func (model *Model) memberSidebar(height int) string {
	const width = 20
	lines := []string{alignedLine(muted.Render("在线成员"), muted.Render(fmt.Sprintf("%02d", len(model.users))), width), ""}
	available := max(0, height-len(lines))
	count := min(available, len(model.users))
	if count < len(model.users) && count > 0 {
		count--
	}
	for _, name := range model.users[:count] {
		style := surface
		if name == model.name {
			name += " (你)"
			style = self
		}
		lines = append(lines, accent.Render("* ")+style.Render(ansi.Truncate(name, width-2, "...")))
	}
	if count < len(model.users) && available > 0 {
		lines = append(lines, muted.Render(fmt.Sprintf("另有 %d 人", len(model.users)-count)))
	}
	return surface.Width(width).Height(height).Render(strings.Join(lines, "\n"))
}

func (model *Model) chatView() string {
	width := model.viewport.Width
	status := self.Render(ansi.Truncate(model.name, 20, "..."))
	if !model.viewport.AtBottom() {
		status = muted.Render("正在浏览历史")
	}
	if len(model.pending) > 0 {
		status = warning.Render(fmt.Sprintf("待确认 %d", len(model.pending)))
	}
	if model.notice != "" {
		status = warning.Render(model.notice)
	}
	counter := muted.Render(fmt.Sprintf("%d / 2000", utf8.RuneCountInString(model.input.Value())))
	main := strings.Join([]string{model.viewport.View(), rule(width), model.input.View(), alignedLine(status, counter, width)}, "\n")
	if model.width >= 90 {
		height := model.viewport.Height + model.input.Height() + 2
		separator := divider.Render(strings.TrimSuffix(strings.Repeat(" | \n", height), "\n"))
		main = lipgloss.JoinHorizontal(lipgloss.Top, main, separator, model.memberSidebar(height))
	}
	footer := muted.Render("F2 换房 | F3 颜文字 | F4 @成员 | F5 撤回")
	footer = alignedLine(footer, muted.Render("Enter 发送 | Shift+Enter 换行"), model.contentWidth())
	return strings.Join([]string{model.chatHeader(), rule(model.contentWidth()), main, footer}, "\n")
}

func (model *Model) View() string {
	if model.width < 24 || model.height < 10 {
		return "请扩大终端窗口（至少 24×10）\nCtrl+C 退出"
	}
	var content string
	switch {
	case model.switcher != nil:
		content = model.roomSwitchView()
	case model.picker != nil:
		content = model.kaomojiView()
	case model.members != nil:
		content = model.membersView()
	case model.recaller != nil:
		content = model.recallView()
	case !model.joined:
		content = model.loginView()
	default:
		content = model.chatView()
	}
	return model.screen(content)
}

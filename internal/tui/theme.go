package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	background = lipgloss.Color("#1C1E21")
	foreground = lipgloss.Color("#E3E6E9")
	surface    = lipgloss.NewStyle().Foreground(foreground).Background(background)
	accent     = surface.Foreground(lipgloss.Color("#7ED9D1")).Bold(true)
	self       = surface.Foreground(lipgloss.Color("#B0CE91"))
	muted      = surface.Foreground(lipgloss.Color("#A0A6AE"))
	warning    = surface.Foreground(lipgloss.Color("#F1C982"))
	divider    = surface.Foreground(lipgloss.Color("#393D42"))
	selected   = surface.Background(lipgloss.Color("#283B3B")).Foreground(lipgloss.Color("#7ED9D1"))
)

func styleTextInput(field *textinput.Model) {
	field.TextStyle = surface
	field.PromptStyle = accent
	field.PlaceholderStyle = muted
	field.Cursor.Style = accent
}

func styleComposer(field *textarea.Model) {
	style := textarea.Style{
		Base: surface, CursorLine: surface, EndOfBuffer: muted,
		Placeholder: muted, Prompt: accent, Text: surface,
	}
	field.FocusedStyle, field.BlurredStyle = style, style
	field.Cursor.Style = accent
}

func (model *Model) contentWidth() int { return max(1, model.width-4) }

// Leave a small right margin around the painted frame.
func (model *Model) screen(content string) string {
	width := model.contentWidth()
	const reset = "\x1b[0m"
	base := strings.TrimSuffix(surface.Render(""), reset)
	restore := strings.NewReplacer(reset, reset+base, ansi.ResetStyle, ansi.ResetStyle+base)
	lines := strings.Split(content, "\n")
	for len(lines) < model.height {
		lines = append(lines, "")
	}
	lines = lines[:min(len(lines), model.height)]
	for index, line := range lines {
		line = ansi.Truncate(line, width, "...")
		painted := surface.Render(" " + line + strings.Repeat(" ", max(0, width-ansi.StringWidth(line))) + " ")
		// Nested Lip Gloss styles reset colors; restore the frame color in gaps.
		lines[index] = painted
		if base != "" {
			lines[index] = restore.Replace(painted) + reset
		}
	}
	return strings.Join(lines, "\n")
}

func rule(width int) string {
	glyph := "─"
	if ansi.StringWidth(glyph) != 1 {
		glyph = "-"
	}
	return divider.Render(strings.Repeat(glyph, max(0, width)))
}

func alignedLine(left, right string, width int) string {
	if ansi.StringWidth(left)+ansi.StringWidth(right)+2 > width {
		return ansi.Truncate(left, width, "...")
	}
	return left + strings.Repeat(" ", width-ansi.StringWidth(left)-ansi.StringWidth(right)) + right
}

func (model *Model) formWidth() int { return min(48, model.contentWidth()) }

func (model *Model) formView(lines []string) string {
	width := model.formWidth()
	left, top := 0, 0
	if !model.compactLogin() {
		left = (model.contentWidth() - width) / 2
		top = max(0, (model.height-len(lines))/2)
	}
	for index, line := range lines {
		lines[index] = strings.Repeat(" ", left) + ansi.Truncate(line, width, "…")
	}
	return strings.Repeat("\n", top) + strings.Join(lines, "\n")
}

func fieldLabel(label string, field textinput.Model) string {
	if field.Focused() {
		return accent.Render(label)
	}
	return muted.Render(label)
}

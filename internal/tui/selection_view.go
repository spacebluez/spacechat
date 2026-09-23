package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

func (model *Model) selectionView(title, search string, labels []string, selection int, detail string) string {
	width, rows := model.contentWidth(), max(1, model.height-4)
	selection = min(selection, max(0, len(labels)-1))
	start := max(0, min(selection-rows/2, len(labels)-rows))
	if search == "" {
		search = rule(width)
	}
	lines := []string{alignedLine(accent.Render(title), muted.Render("SpaceChat"), width), search}
	for row := 0; row < rows; row++ {
		index, line := start+row, ""
		if index < len(labels) {
			line = "  " + labels[index]
			if index == selection {
				line = selected.Width(width).Render(ansi.Truncate("> "+labels[index], width, "…"))
			}
		} else if row == 0 {
			line = muted.Render("没有匹配项")
		}
		lines = append(lines, line)
	}
	lines = append(lines, warning.Render(detail), alignedLine(muted.Render(fmt.Sprintf("%d / %d", min(selection+1, len(labels)), len(labels))), muted.Render("Enter 确认 · Esc 取消"), width))
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], width, "…")
	}
	return strings.Join(lines, "\n")
}

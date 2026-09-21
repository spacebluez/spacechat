package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

func (model *Model) selectionView(title, search string, labels []string, selected int, detail string) string {
	width, rows := max(1, model.width-2), max(1, model.height-4)
	selected = min(selected, max(0, len(labels)-1))
	start := max(0, min(selected-rows/2, len(labels)-rows))
	lines := []string{accent.Render(title), search}
	for row := 0; row < rows; row++ {
		index, line := start+row, ""
		if index < len(labels) {
			line = "  " + labels[index]
			if index == selected {
				line = accent.Render("> " + labels[index])
			}
		} else if row == 0 {
			line = muted.Render("没有匹配项")
		}
		lines = append(lines, line)
	}
	lines = append(lines, warning.Render(detail), muted.Render(fmt.Sprintf("%d / %d", min(selected+1, len(labels)), len(labels))))
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], width, "…")
	}
	return strings.Join(lines, "\n")
}

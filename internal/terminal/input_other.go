//go:build !windows

package terminal

import tea "github.com/charmbracelet/bubbletea"

func Run(model tea.Model) error {
	_, err := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
	return err
}

package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"xchat/internal/client"
	"xchat/internal/tui"
)

var defaultServer = "ws://127.0.0.1:18080/ws"
var version = "dev"

func main() {
	address := flag.String("server", defaultServer, "WebSocket server address")
	showVersion := flag.Bool("version", false, "Show version")
	flag.Parse()
	if *showVersion {
		fmt.Println("xchat", version)
		return
	}
	if err := client.ValidateAddress(*address); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	model := tui.New(*address)
	defer model.Close()
	program := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := program.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "xchat:", err)
		os.Exit(1)
	}
}

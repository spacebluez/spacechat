package main

import (
	"flag"
	"fmt"
	"os"

	"xchat/internal/client"
	"xchat/internal/terminal"
	"xchat/internal/tui"
)

var defaultServer = "ws://127.0.0.1:18081/ws"
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
	if err := terminal.Run(model); err != nil {
		fmt.Fprintln(os.Stderr, "xchat:", err)
		os.Exit(1)
	}
}

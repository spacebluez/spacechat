package main

import (
	"crypto/x509"
	"encoding/base64"
	"flag"
	"fmt"
	"os"

	"xchat/internal/client"
	"xchat/internal/kaomoji"
	"xchat/internal/terminal"
	"xchat/internal/tui"
)

var defaultServer = "wss://127.0.0.1:18081/ws"
var defaultTLSCA string
var version = "dev"

func trustedRoots(path, embedded string) (*x509.CertPool, error) {
	if path != "" {
		return client.LoadRootCAs(path)
	}
	if embedded == "" {
		return nil, nil
	}
	data, err := base64.StdEncoding.DecodeString(embedded)
	if err != nil {
		return nil, fmt.Errorf("invalid built-in TLS CA: %w", err)
	}
	return client.RootCAsFromPEM(data)
}

func main() {
	address := flag.String("server", defaultServer, "WebSocket server address")
	kaomojiPath := flag.String("kaomoji", "", "Optional custom kaomoji catalog JSON file")
	caPath := flag.String("tls-ca", "", "Additional trusted CA certificate PEM file")
	allowInsecure := flag.Bool("allow-insecure", false, "Allow plaintext remote WebSocket connections for debugging")
	showVersion := flag.Bool("version", false, "Show version")
	flag.Parse()
	if *showVersion {
		fmt.Println("xchat", version)
		return
	}
	if err := client.ValidateTransport(*address, *allowInsecure); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	options := client.Options{AllowInsecure: *allowInsecure}
	pool, err := trustedRoots(*caPath, defaultTLSCA)
	if err != nil {
		fmt.Fprintln(os.Stderr, "xchat:", err)
		os.Exit(2)
	}
	options.RootCAs = pool
	model := tui.New(*address, options)
	if *kaomojiPath != "" {
		catalog, err := kaomoji.ReadFile(*kaomojiPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "xchat:", err)
			os.Exit(2)
		}
		model.SetKaomojiOverride(catalog)
	}
	defer model.Close()
	if err := terminal.Run(model); err != nil {
		fmt.Fprintln(os.Stderr, "xchat:", err)
		os.Exit(1)
	}
}

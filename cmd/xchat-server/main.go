package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"xchat/internal/admin"
	"xchat/internal/securestore"
	"xchat/internal/server"
)

type serverConfig struct {
	address                 string
	databasePath            string
	allowedNetworks         string
	encryptionKeyFile       string
	adminSocket             string
	tlsCertificate          string
	tlsKey                  string
	kaomojiPath             string
	allowInsecure           bool
	updateDirectory         string
	installerDirectory      string
	updatePublicKeyFile     string
	validateUpdates         bool
	publicURL               string
	initializeEncryptionKey bool
}

func parseConfig(args []string) (serverConfig, error) {
	set := flag.NewFlagSet("xchat-server", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	config := serverConfig{}
	set.StringVar(&config.address, "listen", "127.0.0.1:18081", "HTTP/WebSocket listen address")
	set.StringVar(&config.databasePath, "db", "data/rooms.db", "SQLite database path")
	set.StringVar(&config.allowedNetworks, "allow-cidr", "127.0.0.0/8,::1/128,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16", "Allowed client CIDRs")
	set.StringVar(&config.encryptionKeyFile, "encryption-key-file", "", "Required base64 32-byte database encryption key file")
	set.StringVar(&config.adminSocket, "admin-socket", "", "Private administrative Unix socket path")
	set.StringVar(&config.tlsCertificate, "tls-cert", os.Getenv("XCHAT_TLS_CERT"), "TLS certificate chain PEM file")
	set.StringVar(&config.tlsKey, "tls-key", os.Getenv("XCHAT_TLS_KEY"), "TLS private key PEM file")
	set.StringVar(&config.kaomojiPath, "kaomoji", os.Getenv("XCHAT_KAOMOJI"), "Kaomoji catalog JSON file; changes reload on GET")
	set.BoolVar(&config.allowInsecure, "allow-insecure", false, "Allow plaintext non-loopback listener for debugging")
	set.StringVar(&config.updateDirectory, "update-dir", "", "Verified client update directory")
	set.StringVar(&config.installerDirectory, "installer-dir", "", "Verified initial installer directory")
	set.StringVar(&config.updatePublicKeyFile, "update-public-key-file", "", "Base64 Ed25519 update public key file")
	set.BoolVar(&config.validateUpdates, "validate-updates", false, "Validate update catalog and exit")
	set.StringVar(&config.publicURL, "public-url", os.Getenv("SPACECHAT_PUBLIC_URL"), "Public ws[s] URL used by dynamic installer scripts")
	set.BoolVar(&config.initializeEncryptionKey, "init-encryption-key", false, "Create a database encryption key only when both key and database are absent")
	if err := set.Parse(args); err != nil {
		return serverConfig{}, err
	}
	if set.NArg() != 0 {
		return serverConfig{}, errors.New("unexpected positional arguments")
	}
	if (config.updateDirectory == "") != (config.updatePublicKeyFile == "") {
		return serverConfig{}, errors.New("-update-dir and -update-public-key-file must be configured together")
	}
	if config.installerDirectory != "" && config.updatePublicKeyFile == "" {
		return serverConfig{}, errors.New("-installer-dir requires -update-dir and -update-public-key-file")
	}
	if config.validateUpdates && config.updateDirectory == "" {
		return serverConfig{}, errors.New("-validate-updates requires an update directory and public key")
	}
	return config, nil
}

func initializeEncryptionKey(config serverConfig) error {
	if !config.initializeEncryptionKey || config.validateUpdates {
		return nil
	}
	return securestore.EnsureKey(config.encryptionKeyFile, config.databasePath)
}

func loadInstallerOptions(config serverConfig) ([]server.RoomsOption, *server.InstallerCatalog, error) {
	if config.installerDirectory == "" {
		return nil, nil, nil
	}
	if config.updatePublicKeyFile == "" {
		return nil, nil, errors.New("installer directory requires an update public key file")
	}
	publicKey, err := readUpdatePublicKey(config.updatePublicKeyFile)
	if err != nil {
		return nil, nil, fmt.Errorf("installer public key: %w", err)
	}
	catalog, err := server.LoadInstallerCatalog(config.installerDirectory, publicKey)
	if err != nil {
		return nil, nil, fmt.Errorf("installer catalog: %w", err)
	}
	if err = catalog.SetPublicURL(config.publicURL); err != nil {
		return nil, nil, fmt.Errorf("installer public URL: %w", err)
	}
	return []server.RoomsOption{server.WithInstallerCatalog(catalog)}, catalog, nil
}

func readUpdatePublicKey(path string) (ed25519.PublicKey, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 256 {
		return nil, errors.New("update public key must be a small regular file, not a symlink")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	encoded := string(raw)
	if len(encoded) > 0 && encoded[len(encoded)-1] == '\n' {
		encoded = encoded[:len(encoded)-1]
	}
	if len(encoded) > 0 && encoded[len(encoded)-1] == '\r' {
		encoded = encoded[:len(encoded)-1]
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(decoded) != ed25519.PublicKeySize {
		return nil, errors.New("update public key must contain one base64 Ed25519 public key")
	}
	return ed25519.PublicKey(decoded), nil
}

func loadUpdateOptions(config serverConfig) ([]server.RoomsOption, *server.UpdateCatalog, error) {
	if config.updateDirectory == "" && config.updatePublicKeyFile == "" {
		return nil, nil, nil
	}
	if config.updateDirectory == "" || config.updatePublicKeyFile == "" {
		return nil, nil, errors.New("update directory and public key file must be configured together")
	}
	publicKey, err := readUpdatePublicKey(config.updatePublicKeyFile)
	if err != nil {
		return nil, nil, fmt.Errorf("update public key: %w", err)
	}
	catalog, err := server.LoadUpdateCatalog(config.updateDirectory, publicKey)
	if err != nil {
		return nil, nil, fmt.Errorf("update catalog: %w", err)
	}
	return []server.RoomsOption{server.WithUpdateCatalog(catalog)}, catalog, nil
}

func main() {
	config, err := parseConfig(os.Args[1:])
	if err != nil {
		slog.Error("command configuration", "error", err)
		os.Exit(2)
	}
	options, catalog, err := loadUpdateOptions(config)
	if err != nil {
		slog.Error("client update configuration", "error", err)
		os.Exit(1)
	}
	if catalog != nil {
		manifest := catalog.Manifest()
		slog.Info("client updates enabled", "latest", manifest.LatestVersion, "minimum", manifest.MinimumSupportedVersion)
	}
	installerOptions, installerCatalog, err := loadInstallerOptions(config)
	if err != nil {
		slog.Error("client installer configuration", "error", err)
		os.Exit(1)
	}
	options = append(options, installerOptions...)
	if installerCatalog != nil {
		manifest := installerCatalog.Manifest()
		slog.Info("client installers enabled", "version", manifest.Version)
	}
	if err := initializeEncryptionKey(config); err != nil {
		slog.Error("initialize encryption key", "error", err)
		os.Exit(1)
	}
	if config.validateUpdates {
		return
	}
	tlsConfig, err := server.TransportConfig(config.address, config.tlsCertificate, config.tlsKey, config.allowInsecure)
	if err != nil {
		slog.Error("TLS configuration", "error", err)
		os.Exit(1)
	}
	key, err := securestore.ReadKey(config.encryptionKeyFile)
	if err != nil {
		slog.Error("encryption key configuration", "error", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Dir(config.databasePath), 0750); err != nil {
		slog.Error("database directory", "error", err)
		os.Exit(1)
	}
	repository, err := securestore.Open(config.databasePath, key)
	if err != nil {
		slog.Error("open database", "error", err)
		os.Exit(1)
	}
	defer repository.Close()
	service := server.NewRooms(repository, options...)
	defer service.Close()
	if err := service.ConfigureKaomoji(config.kaomojiPath); err != nil {
		slog.Error("kaomoji configuration", "error", err)
		os.Exit(1)
	}
	if config.adminSocket != "" {
		management, err := admin.Start(config.adminSocket, service)
		if err != nil {
			slog.Error("admin socket", "error", err)
			os.Exit(1)
		}
		defer management.Close()
	}
	handler, err := server.RestrictNetworks(service.Handler(), config.allowedNetworks)
	if err != nil {
		slog.Error("network configuration", "error", err)
		os.Exit(1)
	}
	httpServer := &http.Server{Addr: config.address, Handler: handler, TLSConfig: tlsConfig, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 * 1024}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	failures := make(chan error, 1)
	go func() {
		slog.Info("xchat listening", "address", config.address, "database", config.databasePath)
		if tlsConfig != nil {
			failures <- httpServer.ListenAndServeTLS("", "")
		} else {
			failures <- httpServer.ListenAndServe()
		}
	}()
	select {
	case <-ctx.Done():
	case err := <-failures:
		if err != http.ErrServerClosed {
			slog.Error("serve", "error", err)
			os.Exit(1)
		}
	}
	service.Close()
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdown); err != nil {
		slog.Error("shutdown", "error", err)
	}
}

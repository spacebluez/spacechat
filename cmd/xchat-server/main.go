package main

import (
	"context"
	"flag"
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

func main() {
	address := flag.String("listen", "127.0.0.1:18081", "HTTP/WebSocket listen address")
	databasePath := flag.String("db", "data/rooms.db", "SQLite database path")
	allowedNetworks := flag.String("allow-cidr", "127.0.0.0/8,::1/128,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16", "Allowed client CIDRs")
	keyPath := flag.String("encryption-key-file", "", "Required base64 32-byte database encryption key file")
	adminSocket := flag.String("admin-socket", "", "Private administrative Unix socket path")
	tlsCert := flag.String("tls-cert", os.Getenv("XCHAT_TLS_CERT"), "TLS certificate chain PEM file")
	tlsKey := flag.String("tls-key", os.Getenv("XCHAT_TLS_KEY"), "TLS private key PEM file")
	kaomojiPath := flag.String("kaomoji", os.Getenv("XCHAT_KAOMOJI"), "Kaomoji catalog JSON file; changes reload on GET")
	allowInsecure := flag.Bool("allow-insecure", false, "Allow plaintext non-loopback listener for debugging")
	flag.Parse()
	tlsConfig, err := server.TransportConfig(*address, *tlsCert, *tlsKey, *allowInsecure)
	if err != nil {
		slog.Error("TLS configuration", "error", err)
		os.Exit(1)
	}
	key, err := securestore.ReadKey(*keyPath)
	if err != nil {
		slog.Error("encryption key configuration", "error", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Dir(*databasePath), 0750); err != nil {
		slog.Error("database directory", "error", err)
		os.Exit(1)
	}
	repository, err := securestore.Open(*databasePath, key)
	if err != nil {
		slog.Error("open database", "error", err)
		os.Exit(1)
	}
	defer repository.Close()
	service := server.NewRooms(repository)
	defer service.Close()
	if err := service.ConfigureKaomoji(*kaomojiPath); err != nil {
		slog.Error("kaomoji configuration", "error", err)
		os.Exit(1)
	}
	if *adminSocket != "" {
		management, err := admin.Start(*adminSocket, service)
		if err != nil {
			slog.Error("admin socket", "error", err)
			os.Exit(1)
		}
		defer management.Close()
	}
	handler, err := server.RestrictNetworks(service.Handler(), *allowedNetworks)
	if err != nil {
		slog.Error("network configuration", "error", err)
		os.Exit(1)
	}
	httpServer := &http.Server{Addr: *address, Handler: handler, TLSConfig: tlsConfig, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 * 1024}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	failures := make(chan error, 1)
	go func() {
		slog.Info("xchat listening", "address", *address, "database", *databasePath)
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

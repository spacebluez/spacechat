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

	"xchat/internal/server"
	"xchat/internal/store"
)

func main() {
	address := flag.String("listen", "127.0.0.1:18080", "HTTP/WebSocket listen address")
	databasePath := flag.String("db", "data/chat.db", "SQLite database path")
	allowedNetworks := flag.String("allow-cidr", "127.0.0.0/8,::1/128,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16", "Allowed client CIDRs")
	flag.Parse()
	if err := os.MkdirAll(filepath.Dir(*databasePath), 0750); err != nil {
		slog.Error("database directory", "error", err)
		os.Exit(1)
	}
	repository, err := store.Open(*databasePath)
	if err != nil {
		slog.Error("open database", "error", err)
		os.Exit(1)
	}
	defer repository.Close()
	service := server.New(repository)
	defer service.Close()
	handler, err := server.RestrictNetworks(service.Handler(), *allowedNetworks)
	if err != nil {
		slog.Error("network configuration", "error", err)
		os.Exit(1)
	}
	httpServer := &http.Server{Addr: *address, Handler: handler, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 * 1024}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	failures := make(chan error, 1)
	go func() {
		slog.Info("xchat listening", "address", *address, "database", *databasePath)
		failures <- httpServer.ListenAndServe()
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

package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"xchat/internal/protocol"
)

type Cleaner interface {
	ClearHistory() (protocol.Cleared, error)
}
type Server struct{ http *http.Server }

func Handler(cleaner Cleaner) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /clear-history", func(writer http.ResponseWriter, request *http.Request) {
		result, err := cleaner.ClearHistory()
		if err != nil {
			slog.Error("clear history failed", "error", err)
			http.Error(writer, "history cleanup failed", http.StatusInternalServerError)
			return
		}
		slog.Info("history cleared", "deleted", result.Deleted)
		writer.Header().Set("Content-Type", "application/json")
		json.NewEncoder(writer).Encode(result)
	})
	return mux
}
func Start(path string, cleaner Cleaner) (*Server, error) {
	parent, err := os.Stat(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	if !parent.IsDir() || parent.Mode().Perm()&0007 != 0 {
		return nil, errors.New("admin socket directory must exclude other users")
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, errors.New("refusing to replace non-socket file")
		}
		existing, dialError := net.DialTimeout("unix", path, 300*time.Millisecond)
		if dialError == nil {
			existing.Close()
			return nil, errors.New("admin socket is already in use")
		}
		if !errors.Is(dialError, syscall.ECONNREFUSED) && !errors.Is(dialError, os.ErrNotExist) {
			return nil, fmt.Errorf("cannot verify stale admin socket: %w", dialError)
		}
		if err = os.Remove(path); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err = os.Chmod(path, 0600); err != nil {
		listener.Close()
		return nil, err
	}
	server := &Server{http: &http.Server{Handler: Handler(cleaner), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second}}
	go func() {
		if err := server.http.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("admin socket failed", "error", err)
		}
	}()
	return server, nil
}
func (server *Server) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.http.Shutdown(ctx); err != nil {
		server.http.Close()
		return err
	}
	return nil
}

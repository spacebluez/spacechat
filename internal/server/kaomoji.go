package server

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"xchat/internal/kaomoji"
)

type kaomojiSource struct {
	mu          sync.Mutex
	path        string
	body        []byte
	etag        string
	lastFailure string
}

func newKaomojiSource(path string) (*kaomojiSource, error) {
	catalog := kaomoji.Default()
	if path != "" {
		var err error
		catalog, err = kaomoji.ReadFile(path)
		if err != nil {
			return nil, err
		}
	}
	source := &kaomojiSource{path: path}
	if err := source.replace(catalog); err != nil {
		return nil, err
	}
	return source, nil
}

func (source *kaomojiSource) replace(catalog kaomoji.Catalog) error {
	body, err := json.Marshal(catalog)
	if err != nil {
		return err
	}
	if len(body) > kaomoji.MaxCatalogBytes {
		return fmt.Errorf("encoded kaomoji catalog exceeds %d bytes", kaomoji.MaxCatalogBytes)
	}
	source.body = body
	source.etag = fmt.Sprintf(`"%x"`, sha256.Sum256(source.body))
	return nil
}

func (source *kaomojiSource) current() ([]byte, string) {
	source.mu.Lock()
	defer source.mu.Unlock()
	if source.path != "" {
		catalog, err := kaomoji.ReadFile(source.path)
		if err == nil {
			err = source.replace(catalog)
		}
		if err != nil {
			if err.Error() != source.lastFailure {
				slog.Warn("kaomoji reload failed; keeping last valid catalog", "error", err)
				source.lastFailure = err.Error()
			}
		} else {
			source.lastFailure = ""
		}
	}
	return source.body, source.etag
}

func (service *Server) ConfigureKaomoji(path string) error {
	source, err := newKaomojiSource(path)
	if err != nil {
		return err
	}
	service.mu.Lock()
	service.kaomoji = source
	service.mu.Unlock()
	return nil
}

func (service *Server) serveKaomoji(writer http.ResponseWriter, request *http.Request) {
	service.mu.Lock()
	source, closed := service.kaomoji, service.closed
	service.mu.Unlock()
	if closed {
		http.Error(writer, "shutting down", http.StatusServiceUnavailable)
		return
	}
	body, etag := source.current()
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-cache")
	writer.Header().Set("ETag", etag)
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	if request.Header.Get("If-None-Match") == etag {
		writer.WriteHeader(http.StatusNotModified)
		return
	}
	writer.Write(body)
}

package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"xchat/internal/kaomoji"
)

func TestKaomojiReloadRejectsOversizedEncodedResponse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kaomoji.json")
	write := func(catalog kaomoji.Catalog) {
		t.Helper()
		var data bytes.Buffer
		encoder := json.NewEncoder(&data)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(catalog); err != nil {
			t.Fatal(err)
		}
		if data.Len() >= kaomoji.MaxCatalogBytes {
			t.Fatal("test input must fit the input size limit")
		}
		if err := os.WriteFile(path, data.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(kaomoji.Default())
	source, err := newKaomojiSource(path)
	if err != nil {
		t.Fatal(err)
	}
	before, etag := source.current()
	catalog := kaomoji.Catalog{Version: 1, Categories: []kaomoji.Category{{ID: "large", Name: "Large"}}}
	for i := 0; i < 200; i++ {
		catalog.Categories[0].Items = append(catalog.Categories[0].Items, kaomoji.Item{Text: fmt.Sprint(i) + strings.Repeat("<", 1000)})
	}
	write(catalog)
	if _, err := kaomoji.ReadFile(path); err != nil {
		t.Fatal("test input must pass catalog validation", err)
	}
	after, nextETag := source.current()
	if !bytes.Equal(before, after) || nextETag != etag {
		t.Fatal("encoded oversized catalog replaced usable snapshot")
	}
}

func TestKaomojiGETReloadCacheAndInvalidUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kaomoji.json")
	write := func(text string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	first := `{"version":1,"categories":[{"id":"custom","name":"Custom","items":[{"text":"first"}]}]}`
	write(first)
	service := New(nil, "")
	defer service.Close()
	if err := service.ConfigureKaomoji(path); err != nil {
		t.Fatal(err)
	}
	handler := service.Handler()
	get := func(etag string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, "/api/kaomoji", nil)
		request.Header.Set("If-None-Match", etag)
		result := httptest.NewRecorder()
		handler.ServeHTTP(result, request)
		return result
	}
	result := get("")
	if result.Code != http.StatusOK || result.Header().Get("ETag") == "" || result.Header().Get("Cache-Control") != "no-cache" {
		t.Fatal("missing catalog or cache headers", result)
	}
	if _, err := kaomoji.Parse(result.Body.Bytes()); err != nil {
		t.Fatal(err)
	}
	etag := result.Header().Get("ETag")
	if unchanged := get(etag); unchanged.Code != http.StatusNotModified || unchanged.Body.Len() != 0 {
		t.Fatal("unchanged catalog retransmitted", unchanged)
	}
	write(strings.ReplaceAll(first, "first", "second"))
	updated := get(etag)
	if updated.Code != http.StatusOK || updated.Header().Get("ETag") == etag || !strings.Contains(updated.Body.String(), "second") {
		t.Fatal("configuration did not reload", updated)
	}
	write(`{"version":1,"categories":[]}`)
	if invalid := get(""); invalid.Body.String() != updated.Body.String() {
		t.Fatal("invalid configuration replaced last valid catalog", invalid)
	}
	if err := service.ConfigureKaomoji(path); err == nil {
		t.Fatal("accepted invalid configuration on startup")
	}
	write(first)
	if recovered := get(updated.Header().Get("ETag")); recovered.Code != http.StatusOK || recovered.Body.String() != result.Body.String() {
		t.Fatal("catalog did not recover after configuration fixed", recovered)
	}
	writer := httptest.NewRecorder()
	handler.ServeHTTP(writer, httptest.NewRequest(http.MethodPost, "/api/kaomoji", strings.NewReader(first)))
	if writer.Code != http.StatusMethodNotAllowed {
		t.Fatal("public endpoint accepts writes", writer.Code)
	}
	service.Close()
	if result := get(""); result.Code != http.StatusServiceUnavailable {
		t.Fatal("closed server still serves catalog", result.Code)
	}
}

func TestKaomojiGETUsesNetworkAllowlist(t *testing.T) {
	service := New(nil, "")
	defer service.Close()
	handler, err := RestrictNetworks(service.Handler(), "127.0.0.0/8")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/kaomoji", nil)
	request.RemoteAddr = "192.0.2.1:5000"
	writer := httptest.NewRecorder()
	handler.ServeHTTP(writer, request)
	if writer.Code != http.StatusForbidden {
		t.Fatal("catalog bypassed IP policy", writer.Code)
	}
}

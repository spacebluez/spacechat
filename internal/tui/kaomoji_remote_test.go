package tui

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"xchat/internal/client"
	"xchat/internal/kaomoji"
)

func TestRemoteKaomojiLoadsWithoutBlockingDraftAndKeepsLastValid(t *testing.T) {
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"version":1,"categories":[{"id":"remote","name":"Remote","items":[{"text":"remote-only","keywords":["hello"]}]}]}`))
	}))
	defer service.Close()
	model := New("ws" + strings.TrimPrefix(service.URL, "http") + "/ws")
	model.joined, model.connected = true, true
	model.network, model.ctx = client.New(model.address), context.Background()
	model.input.SetValue("draft")
	if len(model.catalog) != 0 {
		t.Fatal("client preloaded a built-in catalog")
	}
	command := model.refreshKaomoji()
	if command == nil || !model.catalogLoading || model.refreshKaomoji() != nil {
		t.Fatal("catalog fetch was not asynchronous or was duplicated")
	}
	model.openKaomojiPicker()
	model.picker.search.SetValue("hello")
	model.Update(command())
	items := model.picker.matches()
	if model.catalogLoading || len(items) != 1 || items[0].Text != "remote-only" || model.input.Value() != "draft" {
		t.Fatal("remote catalog not displayed or draft changed", items)
	}
	model.applyKaomoji(kaomojiResult{source: model.network, err: errors.New("offline")})
	if len(model.picker.matches()) != 1 || model.input.Value() != "draft" || !strings.Contains(model.View(), "更新失败") {
		t.Fatal("failed refresh discarded catalog or draft")
	}
	model.applyKaomoji(kaomojiResult{source: client.New(model.address), catalog: kaomoji.Default()})
	if model.catalog[0].ID != "remote" {
		t.Fatal("late event from old room replaced catalog")
	}
}

func TestRemoteKaomojiUpdatePreservesSelectionAndHandlesRemovedCategory(t *testing.T) {
	model := kaomojiModel()
	model.kaomojiOverride = false
	model.network = client.New(model.address)
	model.openKaomojiPicker()
	model.picker.category, model.picker.selected = 0, 1
	selected := model.picker.matches()[1].Text
	updated := kaomoji.Default()
	updated.Categories[0], updated.Categories[1] = updated.Categories[1], updated.Categories[0]
	model.applyKaomoji(kaomojiResult{source: model.network, catalog: updated})
	if model.picker.category != 1 || model.picker.matches()[model.picker.selected].Text != selected {
		t.Fatal("refresh changed selected expression")
	}
	updated.Categories = updated.Categories[:1]
	model.applyKaomoji(kaomojiResult{source: model.network, catalog: updated})
	if model.picker.category != -1 || len(model.picker.matches()) == 0 {
		t.Fatal("removed category left invalid selection")
	}
}

func TestLocalKaomojiOverrideSkipsRemoteAndSurvivesSwitch(t *testing.T) {
	model := switchingModel()
	model.SetKaomojiOverride(kaomoji.Default())
	if model.refreshKaomoji() != nil {
		t.Fatal("local override fetched remote catalog")
	}
	model.openRoomSwitch()
	model.switcher.key.SetValue("different-room")
	model.submitRoomSwitch()
	defer model.Close()
	target := model.switcher.candidate
	if target == nil || !target.kaomojiOverride || len(target.catalog) != len(model.catalog) {
		t.Fatal("room switch lost local override")
	}
}

package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"xchat/internal/protocol"
)

func TestClientSendsVersionAndStopsOnRequiredUpgrade(t *testing.T) {
	joins := make(chan protocol.Join, 1)
	var connections atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connections.Add(1)
		connection, err := websocket.Accept(writer, request, nil)
		if err != nil {
			return
		}
		defer connection.CloseNow()
		var frame protocol.Frame
		if err = wsjson.Read(request.Context(), connection, &frame); err != nil {
			return
		}
		var join protocol.Join
		if err = json.Unmarshal(frame.Payload, &join); err != nil {
			return
		}
		joins <- join
		failure := protocol.Failure{Code: "upgrade_required", Message: "请更新客户端", MinimumVersion: "0.4.0"}
		_ = wsjson.Write(request.Context(), connection, protocol.Encode("error", frame.RequestID, failure))
	}))
	defer server.Close()

	address := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	network := NewWithInfo(address, Info{Version: "0.3.2", OS: "windows", Arch: "amd64"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		network.Run(ctx, "Alice", "room-key")
		close(done)
	}()
	event := awaitEvent(t, network.Events(), func(event Event) bool { return event.State == "upgrade_required" })
	if event.Detail != "请更新客户端" {
		t.Fatalf("wrong upgrade detail %q", event.Detail)
	}
	select {
	case join := <-joins:
		if join.ClientVersion != "0.3.2" || join.ClientOS != "windows" || join.ClientArch != "amd64" {
			t.Fatalf("wrong client metadata: %+v", join)
		}
	case <-time.After(time.Second):
		t.Fatal("join not received")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("client retried required upgrade")
	}
	time.Sleep(50 * time.Millisecond)
	if connections.Load() != 1 {
		t.Fatalf("required upgrade opened %d connections", connections.Load())
	}
}

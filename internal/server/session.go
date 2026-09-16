package server

import (
	"context"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"xchat/internal/protocol"
)

type session struct {
	connection *websocket.Conn
	ctx        context.Context
	cancel     context.CancelFunc
	outgoing   chan protocol.Frame
	name       string
	syncing    bool
	through    int64
	cursor     int64
	deferred   []protocol.Frame
}

func (client *session) enqueue(frame protocol.Frame) {
	select {
	case <-client.ctx.Done():
		return
	default:
	}
	select {
	case client.outgoing <- frame:
	default:
		client.cancel()
	}
}
func (client *session) deliver(frame protocol.Frame) {
	if client.syncing {
		if len(client.deferred) >= 128 {
			client.cancel()
			return
		}
		client.deferred = append(client.deferred, frame)
		return
	}
	client.enqueue(frame)
}
func (client *session) writeLoop() {
	defer client.cancel()
	for {
		select {
		case <-client.ctx.Done():
			return
		case frame := <-client.outgoing:
			ctx, cancel := context.WithTimeout(client.ctx, 10*time.Second)
			err := wsjson.Write(ctx, client.connection, frame)
			cancel()
			if err != nil {
				return
			}
		}
	}
}
func (client *session) heartbeat() {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-client.ctx.Done():
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(client.ctx, 40*time.Second)
			err := client.connection.Ping(ctx)
			cancel()
			if err != nil {
				client.cancel()
				return
			}
		}
	}
}

package server

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"xchat/internal/protocol"
)

func TestConcurrentSendsAreOrderedAndSlowPeerDoesNotBlock(t *testing.T) {
	repository, service, _ := fixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	slowContext, stopSlow := context.WithCancel(ctx)
	defer stopSlow()
	slow := &session{name: "slow", ctx: slowContext, cancel: stopSlow, outgoing: make(chan protocol.Frame, 1)}
	healthy := &session{name: "healthy", ctx: ctx, cancel: cancel, outgoing: make(chan protocol.Frame, 256)}
	service.mu.Lock()
	service.sessions[slow.name] = slow
	service.sessions[healthy.name] = healthy
	service.mu.Unlock()
	var workers sync.WaitGroup
	for index := 0; index < 50; index++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			service.handle(healthy, protocol.Encode("send", "", protocol.Send{Body: "并发"}))
		}()
	}
	workers.Wait()
	select {
	case <-slowContext.Done():
	default:
		t.Fatal("slow peer was not disconnected")
	}
	latest, err := repository.LatestID()
	if err != nil || latest != 50 {
		t.Fatalf("stored %d: %v", latest, err)
	}
	previous := int64(0)
	for len(healthy.outgoing) > 0 {
		frame := <-healthy.outgoing
		if frame.Type != "message" {
			continue
		}
		var message protocol.Message
		if err := json.Unmarshal(frame.Payload, &message); err != nil {
			t.Fatal(err)
		}
		if message.ID != previous+1 {
			t.Fatalf("order %d -> %d", previous, message.ID)
		}
		previous = message.ID
	}
	if previous != 50 {
		t.Fatalf("delivered %d", previous)
	}
}

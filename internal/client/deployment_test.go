package client

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"xchat/internal/protocol"
)

func TestDeployedRejectsEmptyRoomKey(t *testing.T) {
	address := os.Getenv("XCHAT_SMOKE_ADDRESS")
	if address == "" {
		t.Skip("set XCHAT_SMOKE_ADDRESS to verify deployed authentication")
	}
	for _, key := range []string{""} {
		network := New(address)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		done := make(chan struct{})
		go func() { network.Run(ctx, "unauthorized-check", key); close(done) }()
		awaitEvent(t, network.Events(), func(event Event) bool {
			if event.Frame != nil {
				cancel()
				t.Fatalf("unauthorized client received %s", event.Frame.Type)
			}
			return event.State == "unauthorized"
		})
		cancel()
		<-done
	}
	t.Log("deployed server rejected empty room keys without exposing history")
}

func TestDeploymentSmoke(t *testing.T) {
	address := os.Getenv("XCHAT_SMOKE_ADDRESS")
	if address == "" {
		t.Skip("set XCHAT_SMOKE_ADDRESS to test a deployed server")
	}
	marker := os.Getenv("XCHAT_SMOKE_MARKER")
	if marker == "" {
		t.Fatal("XCHAT_SMOKE_MARKER is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	suffix := time.Now().Format("150405")
	receiver := New(address)
	done := make(chan struct{})
	go func() { receiver.Run(ctx, "验收接收-"+suffix, os.Getenv("XCHAT_SMOKE_KEY")); close(done) }()
	defer func() { cancel(); <-done }()
	found := false
	awaitEvent(t, receiver.Events(), func(event Event) bool {
		if event.Frame != nil && event.Frame.Type == "history" {
			var page protocol.Page
			json.Unmarshal(event.Frame.Payload, &page)
			for _, message := range page.Messages {
				if message.Body == marker {
					found = true
				}
			}
		}
		return event.State == "connected"
	})
	if os.Getenv("XCHAT_SMOKE_MODE") == "verify" {
		if !found {
			t.Fatal("persisted deployment marker missing")
		}
		t.Log("persisted marker found after service restart")
		return
	}
	sender := New(address)
	senderDone := make(chan struct{})
	go func() { sender.Run(ctx, "验收发送-"+suffix, os.Getenv("XCHAT_SMOKE_KEY")); close(senderDone) }()
	defer func() { cancel(); <-senderDone }()
	awaitEvent(t, sender.Events(), func(event Event) bool { return event.State == "connected" })
	if err := sender.Send("deployment-check", marker); err != nil {
		t.Fatal(err)
	}
	awaitEvent(t, receiver.Events(), func(event Event) bool {
		if event.Frame == nil || event.Frame.Type != "message" {
			return false
		}
		var message protocol.Message
		json.Unmarshal(event.Frame.Payload, &message)
		return message.Body == marker && message.Nickname == "验收发送-"+suffix
	})
	awaitEvent(t, sender.Events(), func(event Event) bool {
		return event.Frame != nil && event.Frame.Type == "ack" && event.Frame.RequestID == "deployment-check"
	})
	t.Log("two deployed clients exchanged and acknowledged the marker")
}

package client

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"testing"
	"time"

	"xchat/internal/protocol"
)

func TestDeploymentKaomojiCatalog(t *testing.T) {
	address := os.Getenv("XCHAT_SMOKE_ADDRESS")
	if address == "" {
		t.Skip("set XCHAT_SMOKE_ADDRESS to test deployed kaomoji catalog")
	}
	network := deploymentClient(t, address)
	catalog, err := network.FetchKaomoji(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	keyword := os.Getenv("XCHAT_KAOMOJI_EXPECT_KEYWORD")
	found := keyword == ""
	count := 0
	for _, category := range catalog.Categories {
		count += len(category.Items)
		for _, item := range category.Items {
			found = found || slices.Contains(item.Keywords, keyword)
		}
	}
	if !found {
		t.Fatal("server catalog is missing expected updated keyword")
	}
	if _, err := network.FetchKaomoji(context.Background()); err != nil {
		t.Fatal("conditional catalog request failed", err)
	}
	t.Logf("deployed HTTPS catalog and conditional GET verified: %d categories, %d items", len(catalog.Categories), count)
}

func deploymentClient(t *testing.T, address string) *Client {
	t.Helper()
	var options Options
	if path := os.Getenv("XCHAT_SMOKE_CA"); path != "" {
		pool, err := LoadRootCAs(path)
		if err != nil {
			t.Fatal(err)
		}
		options.RootCAs = pool
	}
	return New(address, options)
}

func TestDeployedRejectsEmptyRoomKey(t *testing.T) {
	address := os.Getenv("XCHAT_SMOKE_ADDRESS")
	if address == "" {
		t.Skip("set XCHAT_SMOKE_ADDRESS to verify deployed authentication")
	}
	for _, key := range []string{""} {
		network := deploymentClient(t, address)
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
	receiver := deploymentClient(t, address)
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
	sender := deploymentClient(t, address)
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

func TestDeploymentMentionsAndRecall(t *testing.T) {
	address, key := os.Getenv("XCHAT_SMOKE_ADDRESS"), os.Getenv("XCHAT_SMOKE_KEY")
	if address == "" {
		t.Skip("set XCHAT_SMOKE_ADDRESS to test deployed chat features")
	}
	if key == "" {
		t.Fatal("XCHAT_SMOKE_KEY must identify a dedicated test room")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	suffix := time.Now().Format("150405")
	sender, receiver := deploymentClient(t, address), deploymentClient(t, address)
	start := func(network *Client, name string) {
		done := make(chan struct{})
		go func() { network.Run(ctx, name, key); close(done) }()
		t.Cleanup(func() { cancel(); <-done })
	}
	senderName, receiverName := "发送测试-"+suffix, "接收测试-"+suffix
	start(sender, senderName)
	awaitEvent(t, sender.Events(), func(event Event) bool { return event.State == "connected" })
	start(receiver, receiverName)
	awaitEvent(t, receiver.Events(), func(event Event) bool { return event.State == "connected" })
	// Wait for presence on the sender so the deployed server sees both members.
	awaitEvent(t, sender.Events(), func(event Event) bool {
		if event.Frame == nil || event.Frame.Type != "presence" {
			return false
		}
		var presence protocol.Presence
		if json.Unmarshal(event.Frame.Payload, &presence) != nil {
			return false
		}
		for _, name := range presence.Users {
			if name == receiverName {
				return true
			}
		}
		return false
	})
	body := protocol.MentionText(receiverName) + " (^_^)"
	if err := sender.Send("feature-send", body); err != nil {
		t.Fatal(err)
	}
	ack := awaitEvent(t, sender.Events(), func(event Event) bool { return event.Frame != nil && event.Frame.Type == "ack" })
	var message protocol.Message
	if err := json.Unmarshal(ack.Frame.Payload, &message); err != nil || !message.CanRecall {
		t.Fatal("own message not recallable", err)
	}
	event := awaitEvent(t, receiver.Events(), func(event Event) bool { return event.Frame != nil && event.Frame.Type == "message" })
	var received protocol.Message
	if err := json.Unmarshal(event.Frame.Payload, &received); err != nil || received.Body != body || len(received.Mentions) != 1 || received.Mentions[0] != receiverName || received.CanRecall {
		t.Fatal("deployed mention or ownership mismatch", err)
	}
	if err := sender.Recall("feature-recall", message.ID); err != nil {
		t.Fatal(err)
	}
	for _, network := range []*Client{sender, receiver} {
		event := awaitEvent(t, network.Events(), func(event Event) bool { return event.Frame != nil && event.Frame.Type == "recalled" })
		var recalled protocol.Recalled
		if err := json.Unmarshal(event.Frame.Payload, &recalled); err != nil || recalled.Message.ID != message.ID || recalled.Message.Body != "" || !recalled.Message.Recalled || len(recalled.Message.Mentions) != 0 {
			t.Fatal("deployed recall mismatch", err)
		}
	}
	fresh := sender.NewPeer()
	start(fresh, "历史测试-"+suffix)
	event = awaitEvent(t, fresh.Events(), func(event Event) bool { return event.Frame != nil && event.Frame.Type == "history" })
	var page protocol.Page
	if err := json.Unmarshal(event.Frame.Payload, &page); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range page.Messages {
		if entry.ID == message.ID {
			found = entry.Recalled && entry.Body == "" && len(entry.Mentions) == 0
		}
	}
	if !found {
		t.Fatal("fresh client did not receive persisted recall")
	}
	t.Log("deployed WSS clients sent kaomoji, resolved a member mention, recalled the message and verified fresh history")
}

package server

import (
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"xchat/internal/protocol"
	"xchat/internal/securestore"
)

func (client *session) visibleMessage(message protocol.Message) protocol.Message {
	created, err := time.Parse(time.RFC3339Nano, message.CreatedAt)
	message.CanRecall = client.owner != "" && message.OwnerID == client.owner && !message.Recalled && err == nil && time.Since(created) <= protocol.RecallWindow
	return message
}

func (client *session) visiblePage(page protocol.Page) protocol.Page {
	for i := range page.Messages {
		page.Messages[i] = client.visibleMessage(page.Messages[i])
	}
	return page
}

func (service *Server) recall(client *session, frame protocol.Frame, fail func(string, string)) {
	if client.syncing {
		fail("syncing", "同步完成后才能撤回")
		return
	}
	var request protocol.Recall
	if json.Unmarshal(frame.Payload, &request) != nil || request.MessageID <= 0 {
		fail("invalid_request", "撤回消息编号无效")
		return
	}
	room, ok := client.repository.(*securestore.Room)
	if !ok {
		fail("unsupported", "当前服务不支持撤回")
		return
	}
	message, err := room.Recall(request.MessageID, client.owner)
	if errors.Is(err, securestore.ErrRecallDenied) {
		fail("recall_denied", "只能撤回当前客户端发送的本人消息")
		return
	}
	if errors.Is(err, securestore.ErrRecallExpired) {
		fail("recall_expired", "消息已超过两分钟，无法撤回")
		return
	}
	if err != nil {
		slog.Error("recall message", "error", err)
		fail("storage_error", "撤回失败，请稍后重试")
		return
	}
	for _, recipient := range service.sessions {
		if recipient.room != client.room {
			continue
		}
		requestID := ""
		if recipient == client {
			requestID = frame.RequestID
		}
		recipient.deliver(protocol.Encode("recalled", requestID, protocol.Recalled{Message: message, InstanceID: room.InstanceID()}))
	}
}

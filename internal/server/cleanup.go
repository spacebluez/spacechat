package server

import (
	"errors"
	"time"

	"xchat/internal/protocol"
)

func (service *Server) ClearHistory() (protocol.Cleared, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.closed {
		return protocol.Cleared{}, errors.New("chat service is shutting down")
	}
	deleted, err := service.repository.Clear()
	if err != nil {
		return protocol.Cleared{}, err
	}
	result := protocol.Cleared{InstanceID: service.repository.InstanceID(), Deleted: deleted, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	frame := protocol.Encode("history_cleared", "", result)
	for _, client := range service.sessions {
		client.syncing = false
		client.deferred = nil
		client.cursor = 0
		client.through = 0
		client.enqueue(frame)
	}
	service.presence()
	return result, nil
}

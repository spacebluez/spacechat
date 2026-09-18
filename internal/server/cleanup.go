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
	var deleted int64
	var err error
	if service.rooms != nil {
		deleted, err = service.rooms.ClearAll()
	} else {
		deleted, err = service.repository.Clear()
	}
	if err != nil {
		return protocol.Cleared{}, err
	}
	result := protocol.Cleared{Deleted: deleted, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if service.rooms == nil {
		result.InstanceID = service.repository.InstanceID()
	}
	for _, client := range service.sessions {
		client.syncing = false
		client.deferred = nil
		client.cursor = 0
		client.through = 0
		event := result
		if service.rooms != nil {
			event.Deleted = 0
		}
		if client.repository != nil {
			event.InstanceID = client.repository.InstanceID()
		}
		client.enqueue(protocol.Encode("history_cleared", "", event))
	}
	service.presence()
	return result, nil
}

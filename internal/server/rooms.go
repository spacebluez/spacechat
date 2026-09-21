package server

import (
	"context"
	"xchat/internal/securestore"
)

func NewRooms(repository *securestore.Store, options ...RoomsOption) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	service := &Server{rooms: repository, sessions: make(map[string]*session), ctx: ctx, cancel: cancel}
	for _, option := range options {
		option(service)
	}
	return service
}

func (client *session) sessionKey() string {
	if client.room == "" {
		return client.name
	}
	return client.room + "\x00" + client.name
}

func (service *Server) health() error {
	if service.rooms != nil {
		return service.rooms.Health()
	}
	_, err := service.repository.LatestID()
	return err
}

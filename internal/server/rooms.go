package server

import "xchat/internal/securestore"

func NewRooms(repository *securestore.Store) *Server {
	service := New(nil, "")
	service.rooms = repository
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

package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"xchat/internal/protocol"
)

type Repository interface {
	InstanceID() string
	LatestID() (int64, error)
	Append(string, string) (protocol.Message, error)
	Page(int64, int64, int64) (protocol.Page, error)
}

type Server struct {
	mu         sync.Mutex
	repository Repository
	sessions   map[string]*session
	ctx        context.Context
	cancel     context.CancelFunc
	closed     bool
}

func New(repository Repository) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{repository: repository, sessions: make(map[string]*session), ctx: ctx, cancel: cancel}
}
func (service *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(writer http.ResponseWriter, request *http.Request) {
		service.mu.Lock()
		defer service.mu.Unlock()
		if service.closed {
			http.Error(writer, "shutting down", http.StatusServiceUnavailable)
			return
		}
		if _, err := service.repository.LatestID(); err != nil {
			http.Error(writer, "storage unavailable", http.StatusServiceUnavailable)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.Write([]byte("{\"status\":\"ok\"}\n"))
	})
	mux.HandleFunc("GET /ws", service.serveConnection)
	return mux
}
func (service *Server) Close() {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.closed = true
	service.cancel()
	for _, client := range service.sessions {
		client.cancel()
	}
}
func (service *Server) serveConnection(writer http.ResponseWriter, request *http.Request) {
	connection, err := websocket.Accept(writer, request, nil)
	if err != nil {
		return
	}
	defer connection.CloseNow()
	connection.SetReadLimit(32 * 1024)
	ctx, cancel := context.WithCancel(service.ctx)
	defer cancel()
	client := &session{connection: connection, ctx: ctx, cancel: cancel, outgoing: make(chan protocol.Frame, 256)}
	joinContext, joinCancel := context.WithTimeout(ctx, 10*time.Second)
	var frame protocol.Frame
	err = wsjson.Read(joinContext, connection, &frame)
	joinCancel()
	if err != nil {
		return
	}
	var join protocol.Join
	failure := protocol.Failure{}
	if frame.Type != "join" || json.Unmarshal(frame.Payload, &join) != nil {
		failure = protocol.Failure{Code: "invalid_join", Message: "请先提交昵称"}
	} else {
		failure = service.join(client, join)
	}
	if failure.Code != "" {
		writeContext, writeCancel := context.WithTimeout(ctx, 5*time.Second)
		defer writeCancel()
		wsjson.Write(writeContext, connection, protocol.Encode("error", frame.RequestID, failure))
		connection.Close(websocket.StatusPolicyViolation, failure.Code)
		return
	}
	defer service.leave(client)
	go client.writeLoop()
	go client.heartbeat()
	for {
		if err = wsjson.Read(ctx, connection, &frame); err != nil {
			return
		}
		service.handle(client, frame)
	}
}
func (service *Server) join(client *session, join protocol.Join) protocol.Failure {
	join.Nickname = strings.TrimSpace(join.Nickname)
	if err := protocol.ValidateName(join.Nickname); err != nil {
		return protocol.Failure{Code: "invalid_name", Message: err.Error()}
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.closed {
		return protocol.Failure{Code: "unavailable", Message: "服务正在关闭"}
	}
	if _, exists := service.sessions[join.Nickname]; exists {
		return protocol.Failure{Code: "name_taken", Message: "昵称已在线，请修改昵称或等待旧连接释放"}
	}
	latest, err := service.repository.LatestID()
	if err != nil {
		return protocol.Failure{Code: "storage_error", Message: "无法读取历史"}
	}
	if join.AfterID < 0 || join.AfterID > latest && join.InstanceID == service.repository.InstanceID() {
		return protocol.Failure{Code: "invalid_cursor", Message: "同步游标无效"}
	}
	resumed := join.InstanceID == service.repository.InstanceID()
	before, after := int64(0), int64(0)
	if resumed {
		before = -1
		after = join.AfterID
	}
	page, err := service.repository.Page(before, after, latest)
	if err != nil {
		return protocol.Failure{Code: "storage_error", Message: "无法读取历史"}
	}
	client.name = join.Nickname
	client.through = latest
	client.syncing = resumed
	service.sessions[client.name] = client
	client.enqueue(protocol.Encode("welcome", "", protocol.Welcome{InstanceID: service.repository.InstanceID(), ThroughID: latest, Resumed: resumed, Users: service.users()}))
	if resumed {
		client.enqueue(protocol.Encode("sync", "initial", page))
		client.cursor = join.AfterID
		if len(page.Messages) > 0 {
			client.cursor = page.Messages[len(page.Messages)-1].ID
		}
	} else {
		client.enqueue(protocol.Encode("history", "initial", page))
	}
	if !resumed || !page.HasMore {
		service.finishSync(client)
	}
	service.presence()
	return protocol.Failure{}
}
func (service *Server) leave(client *session) {
	client.cancel()
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.sessions[client.name] == client {
		delete(service.sessions, client.name)
		service.presence()
	}
}
func (service *Server) users() []string {
	users := make([]string, 0, len(service.sessions))
	for name := range service.sessions {
		users = append(users, name)
	}
	sort.Strings(users)
	return users
}
func (service *Server) presence() {
	frame := protocol.Encode("presence", "", protocol.Presence{Users: service.users()})
	for _, client := range service.sessions {
		client.deliver(frame)
	}
}
func (service *Server) finishSync(client *session) {
	client.enqueue(protocol.Encode("sync_complete", "", protocol.Complete{ThroughID: client.through}))
	client.syncing = false
	for _, frame := range client.deferred {
		client.enqueue(frame)
	}
	client.deferred = nil
}
func (service *Server) handle(client *session, frame protocol.Frame) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.closed || client.ctx.Err() != nil {
		return
	}
	fail := func(code, message string) {
		client.enqueue(protocol.Encode("error", frame.RequestID, protocol.Failure{Code: code, Message: message}))
	}
	if len(frame.RequestID) > 128 {
		fail("invalid_request", "请求标识过长")
		return
	}
	switch frame.Type {
	case "send":
		if client.syncing {
			fail("syncing", "同步完成后才能发送")
			return
		}
		var request protocol.Send
		if json.Unmarshal(frame.Payload, &request) != nil {
			fail("invalid_message", "消息格式错误")
			return
		}
		if err := protocol.ValidateBody(request.Body); err != nil {
			fail("invalid_message", err.Error())
			return
		}
		message, err := service.repository.Append(client.name, request.Body)
		if err != nil {
			slog.Error("persist message", "error", err)
			fail("storage_error", "保存失败，消息未发送")
			return
		}
		for _, recipient := range service.sessions {
			requestID := ""
			if recipient == client {
				requestID = frame.RequestID
			}
			recipient.deliver(protocol.Encode("message", requestID, message))
		}
		client.enqueue(protocol.Encode("ack", frame.RequestID, message))
	case "history":
		if client.syncing {
			fail("syncing", "请等待同步结束")
			return
		}
		var query protocol.Query
		if json.Unmarshal(frame.Payload, &query) != nil || query.BeforeID <= 0 || query.AfterID != 0 {
			fail("invalid_cursor", "历史游标无效")
			return
		}
		latest, err := service.repository.LatestID()
		if err != nil {
			fail("storage_error", "读取历史失败")
			return
		}
		page, err := service.repository.Page(query.BeforeID, 0, latest)
		if err != nil {
			fail("storage_error", "读取历史失败")
			return
		}
		client.enqueue(protocol.Encode("history", frame.RequestID, page))
	case "sync":
		var query protocol.Query
		if !client.syncing || json.Unmarshal(frame.Payload, &query) != nil || query.AfterID != client.cursor || query.BeforeID != 0 {
			fail("invalid_cursor", "同步游标无效")
			return
		}
		page, err := service.repository.Page(-1, query.AfterID, client.through)
		if err != nil {
			fail("storage_error", "补齐历史失败")
			client.cancel()
			return
		}
		client.enqueue(protocol.Encode("sync", frame.RequestID, page))
		if len(page.Messages) > 0 {
			client.cursor = page.Messages[len(page.Messages)-1].ID
		}
		if !page.HasMore {
			service.finishSync(client)
		}
	default:
		fail("invalid_request", "未知请求类型")
	}
}

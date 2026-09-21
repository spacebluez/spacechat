package client

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/url"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"xchat/internal/kaomoji"
	"xchat/internal/protocol"
)

type Event struct {
	Frame  *protocol.Frame
	State  string
	Detail string
}
type link struct {
	ctx      context.Context
	outgoing chan protocol.Frame
	ready    bool
}
type Info struct {
	Version string
	OS      string
	Arch    string
}
type Client struct {
	address       string
	info          Info
	events        chan Event
	mu            sync.Mutex
	active        *link
	instance      string
	cursor        int64
	everConnected bool
	retryMin      time.Duration
	options       Options
	token         string
	catalogMu     sync.Mutex
	catalog       *kaomoji.Catalog
	catalogETag   string
}

func New(address string, configuration ...Options) *Client {
	return NewWithInfo(address, Info{}, configuration...)
}

func NewWithInfo(address string, info Info, configuration ...Options) *Client {
	var options Options
	if len(configuration) > 0 {
		options = configuration[0]
	}
	var token [32]byte
	cryptorand.Read(token[:])
	return &Client{address: address, info: info, events: make(chan Event, 128), retryMin: time.Second, options: options, token: hex.EncodeToString(token[:])}
}
func ValidateAddress(address string) error {
	parsed, err := url.Parse(address)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "ws" && parsed.Scheme != "wss") || parsed.User != nil || parsed.Fragment != "" {
		return errors.New("服务器地址必须为 ws://host:port/ws 或 wss://host:port/ws")
	}
	return nil
}
func (network *Client) Events() <-chan Event { return network.events }
func (network *Client) emit(ctx context.Context, event Event) bool {
	select {
	case network.events <- event:
		return true
	case <-ctx.Done():
		return false
	}
}
func (network *Client) Send(requestID, body string) error {
	if err := protocol.ValidateBody(body); err != nil {
		return err
	}
	return network.queue(protocol.Encode("send", requestID, protocol.Send{Body: body}))
}
func (network *Client) History(before int64) error {
	return network.queue(protocol.Encode("history", "older", protocol.Query{BeforeID: before}))
}
func (network *Client) Recall(requestID string, messageID int64) error {
	if messageID <= 0 {
		return errors.New("撤回消息编号无效")
	}
	return network.queue(protocol.Encode("recall", requestID, protocol.Recall{MessageID: messageID}))
}
func (network *Client) queue(frame protocol.Frame) error {
	network.mu.Lock()
	defer network.mu.Unlock()
	if network.active == nil || !network.active.ready || network.active.ctx.Err() != nil {
		return errors.New("当前未连接，请等待重连")
	}
	select {
	case network.active.outgoing <- frame:
		return nil
	default:
		return errors.New("发送队列已满，请稍后再试")
	}
}
func (network *Client) Run(ctx context.Context, name, key string) {
	defer close(network.events)
	if err := ValidateTransport(network.address, network.options.AllowInsecure); err != nil {
		network.emit(ctx, Event{State: "invalid_address", Detail: err.Error()})
		return
	}
	if err := protocol.ValidateName(name); err != nil {
		network.emit(ctx, Event{State: "invalid_name", Detail: err.Error()})
		return
	}
	delay := network.retryMin
	for ctx.Err() == nil {
		network.emit(ctx, Event{State: "connecting", Detail: "正在连接服务器…"})
		started := time.Now()
		err := network.connect(ctx, name, key)
		if ctx.Err() != nil {
			return
		}
		var certificateError *tls.CertificateVerificationError
		if errors.As(err, &certificateError) {
			network.emit(ctx, Event{State: "tls_error", Detail: fmt.Sprintf("TLS 证书校验失败：%v", certificateError.Err)})
			return
		}
		var refusal *joinError
		if errors.As(err, &refusal) && (refusal.code == "unauthorized" || refusal.code == "invalid_name" || refusal.code == "upgrade_required" || refusal.code == "unsupported_client" || (refusal.code == "name_taken" && !network.everConnected)) {
			network.emit(ctx, Event{State: refusal.code, Detail: refusal.message})
			return
		}
		detail := "连接断开，正在重试"
		if err != nil {
			detail = fmt.Sprintf("连接断开：%v", err)
		}
		network.emit(ctx, Event{State: "disconnected", Detail: detail})
		if time.Since(started) > 30*time.Second {
			delay = network.retryMin
		}
		timer := time.NewTimer(delay + time.Duration(rand.Int64N(int64(delay/4)+1)))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		delay = min(delay*2, 30*time.Second)
	}
}

type joinError struct{ code, message string }

func (failure *joinError) Error() string { return failure.message }
func decode[Value any](frame protocol.Frame) (Value, error) {
	var value Value
	err := json.Unmarshal(frame.Payload, &value)
	return value, err
}
func (network *Client) connect(parent context.Context, name, key string) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	dialContext, dialCancel := context.WithTimeout(ctx, 10*time.Second)
	httpClient := network.options.httpClient()
	defer httpClient.CloseIdleConnections()
	connection, _, err := websocket.Dial(dialContext, network.address, &websocket.DialOptions{HTTPClient: httpClient})
	dialCancel()
	if err != nil {
		return err
	}
	defer connection.CloseNow()
	connection.SetReadLimit(2 * 1024 * 1024)
	writeContext, writeCancel := context.WithTimeout(ctx, 10*time.Second)
	err = wsjson.Write(writeContext, connection, protocol.Encode("join", "join", protocol.Join{
		Nickname:      name,
		AccessKey:     key,
		InstanceID:    network.instance,
		AfterID:       network.cursor,
		ClientToken:   network.token,
		ClientVersion: network.info.Version,
		ClientOS:      network.info.OS,
		ClientArch:    network.info.Arch,
	}))
	writeCancel()
	if err != nil {
		return err
	}
	current := &link{ctx: ctx, outgoing: make(chan protocol.Frame, 64)}
	network.mu.Lock()
	network.active = current
	network.mu.Unlock()
	defer func() {
		network.mu.Lock()
		if network.active == current {
			network.active = nil
		}
		network.mu.Unlock()
	}()
	go func() {
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case frame := <-current.outgoing:
				writeContext, writeCancel := context.WithTimeout(ctx, 10*time.Second)
				err := wsjson.Write(writeContext, connection, frame)
				writeCancel()
				if err != nil {
					return
				}
			}
		}
	}()
	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pingContext, pingCancel := context.WithTimeout(ctx, 40*time.Second)
				err := connection.Ping(pingContext)
				pingCancel()
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()
	welcomed := false
	for {
		var frame protocol.Frame
		err = wsjson.Read(ctx, connection, &frame)
		if err != nil {
			return err
		}
		switch frame.Type {
		case "welcome":
			welcome, err := decode[protocol.Welcome](frame)
			if err != nil {
				return err
			}
			if !welcome.Resumed {
				network.cursor = 0
			}
			network.instance = welcome.InstanceID
			welcomed = true
		case "history", "sync":
			page, err := decode[protocol.Page](frame)
			if err != nil {
				return err
			}
			if !network.emit(ctx, Event{Frame: &frame}) {
				return ctx.Err()
			}
			if frame.Type == "sync" || frame.RequestID == "initial" {
				for _, message := range page.Messages {
					network.cursor = max(network.cursor, message.ID)
				}
			}
			if frame.Type == "sync" && page.HasMore {
				if len(page.Messages) == 0 {
					return errors.New("服务器返回空的同步页")
				}
				select {
				case current.outgoing <- protocol.Encode("sync", "next", protocol.Query{AfterID: network.cursor}):
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			continue
		case "message":
			message, err := decode[protocol.Message](frame)
			if err != nil {
				return err
			}
			if !network.emit(ctx, Event{Frame: &frame}) {
				return ctx.Err()
			}
			network.cursor = max(network.cursor, message.ID)
			continue
		case "recalled":
			recalled, err := decode[protocol.Recalled](frame)
			if err != nil {
				return err
			}
			if !network.emit(ctx, Event{Frame: &frame}) {
				return ctx.Err()
			}
			network.instance = recalled.InstanceID
			continue
		case "history_cleared":
			cleared, err := decode[protocol.Cleared](frame)
			if err != nil {
				return err
			}
			network.instance = cleared.InstanceID
			network.cursor = 0
			network.everConnected = true
			network.mu.Lock()
			current.ready = true
			network.mu.Unlock()
		case "sync_complete":
			complete, err := decode[protocol.Complete](frame)
			if err != nil {
				return err
			}
			network.cursor = max(network.cursor, complete.ThroughID)
			network.everConnected = true
			network.mu.Lock()
			current.ready = true
			network.mu.Unlock()
			if !network.emit(ctx, Event{Frame: &frame}) {
				return ctx.Err()
			}
			network.emit(ctx, Event{State: "connected", Detail: "已连接"})
			continue
		case "error":
			failure, err := decode[protocol.Failure](frame)
			if err != nil {
				return err
			}
			if !welcomed {
				return &joinError{code: failure.Code, message: failure.Message}
			}
		}
		if !network.emit(ctx, Event{Frame: &frame}) {
			return ctx.Err()
		}
	}
}

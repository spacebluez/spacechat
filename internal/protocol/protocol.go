package protocol

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const PageSize = 100
const RecallWindow = 2 * time.Minute

type Frame struct {
	Type      string          `json:"type"`
	RequestID string          `json:"request_id,omitempty"`
	Payload   json.RawMessage `json:"payload"`
}

type Message struct {
	ID        int64    `json:"id"`
	Nickname  string   `json:"nickname"`
	Body      string   `json:"body"`
	CreatedAt string   `json:"created_at"`
	Mentions  []string `json:"mentions,omitempty"`
	Recalled  bool     `json:"recalled,omitempty"`
	CanRecall bool     `json:"can_recall,omitempty"`
	OwnerID   string   `json:"-"`
}

type Join struct {
	AccessKey   string `json:"access_key"`
	Nickname    string `json:"nickname"`
	InstanceID  string `json:"instance_id,omitempty"`
	AfterID     int64  `json:"after_id,omitempty"`
	ClientToken string `json:"client_token,omitempty"`
}

type Welcome struct {
	InstanceID string   `json:"instance_id"`
	ThroughID  int64    `json:"through_id"`
	Resumed    bool     `json:"resumed"`
	Users      []string `json:"users"`
}

type Page struct {
	Messages  []Message `json:"messages"`
	HasMore   bool      `json:"has_more"`
	ThroughID int64     `json:"through_id"`
}

type Query struct {
	BeforeID int64 `json:"before_id,omitempty"`
	AfterID  int64 `json:"after_id,omitempty"`
}

type Send struct {
	Body string `json:"body"`
}
type Recall struct {
	MessageID int64 `json:"message_id"`
}
type Recalled struct {
	Message    Message `json:"message"`
	InstanceID string  `json:"instance_id"`
}
type Presence struct {
	Users []string `json:"users"`
}
type Failure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type Cleared struct {
	InstanceID string `json:"instance_id"`
	Deleted    int64  `json:"deleted"`
	CreatedAt  string `json:"created_at"`
}

type Complete struct {
	ThroughID int64 `json:"through_id"`
}

func Encode(kind, requestID string, payload any) Frame {
	data, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return Frame{Type: kind, RequestID: requestID, Payload: data}
}

func ValidateName(name string) error {
	if name != strings.TrimSpace(name) || utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 20 {
		return errors.New("昵称须为 1–20 个字符，首尾不能有空白")
	}
	return validateText(name)
}

func ValidateBody(body string) error {
	if strings.TrimSpace(body) == "" || utf8.RuneCountInString(body) > 2000 {
		return errors.New("消息须为 1–2000 个字符且不能全为空白")
	}
	return validateText(strings.ReplaceAll(body, "\n", ""))
}

func validateText(text string) error {
	if !utf8.ValidString(text) {
		return errors.New("文本必须为有效 UTF-8")
	}
	for _, character := range text {
		if unicode.IsControl(character) || unicode.Is(unicode.Cf, character) {
			return errors.New("文本不能包含控制字符")
		}
	}
	return nil
}

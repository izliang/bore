package queue

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"time"
)

// simpleMessage 是 Message 接口的默认实现，用于构造外发消息。
// 各后端实现可能使用自己的类型包装此结构或提供独立实现。
type simpleMessage struct {
	body      []byte
	headers   map[string]string
	messageID string
}

// NewMessage 使用给定的字节数组创建消息，自动生成唯一 messageID。
//
// body 会被拷贝，调用方可以安全复用传入的切片。
func NewMessage(body []byte) Message {
	return &simpleMessage{
		body:      slices.Clone(body),
		headers:   make(map[string]string),
		messageID: newMessageID(),
	}
}

// NewMessageWithID 使用调用方提供的 messageID 创建消息。
// 适用于需要跨系统追踪消息的场景（如从上游传入 messageID 时保持一致）。
func NewMessageWithID(body []byte, messageID string) Message {
	return &simpleMessage{
		body:      slices.Clone(body),
		headers:   make(map[string]string),
		messageID: messageID,
	}
}

// NewJSONMessage 将 v 序列化为 JSON 并创建消息，自动设置 Content-Type 为 application/json。
//
// 这是最常用的消息构造方式，适用于结构化的业务数据。
func NewJSONMessage(v any) (Message, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	m := &simpleMessage{
		body:      body,
		headers:   map[string]string{"Content-Type": "application/json"},
		messageID: newMessageID(),
	}
	return m, nil
}

// NewJSONMessageWithID 将 v 序列化为 JSON 并使用指定的 messageID 创建消息。
func NewJSONMessageWithID(v any, messageID string) (Message, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	m := &simpleMessage{
		body:      body,
		headers:   map[string]string{"Content-Type": "application/json"},
		messageID: messageID,
	}
	return m, nil
}

// Body 返回消息体字节的副本，调用方可安全修改。
func (m *simpleMessage) Body() []byte { return slices.Clone(m.body) }

// Headers 返回消息头的副本，调用方可安全修改。
func (m *simpleMessage) Headers() map[string]string { return maps.Clone(m.headers) }

// SetHeader 设置单个消息头，返回消息自身以支持链式调用。
// 仅在构造消息阶段使用（simpleMessage 专有方法）。
func (m *simpleMessage) SetHeader(key, value string) *simpleMessage {
	if m.headers == nil {
		m.headers = make(map[string]string)
	}
	m.headers[key] = value
	return m
}

// Ack 对外发消息为无操作（只有接收到的消息才需要 Ack/Nack）。
func (m *simpleMessage) Ack() error { return nil }

// Nack 对外发消息为无操作。
func (m *simpleMessage) Nack(requeue bool) error { return nil }

// MessageID 返回消息的唯一标识。
func (m *simpleMessage) MessageID() string { return m.messageID }

// ParseJSON 将消息体反序列化到 v 中。msg 不能为 nil。
//
// 使用示例：
//
//	var payload MyStruct
//	if err := queue.ParseJSON(msg, &payload); err != nil {
//	    return queue.Drop(err)
//	}
func ParseJSON(msg Message, v any) error {
	if msg == nil {
		return ErrInvalidMessage
	}
	return json.Unmarshal(msg.Body(), v)
}

// newMessageID 使用 crypto/rand 生成 16 字节的十六进制消息 ID。
// 如果随机数生成失败（极端情况），回退到纳秒时间戳。
func newMessageID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("msg-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// MessageBuilder 提供流式 API 构造消息，支持链式调用。
//
// 适用于需要设置多个 header 或自定义属性的场景：
//
//	msg := queue.NewMessageBuilder([]byte("body")).
//	    WithHeader("X-Trace-ID", traceID).
//	    WithContentType("text/plain").
//	    WithTimestamp(time.Now()).
//	    Build()
type MessageBuilder struct {
	body      []byte
	headers   map[string]string
	messageID string
}

// NewMessageBuilder 创建消息构造器，body 会被拷贝。
func NewMessageBuilder(body []byte) *MessageBuilder {
	return &MessageBuilder{
		body:      slices.Clone(body),
		headers:   make(map[string]string),
		messageID: newMessageID(),
	}
}

// WithHeader 添加单个消息头，返回构造器自身以支持链式调用。
func (b *MessageBuilder) WithHeader(key, value string) *MessageBuilder {
	b.headers[key] = value
	return b
}

// WithHeaders 批量添加消息头。
func (b *MessageBuilder) WithHeaders(headers map[string]string) *MessageBuilder {
	maps.Copy(b.headers, headers)
	return b
}

// WithMessageID 设置自定义消息 ID。
func (b *MessageBuilder) WithMessageID(id string) *MessageBuilder {
	b.messageID = id
	return b
}

// WithContentType 设置 Content-Type 头。
func (b *MessageBuilder) WithContentType(ct string) *MessageBuilder {
	b.headers["Content-Type"] = ct
	return b
}

// WithTimestamp 添加发布时间戳头（X-Publish-Timestamp）。
func (b *MessageBuilder) WithTimestamp(t time.Time) *MessageBuilder {
	b.headers["X-Publish-Timestamp"] = t.Format(time.RFC3339Nano)
	return b
}

// Build 返回构造好的 Message，其 body 和 headers 均为独立副本。
func (b *MessageBuilder) Build() Message {
	return &simpleMessage{
		body:      slices.Clone(b.body),
		headers:   maps.Clone(b.headers),
		messageID: b.messageID,
	}
}

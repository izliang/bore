package kafka

import (
	"sort"
	"sync/atomic"

	"github.com/izliang/bore/queue"
	"github.com/segmentio/kafka-go"
)

// kafkaMessage 将 kafka.Message 适配为 queue.Message 接口。
//
// Ack/Nack 操作通过 commitFn/nackFn 回调实现，支持 offset 提交和重试路由。
// 使用 atomic.Bool 确保重复确认安全（幂等）。
type kafkaMessage struct {
	msg       kafka.Message
	messageID string      // 当 msg.Key 为空时的后备 messageID
	committed atomic.Bool // 防止重复确认
	commitFn  func(kafka.Message) error
	nackFn    func(kafka.Message, bool) error
}

// Body 返回消息体的字节切片。
func (m *kafkaMessage) Body() []byte {
	return m.msg.Value
}

// Headers 返回消息头的副本，key-value 均为字符串。
func (m *kafkaMessage) Headers() map[string]string {
	return fromKafkaHeaders(m.msg.Headers)
}

// Ack 确认消息已成功处理，提交 offset。
// 重复调用安全（第二次及之后为无操作）。
func (m *kafkaMessage) Ack() error {
	if !m.committed.CompareAndSwap(false, true) {
		return nil
	}
	if m.commitFn != nil {
		return m.commitFn(m.msg)
	}
	return nil
}

// Nack 否定确认。requeue=true 时发布到 retry topic，requeue=false 时发布到 DLQ。
// 重复调用安全。
func (m *kafkaMessage) Nack(requeue bool) error {
	if !m.committed.CompareAndSwap(false, true) {
		return nil
	}
	if m.nackFn != nil {
		return m.nackFn(m.msg, requeue)
	}
	return nil
}

// MessageID 返回消息的唯一标识符。
// 优先使用 Kafka 消息的 Key，如果 Key 为空则使用内部记录的 messageID。
func (m *kafkaMessage) MessageID() string {
	if len(m.msg.Key) > 0 {
		return string(m.msg.Key)
	}
	if m.messageID != "" {
		return m.messageID
	}
	return ""
}

// toKafkaHeaders 将 map[string]string 转换为排序的 kafka.Header 切片。
// kafka-go 要求 headers 按 key 字母序排列以确保确定性哈希。
func toKafkaHeaders(headers map[string]string) []kafka.Header {
	if len(headers) == 0 {
		return nil
	}
	result := make([]kafka.Header, 0, len(headers))
	for k, v := range headers {
		result = append(result, kafka.Header{Key: k, Value: []byte(v)})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Key < result[j].Key
	})
	return result
}

// fromKafkaHeaders 将 kafka.Header 切片转换为 map[string]string。
func fromKafkaHeaders(headers []kafka.Header) map[string]string {
	result := make(map[string]string, len(headers))
	for _, h := range headers {
		result[h.Key] = string(h.Value)
	}
	return result
}

// 编译期接口实现校验。
var _ queue.Message = (*kafkaMessage)(nil)

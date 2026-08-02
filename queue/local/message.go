package local

import "sync/atomic"

// localMessage implements queue.Message for in-process delivery.
//
// Ack/Nack semantics:
//   - Ack() marks the message as successfully processed. Safe to call multiple times.
//   - Nack(true) is a no-op (requeue is handled by the consumer via channel re-send).
//   - Nack(false) marks the message as dropped. Safe to call multiple times.
//
// The acked/nacked atomics are for observability; the actual lifecycle is managed
// by the consumer's retry/requeue logic.
type localMessage struct {
	body      []byte
	headers   map[string]string
	messageID string

	acked      atomic.Bool
	nacked     atomic.Bool
	retryCount int
}

// Body returns the message payload. The returned slice is a copy for safety.
func (m *localMessage) Body() []byte {
	body := make([]byte, len(m.body))
	copy(body, m.body)
	return body
}

// Headers returns the message headers. The returned map is a copy.
func (m *localMessage) Headers() map[string]string {
	h := make(map[string]string, len(m.headers))
	for k, v := range m.headers {
		h[k] = v
	}
	return h
}

// Ack acknowledges successful processing.
func (m *localMessage) Ack() error {
	m.acked.Store(true)
	return nil
}

// Nack negatively acknowledges. requeue=true is a no-op (handled by consumer);
// requeue=false marks as dropped.
func (m *localMessage) Nack(requeue bool) error {
	if !requeue {
		m.nacked.Store(true)
	}
	return nil
}

// MessageID returns the message identifier.
func (m *localMessage) MessageID() string {
	return m.messageID
}

// ack is an internal method called by the consumer after successful processing.
func (m *localMessage) ack() {
	m.acked.Store(true)
}

// nack is an internal method called by the consumer when dropping a message.
func (m *localMessage) nack() {
	m.nacked.Store(true)
}

// IsAcked returns true if the message has been acknowledged.
func (m *localMessage) IsAcked() bool {
	return m.acked.Load()
}

// IsNacked returns true if the message has been dead-lettered (dropped).
func (m *localMessage) IsNacked() bool {
	return m.nacked.Load()
}

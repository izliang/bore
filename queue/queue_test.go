package queue

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMessage(t *testing.T) {
	body := []byte("hello world")
	msg := NewMessage(body)

	assert.Equal(t, body, msg.Body())
	assert.NotEmpty(t, msg.MessageID())
	assert.NotNil(t, msg.Headers())
	assert.Empty(t, msg.Headers())

	// Ack/Nack are no-ops for outbound messages
	assert.NoError(t, msg.Ack())
	assert.NoError(t, msg.Nack(true))
}

func TestNewMessageWithID(t *testing.T) {
	msg := NewMessageWithID([]byte("payload"), "my-custom-id")
	assert.Equal(t, "my-custom-id", msg.MessageID())
	assert.Equal(t, []byte("payload"), msg.Body())
}

func TestNewJSONMessage(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	msg, err := NewJSONMessage(payload{Name: "Alice", Age: 30})
	require.NoError(t, err)
	assert.Equal(t, "application/json", msg.Headers()["Content-Type"])

	var result payload
	err = ParseJSON(msg, &result)
	require.NoError(t, err)
	assert.Equal(t, "Alice", result.Name)
	assert.Equal(t, 30, result.Age)
}

func TestNewJSONMessageError(t *testing.T) {
	// json.Marshal fails on channels
	_, err := NewJSONMessage(make(chan int))
	assert.Error(t, err)
}

func TestNewJSONMessageWithID(t *testing.T) {
	msg, err := NewJSONMessageWithID(map[string]string{"k": "v"}, "id-123")
	require.NoError(t, err)
	assert.Equal(t, "id-123", msg.MessageID())
	assert.Equal(t, "application/json", msg.Headers()["Content-Type"])

	var result map[string]string
	err = ParseJSON(msg, &result)
	require.NoError(t, err)
	assert.Equal(t, "v", result["k"])
}

func TestMessageBuilder(t *testing.T) {
	msg := NewMessageBuilder([]byte("body")).
		WithHeader("x-custom", "value").
		WithContentType("text/plain").
		WithMessageID("builder-id").
		WithHeaders(map[string]string{"x-trace": "trace123"}).
		Build()

	assert.Equal(t, []byte("body"), msg.Body())
	assert.Equal(t, "builder-id", msg.MessageID())
	assert.Equal(t, "value", msg.Headers()["x-custom"])
	assert.Equal(t, "text/plain", msg.Headers()["Content-Type"])
	assert.Equal(t, "trace123", msg.Headers()["x-trace"])
}

func TestSimpleMessageSetHeader(t *testing.T) {
	sm := &simpleMessage{
		body:      []byte("body"),
		messageID: "id",
	}
	sm.SetHeader("k1", "v1").SetHeader("k2", "v2")
	assert.Equal(t, "v1", sm.Headers()["k1"])
	assert.Equal(t, "v2", sm.Headers()["k2"])
}

func TestParseJSON(t *testing.T) {
	msg := NewMessage([]byte(`{"x": 1, "y": "hello"}`))
	var result map[string]any
	err := ParseJSON(msg, &result)
	require.NoError(t, err)
	assert.Equal(t, float64(1), result["x"])
	assert.Equal(t, "hello", result["y"])
}

func TestParseJSONError(t *testing.T) {
	msg := NewMessage([]byte(`not json`))
	var result map[string]any
	err := ParseJSON(msg, &result)
	assert.Error(t, err)
}

func TestNewMessageIDUniqueness(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		msg := NewMessage([]byte("test"))
		id := msg.MessageID()
		assert.False(t, ids[id], "duplicate message ID: %s", id)
		ids[id] = true
	}
}

func TestDefaultDeclareOptions(t *testing.T) {
	opts := DefaultDeclareOptions()
	assert.True(t, opts.Durable)
	assert.False(t, opts.AutoDelete)
	assert.False(t, opts.Exclusive)
	assert.False(t, opts.NoWait)
}

func TestDefaultSubscribeOptions(t *testing.T) {
	opts := DefaultSubscribeOptions()
	assert.False(t, opts.AutoAck)
	assert.Equal(t, 1, opts.Concurrency)
	assert.Equal(t, 0, opts.PrefetchCount)
}

func TestDefaultRetryPolicy(t *testing.T) {
	policy := DefaultRetryPolicy()
	assert.Equal(t, 3, policy.MaxRetries)
	assert.Equal(t, "1s", policy.Backoff.String())
	assert.Equal(t, "1m0s", policy.MaxBackoff.String())
}

func TestHandleResultHelpers(t *testing.T) {
	err := assert.AnError

	assert.Equal(t, HandleAck, HandleResult{}.Action)
	assert.Equal(t, HandleAck, Ack().Action)

	retry := Retry(err)
	assert.Equal(t, HandleRetry, retry.Action)
	assert.Equal(t, err, retry.Err)

	drop := Drop(err)
	assert.Equal(t, HandleNack, drop.Action)
	assert.Equal(t, err, drop.Err)
	assert.False(t, drop.Requeue)

	requeue := Requeue(err)
	assert.Equal(t, HandleNack, requeue.Action)
	assert.Equal(t, err, requeue.Err)
	assert.True(t, requeue.Requeue)
}

func TestSentinelErrors(t *testing.T) {
	// Verify sentinel errors are distinct
	assert.NotEqual(t, ErrQueueClosed, ErrNotInitialized)
	assert.NotEqual(t, ErrPublishFailed, ErrSubscribeFailed)
	assert.NotEqual(t, ErrDeclareFailed, ErrConnectionFailed)
}

func TestNewMetricsGetCollectors(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := NewMetrics("queue_test")
	registry.MustRegister(m.GetCollectors()...)

	m.MessagesPublished.WithLabelValues("vhost", "q", "").Inc()
	m.HandlerPanics.WithLabelValues("vhost", "q").Inc()
}

func TestMessageRoundTripJSON(t *testing.T) {
	original := struct {
		Action string `json:"action"`
		Data   string `json:"data"`
	}{
		Action: "process",
		Data:   "important-data",
	}

	msg, err := NewJSONMessage(original)
	require.NoError(t, err)

	var restored struct {
		Action string `json:"action"`
		Data   string `json:"data"`
	}
	err = json.Unmarshal(msg.Body(), &restored)
	require.NoError(t, err)

	assert.Equal(t, original.Action, restored.Action)
	assert.Equal(t, original.Data, restored.Data)
}

func TestNewMessageIDCollisionResistance(t *testing.T) {
	// Generate 10k IDs and verify no collisions
	ids := make(map[string]bool, 10000)
	for i := 0; i < 10000; i++ {
		id := newMessageID()
		assert.False(t, ids[id], "collision at iteration %d: %s", i, id)
		ids[id] = true
		// Verify ID format (hex encoded, 32 chars for 16 bytes)
		assert.Len(t, id, 32)
	}
}

func TestMessageBuilderChaining(t *testing.T) {
	msg := NewMessageBuilder([]byte("payload")).
		WithContentType("application/json").
		WithHeader("x-request-id", "req-123").
		WithHeader("x-trace-id", "trace-456").
		WithTimestamp(time.Date(2026, 5, 12, 10, 0, 0, 0, time.UTC)).
		WithMessageID("custom-id").
		Build()

	assert.Equal(t, []byte("payload"), msg.Body())
	assert.Equal(t, "custom-id", msg.MessageID())
	assert.Equal(t, "application/json", msg.Headers()["Content-Type"])
	assert.Equal(t, "req-123", msg.Headers()["x-request-id"])
	assert.Equal(t, "trace-456", msg.Headers()["x-trace-id"])
	assert.Contains(t, msg.Headers()["X-Publish-Timestamp"], "2026-05-12T10:00:00")
}

func TestDeclareOptionsDefaults(t *testing.T) {
	opts := DefaultDeclareOptions()
	assert.True(t, opts.Durable)
	assert.False(t, opts.AutoDelete)
	assert.False(t, opts.Exclusive)
	assert.False(t, opts.NoWait)
	assert.Nil(t, opts.Arguments)
	assert.Empty(t, opts.DeadLetterExchange)
	assert.Equal(t, time.Duration(0), opts.MessageTTL)
}

func TestSubscribeOptionsDefaults(t *testing.T) {
	opts := DefaultSubscribeOptions()
	assert.False(t, opts.AutoAck)
	assert.Equal(t, 1, opts.Concurrency)
	assert.Equal(t, 0, opts.PrefetchCount)
	assert.Empty(t, opts.ConsumerTag)
	assert.Nil(t, opts.Retry)
}

func TestRetryPolicyDefaults(t *testing.T) {
	policy := DefaultRetryPolicy()
	assert.Equal(t, 3, policy.MaxRetries)
	assert.Equal(t, time.Second, policy.Backoff)
	assert.Equal(t, 60*time.Second, policy.MaxBackoff)
}

func TestParseJSONWithNilMessage(t *testing.T) {
	var v map[string]any
	err := ParseJSON(nil, &v)
	assert.ErrorIs(t, err, ErrInvalidMessage)
}

func TestSimpleMessageHeadersImmutability(t *testing.T) {
	m := NewMessage([]byte("body")).(*simpleMessage)
	m.SetHeader("a", "1")

	headers := m.Headers()
	headers["b"] = "2"

	assert.Empty(t, m.Headers()["b"])
	assert.Equal(t, "1", m.Headers()["a"])
}

func TestNewMessageBuilderDefaultHeaders(t *testing.T) {
	b := NewMessageBuilder([]byte("test"))
	assert.NotNil(t, b.headers)
	assert.Empty(t, b.headers)
	assert.NotEmpty(t, b.messageID)
}

func TestDeclareOptionsCustom(t *testing.T) {
	opts := &DeclareOptions{
		Durable:            false,
		AutoDelete:         true,
		DeadLetterExchange: "my-dlx",
		MessageTTL:         5 * time.Minute,
		MaxLength:          1000,
	}
	assert.False(t, opts.Durable)
	assert.True(t, opts.AutoDelete)
	assert.Equal(t, "my-dlx", opts.DeadLetterExchange)
	assert.Equal(t, 5*time.Minute, opts.MessageTTL)
	assert.Equal(t, 1000, opts.MaxLength)
}

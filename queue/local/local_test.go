package local

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/izliang/bore/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLocalQueue(t *testing.T) {
	q := New(Options{DefaultBufferSize: 100})
	assert.True(t, q.IsHealthy())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	assert.NoError(t, q.Shutdown(ctx))
	assert.False(t, q.IsHealthy())
}

func TestPublishAndSubscribe(t *testing.T) {
	q := New(Options{})
	defer q.Shutdown(context.Background())

	q.Declare("test-queue", nil)

	// Subscribe first (in goroutine)
	var received atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = q.Subscribe(ctx, "test-queue", func(ctx context.Context, msg queue.Message) queue.HandleResult {
			received.Add(1)
			assert.Equal(t, "hello", string(msg.Body()))
			return queue.Ack()
		}, nil)
	}()

	// Give subscriber time to start
	time.Sleep(50 * time.Millisecond)

	// Publish
	err := q.Publish(context.Background(), "test-queue", queue.NewMessage([]byte("hello")))
	require.NoError(t, err)

	// Wait for delivery
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, int32(1), received.Load())
}

func TestPublishToAutoCreatedQueue(t *testing.T) {
	q := New(Options{})
	defer q.Shutdown(context.Background())

	// Publish without explicit Declare - auto-creates the queue
	err := q.Publish(context.Background(), "auto-queue", queue.NewMessage([]byte("auto")))
	require.NoError(t, err)

	// Now subscribe and verify we get the message
	var received atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = q.Subscribe(ctx, "auto-queue", func(ctx context.Context, msg queue.Message) queue.HandleResult {
			received.Add(1)
			assert.Equal(t, "auto", string(msg.Body()))
			return queue.Ack()
		}, nil)
	}()

	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, int32(1), received.Load())
}

func TestPublishClosedQueue(t *testing.T) {
	q := New(Options{})
	q.Shutdown(context.Background())

	err := q.Publish(context.Background(), "q", queue.NewMessage([]byte("x")))
	assert.ErrorIs(t, err, queue.ErrQueueClosed)
}

func TestPublishNilMessage(t *testing.T) {
	q := New(Options{})
	defer q.Shutdown(context.Background())

	err := q.Publish(context.Background(), "q", nil)
	assert.ErrorIs(t, err, queue.ErrInvalidMessage)
}

func TestSubscribeNilHandler(t *testing.T) {
	q := New(Options{})
	defer q.Shutdown(context.Background())

	err := q.Subscribe(context.Background(), "q", nil, nil)
	assert.ErrorIs(t, err, queue.ErrInvalidHandler)
}

func TestSubscribeClosedQueue(t *testing.T) {
	q := New(Options{})
	q.Shutdown(context.Background())

	err := q.Subscribe(context.Background(), "q", func(ctx context.Context, msg queue.Message) queue.HandleResult {
		return queue.Ack()
	}, nil)
	assert.ErrorIs(t, err, queue.ErrQueueClosed)
}

func TestDeclareClosedQueue(t *testing.T) {
	q := New(Options{})
	q.Shutdown(context.Background())

	err := q.Declare("q", nil)
	assert.ErrorIs(t, err, queue.ErrQueueClosed)
}

func TestShutdownIdempotent(t *testing.T) {
	q := New(Options{})
	assert.NoError(t, q.Shutdown(context.Background()))
	assert.NoError(t, q.Shutdown(context.Background())) // second call is no-op
}

func TestConcurrentPublishAndShutdownDoesNotPanic(t *testing.T) {
	q := New(Options{DefaultBufferSize: 1000})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = q.Subscribe(ctx, "race", func(ctx context.Context, msg queue.Message) queue.HandleResult {
			return queue.Ack()
		}, &queue.SubscribeOptions{Concurrency: 4})
	}()

	time.Sleep(50 * time.Millisecond)

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				_ = q.Publish(context.Background(), "race", queue.NewMessage([]byte("x")))
			}
		}()
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutdownCancel()
	assert.NoError(t, q.Shutdown(shutdownCtx))
	wg.Wait()
}

func TestConcurrentPublish(t *testing.T) {
	q := New(Options{DefaultBufferSize: 1000})
	defer q.Shutdown(context.Background())

	var received atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = q.Subscribe(ctx, "concurrent", func(ctx context.Context, msg queue.Message) queue.HandleResult {
			received.Add(1)
			return queue.Ack()
		}, &queue.SubscribeOptions{Concurrency: 4})
	}()

	time.Sleep(50 * time.Millisecond)

	// Publish from multiple goroutines
	const numGoroutines = 10
	const msgsPerGoroutine = 100
	var wg sync.WaitGroup

	for i := range numGoroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := range msgsPerGoroutine {
				body := fmt.Sprintf("msg-%d-%d", id, j)
				err := q.Publish(context.Background(), "concurrent", queue.NewMessage([]byte(body)))
				assert.NoError(t, err)
			}
		}(i)
	}

	wg.Wait()

	// Wait for all messages to be processed
	time.Sleep(500 * time.Millisecond)
	assert.Equal(t, int32(numGoroutines*msgsPerGoroutine), received.Load())
}

func TestSubscribeCancellation(t *testing.T) {
	q := New(Options{})
	defer q.Shutdown(context.Background())

	ctx, cancel := context.WithCancel(context.Background())

	var started atomic.Bool
	done := make(chan error, 1)

	go func() {
		done <- q.Subscribe(ctx, "cancel-test", func(ctx context.Context, msg queue.Message) queue.HandleResult {
			started.Store(true)
			time.Sleep(10 * time.Millisecond)
			return queue.Ack()
		}, nil)
	}()

	// Cancel after a short delay
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("Subscribe did not return after cancellation")
	}
}

func TestAutoAck(t *testing.T) {
	q := New(Options{})
	defer q.Shutdown(context.Background())

	var received atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = q.Subscribe(ctx, "auto-ack", func(ctx context.Context, msg queue.Message) queue.HandleResult {
			received.Add(1)
			// Return an error but AutoAck=true means it should still be acked
			return queue.Retry(fmt.Errorf("some error"))
		}, &queue.SubscribeOptions{AutoAck: true})
	}()

	time.Sleep(50 * time.Millisecond)

	q.Publish(context.Background(), "auto-ack", queue.NewMessage([]byte("test")))
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, int32(1), received.Load())
}

func TestNackWithoutRequeue(t *testing.T) {
	q := New(Options{})
	defer q.Shutdown(context.Background())

	var received atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = q.Subscribe(ctx, "nack-test", func(ctx context.Context, msg queue.Message) queue.HandleResult {
			received.Add(1)
			msg.Nack(false) // explicitly nack, no requeue
			return queue.Drop(fmt.Errorf("handler error"))
		}, nil)
	}()

	time.Sleep(50 * time.Millisecond)

	q.Publish(context.Background(), "nack-test", queue.NewMessage([]byte("test")))
	time.Sleep(100 * time.Millisecond)

	// Should be received exactly once (no retries by default)
	assert.Equal(t, int32(1), received.Load())
}

func TestRequeueResult(t *testing.T) {
	q := New(Options{})
	defer q.Shutdown(context.Background())

	var received atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = q.Subscribe(ctx, "requeue-test", func(ctx context.Context, msg queue.Message) queue.HandleResult {
			if received.Add(1) == 1 {
				return queue.Requeue(fmt.Errorf("temporary unavailable"))
			}
			return queue.Ack()
		}, nil)
	}()

	time.Sleep(50 * time.Millisecond)

	q.Publish(context.Background(), "requeue-test", queue.NewMessage([]byte("test")))
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, int32(2), received.Load())
}

func TestRetryPolicy(t *testing.T) {
	q := New(Options{})
	defer q.Shutdown(context.Background())

	var attempts atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})

	go func() {
		defer close(done)
		_ = q.Subscribe(ctx, "retry-test", func(ctx context.Context, msg queue.Message) queue.HandleResult {
			attempts.Add(1)
			return queue.Retry(fmt.Errorf("always fail"))
		}, &queue.SubscribeOptions{
			Retry: &queue.RetryPolicy{
				MaxRetries: 3,
				Backoff:    10 * time.Millisecond,
				MaxBackoff: 50 * time.Millisecond,
			},
		})
	}()

	time.Sleep(50 * time.Millisecond)

	q.Publish(context.Background(), "retry-test", queue.NewMessage([]byte("test")))

	// Wait for all retries to complete (3 retries + initial = 4 attempts)
	time.Sleep(500 * time.Millisecond)
	cancel()
	<-done

	assert.Equal(t, int32(4), attempts.Load()) // initial + 3 retries
}

func TestMultipleSubscribersSameQueue(t *testing.T) {
	q := New(Options{DefaultBufferSize: 100})
	defer q.Shutdown(context.Background())

	var received1, received2 atomic.Int32
	ctx1, cancel1 := context.WithCancel(context.Background())
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel1()
	defer cancel2()

	// Two subscribers on the same queue
	go func() {
		_ = q.Subscribe(ctx1, "multi-sub", func(ctx context.Context, msg queue.Message) queue.HandleResult {
			received1.Add(1)
			return queue.Ack()
		}, nil)
	}()

	go func() {
		_ = q.Subscribe(ctx2, "multi-sub", func(ctx context.Context, msg queue.Message) queue.HandleResult {
			received2.Add(1)
			return queue.Ack()
		}, nil)
	}()

	time.Sleep(50 * time.Millisecond)

	// Publish 10 messages
	for i := 0; i < 10; i++ {
		q.Publish(context.Background(), "multi-sub", queue.NewMessage([]byte("msg")))
	}

	time.Sleep(200 * time.Millisecond)

	total := received1.Load() + received2.Load()
	assert.Equal(t, int32(10), total, "all messages should be consumed exactly once")
}

func TestJSONMessageRoundTrip(t *testing.T) {
	q := New(Options{})
	defer q.Shutdown(context.Background())

	type payload struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}

	var received payload
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = q.Subscribe(ctx, "json-test", func(ctx context.Context, msg queue.Message) queue.HandleResult {
			err := queue.ParseJSON(msg, &received)
			assert.NoError(t, err)
			return queue.Ack()
		}, nil)
	}()

	time.Sleep(50 * time.Millisecond)

	msg, err := queue.NewJSONMessage(payload{Name: "test", Count: 42})
	require.NoError(t, err)
	q.Publish(context.Background(), "json-test", msg)

	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done

	assert.Equal(t, "test", received.Name)
	assert.Equal(t, 42, received.Count)
}

func TestPublishWithContextCancellation(t *testing.T) {
	q := New(Options{DefaultBufferSize: 1})
	defer q.Shutdown(context.Background())

	// Fill the buffer
	q.Publish(context.Background(), "full", queue.NewMessage([]byte("a")))

	// Try to publish with cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := q.Publish(ctx, "full", queue.NewMessage([]byte("b")))
	assert.ErrorIs(t, err, context.Canceled)
}

func TestShutdownStopsConsumers(t *testing.T) {
	q := New(Options{})

	ctx := context.Background()
	done := make(chan error, 1)
	go func() {
		done <- q.Subscribe(ctx, "shutdown-test", func(ctx context.Context, msg queue.Message) queue.HandleResult {
			time.Sleep(10 * time.Millisecond)
			return queue.Ack()
		}, &queue.SubscribeOptions{Concurrency: 2})
	}()

	time.Sleep(50 * time.Millisecond)
	for range 5 {
		assert.NoError(t, q.Publish(context.Background(), "shutdown-test", queue.NewMessage([]byte("msg"))))
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutdownCancel()
	assert.NoError(t, q.Shutdown(shutdownCtx))

	select {
	case err := <-done:
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("Subscribe did not return after shutdown")
	}

	err := q.Publish(context.Background(), "shutdown-test", queue.NewMessage([]byte("msg")))
	assert.ErrorIs(t, err, queue.ErrQueueClosed)
}

func TestDeclareWithBufferSize(t *testing.T) {
	q := New(Options{DefaultBufferSize: 10})
	defer q.Shutdown(context.Background())

	// Declare with custom MaxLength as buffer size
	err := q.Declare("custom-buf", &queue.DeclareOptions{MaxLength: 5})
	require.NoError(t, err)

	// Fill the buffer
	for i := 0; i < 5; i++ {
		err := q.Publish(context.Background(), "custom-buf", queue.NewMessage([]byte("fill")))
		require.NoError(t, err)
	}

	// The 6th publish should block (buffer full) - test with a timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err = q.Publish(ctx, "custom-buf", queue.NewMessage([]byte("overflow")))
	assert.Error(t, err) // should timeout
}

func TestHandlerPanic(t *testing.T) {
	q := New(Options{})
	defer q.Shutdown(context.Background())

	var received atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = q.Subscribe(ctx, "panic-test", func(ctx context.Context, msg queue.Message) queue.HandleResult {
			received.Add(1)
			panic("oops!")
		}, &queue.SubscribeOptions{
			Retry: &queue.RetryPolicy{MaxRetries: 0}, // no retries for panics
		})
	}()

	time.Sleep(50 * time.Millisecond)
	q.Publish(context.Background(), "panic-test", queue.NewMessage([]byte("boom")))

	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, int32(1), received.Load(), "handler should execute before panic")
}

func TestPublishWithRoutingIgnoresExchange(t *testing.T) {
	q := New(Options{})
	defer q.Shutdown(context.Background())

	var received atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = q.Subscribe(ctx, "route-test", func(ctx context.Context, msg queue.Message) queue.HandleResult {
			received.Add(1)
			return queue.Ack()
		}, nil)
	}()

	time.Sleep(50 * time.Millisecond)

	// exchange is ignored, message goes to routingKey queue
	err := q.PublishWithRouting(context.Background(), "ignored-exchange", "route-test",
		queue.NewMessage([]byte("routed")))
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, int32(1), received.Load())
}

func TestLocalMessageAckNack(t *testing.T) {
	lm := &localMessage{
		body:      []byte("test"),
		headers:   map[string]string{"x": "y"},
		messageID: "id-1",
	}

	assert.Equal(t, "test", string(lm.Body()))
	assert.Equal(t, "y", lm.Headers()["x"])
	assert.Equal(t, "id-1", lm.MessageID())

	assert.False(t, lm.IsAcked())
	assert.NoError(t, lm.Ack())
	assert.True(t, lm.IsAcked())

	assert.False(t, lm.IsNacked())
	assert.NoError(t, lm.Nack(false))
	assert.True(t, lm.IsNacked())

	// Nack with requeue should not set nacked flag
	lm2 := &localMessage{body: []byte("x")}
	assert.NoError(t, lm2.Nack(true))
	assert.False(t, lm2.IsNacked())
}

func TestNewWithZeroBufferSize(t *testing.T) {
	q := New(Options{DefaultBufferSize: 0})
	defer q.Shutdown(context.Background())
	assert.True(t, q.IsHealthy())

	// Should default to 1024, so a few publishes should work
	for i := 0; i < 10; i++ {
		assert.NoError(t, q.Publish(context.Background(), "buf-test", queue.NewMessage([]byte("x"))))
	}
}

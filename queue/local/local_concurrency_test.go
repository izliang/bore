package local_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/izliang/bore/queue"
	"github.com/izliang/bore/queue/controller"
	"github.com/izliang/bore/queue/local"
)

func TestLocalQueue_DynamicConcurrencyScaleUp(t *testing.T) {
	ctrl := controller.NewStatic(map[string]int{"q": 1})

	q := local.New(local.Options{ConcurrencyController: ctrl})
	defer q.Shutdown(context.Background())

	q.Declare("q", nil)

	var processed atomic.Int64

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		q.Subscribe(ctx, "q", func(ctx context.Context, msg queue.Message) queue.HandleResult {
			processed.Add(1)
			time.Sleep(50 * time.Millisecond)
			return queue.Ack()
		}, nil)
	}()

	// Let the subscriber start.
	time.Sleep(100 * time.Millisecond)

	// Publish 20 messages while concurrency=1 (serial processing).
	for i := 0; i < 20; i++ {
		q.Publish(ctx, "q", queue.NewMessage([]byte("msg")))
	}

	// Scale up to 4.
	ctrl.Set("q", 4)
	time.Sleep(100 * time.Millisecond)

	// Publish 20 more messages with concurrency=4.
	for i := 0; i < 20; i++ {
		q.Publish(ctx, "q", queue.NewMessage([]byte("msg")))
	}

	time.Sleep(500 * time.Millisecond)
	if n := processed.Load(); n < 20 {
		t.Errorf("processed %d messages, want >= 20", n)
	}
}

func TestLocalQueue_DynamicConcurrencyScaleDown(t *testing.T) {
	ctrl := controller.NewStatic(map[string]int{"q": 4})

	q := local.New(local.Options{ConcurrencyController: ctrl})
	defer q.Shutdown(context.Background())

	q.Declare("q", nil)

	var processed atomic.Int64

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		q.Subscribe(ctx, "q", func(ctx context.Context, msg queue.Message) queue.HandleResult {
			processed.Add(1)
			time.Sleep(50 * time.Millisecond)
			return queue.Ack()
		}, nil)
	}()

	time.Sleep(100 * time.Millisecond)

	// Scale down to 1.
	ctrl.Set("q", 1)
	time.Sleep(100 * time.Millisecond)

	// Publish messages.
	for i := 0; i < 10; i++ {
		q.Publish(ctx, "q", queue.NewMessage([]byte("msg")))
	}

	time.Sleep(500 * time.Millisecond)
	if n := processed.Load(); n < 5 {
		t.Errorf("processed %d messages, want >= 5", n)
	}
}

func TestLocalQueue_NoControllerUsesStaticConcurrency(t *testing.T) {
	// Without controller, SubscribeOptions.Concurrency should be used.
	q := local.New(local.Options{})
	defer q.Shutdown(context.Background())

	q.Declare("q", nil)

	var processed atomic.Int64

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		q.Subscribe(ctx, "q", func(ctx context.Context, msg queue.Message) queue.HandleResult {
			processed.Add(1)
			time.Sleep(20 * time.Millisecond)
			return queue.Ack()
		}, &queue.SubscribeOptions{Concurrency: 3})
	}()

	time.Sleep(100 * time.Millisecond)

	for i := 0; i < 15; i++ {
		q.Publish(ctx, "q", queue.NewMessage([]byte("msg")))
	}

	time.Sleep(300 * time.Millisecond)
	if n := processed.Load(); n < 10 {
		t.Errorf("processed %d messages, want >= 10", n)
	}
}

package rabbitmq

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/izliang/bore/propagator"
	"github.com/izliang/bore/propagator/carrier"
	"github.com/izliang/bore/queue"

	amqp "github.com/rabbitmq/amqp091-go"
)

// channelPool is a fixed-size pool of AMQP channels for concurrent publishing.
// Since AMQP channels are not thread-safe, each Publish grabs a channel from the pool.
type channelPool struct {
	cm       *connectionManager
	channels chan *amqp.Channel
	done     chan struct{}
	size     int
	closed   atomic.Bool
}

func newChannelPool(cm *connectionManager, size int) (*channelPool, error) {
	if size < 1 {
		size = 1
	}
	pool := &channelPool{
		cm:       cm,
		channels: make(chan *amqp.Channel, size),
		done:     make(chan struct{}),
		size:     size,
	}
	for range size {
		ch, err := cm.channel()
		if err != nil {
			pool.close()
			return nil, fmt.Errorf("failed to create producer channel: %w", err)
		}
		pool.channels <- ch
	}
	return pool, nil
}

// get acquires a channel from the pool. Blocks if all channels are in use.
func (p *channelPool) get(ctx context.Context) (*amqp.Channel, error) {
	if p.closed.Load() {
		return nil, errQueueClosed
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-p.done:
		return nil, errQueueClosed
	case ch := <-p.channels:
		return ch, nil
	}
}

// put returns a channel to the pool.
func (p *channelPool) put(ch *amqp.Channel) {
	if ch == nil {
		return
	}
	if p.closed.Load() {
		_ = ch.Close() // nolint: errcheck
		return
	}
	select {
	case <-p.done:
		_ = ch.Close() // nolint: errcheck
	case p.channels <- ch:
	default:
		_ = ch.Close() // nolint: errcheck
	}
}

// refresh recreates idle channels in the pool (called after reconnect).
func (p *channelPool) refresh() error {
	if p.closed.Load() {
		return errQueueClosed
	}

	for {
		select {
		case ch := <-p.channels:
			_ = ch.Close() // nolint: errcheck
		default:
			for len(p.channels) < p.size {
				ch, err := p.cm.channel()
				if err != nil {
					return fmt.Errorf("failed to refresh producer channel: %w", err)
				}
				p.channels <- ch
			}
			return nil
		}
	}
}

func (p *channelPool) close() {
	if !p.closed.CompareAndSwap(false, true) {
		return
	}
	close(p.done)
	for {
		select {
		case ch := <-p.channels:
			_ = ch.Close() // nolint: errcheck
		default:
			return
		}
	}
}

// producer implements queue.Producer using a channel pool for concurrency safety.
type producer struct {
	pool       *channelPool
	metrics    *queue.Metrics
	vhost      string
	propagator propagator.Propagator
}

func (p *producer) Publish(ctx context.Context, routingKey string, msg queue.Message) error {
	return p.PublishWithRouting(ctx, "", routingKey, msg)
}

func (p *producer) PublishWithRouting(ctx context.Context, exchange, routingKey string, msg queue.Message) error {
	if msg == nil {
		return fmt.Errorf("%w: nil message", queue.ErrInvalidMessage)
	}

	start := time.Now()

	ch, err := p.pool.get(ctx)
	if err != nil {
		return fmt.Errorf("%w: %v", queue.ErrPublishFailed, err)
	}
	defer p.pool.put(ch)

	headersMap := msg.Headers()
	if p.propagator != nil {
		p.propagator.Inject(ctx, &carrier.MQHeaders{Headers: headersMap})
	}

	headers := amqp.Table{}
	for k, v := range headersMap {
		headers[k] = v
	}

	contentType := headersMap["Content-Type"]
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	publishing := amqp.Publishing{
		Headers:         headers,
		ContentType:     contentType,
		ContentEncoding: headersMap["Content-Encoding"],
		DeliveryMode:    amqp.Persistent,
		MessageId:       msg.MessageID(),
		Timestamp:       time.Now(),
		Body:            msg.Body(),
	}

	err = ch.PublishWithContext(ctx, exchange, routingKey, false, false, publishing)
	if err != nil {
		return fmt.Errorf("%w: %v", queue.ErrPublishFailed, err)
	}

	if p.metrics != nil {
		p.metrics.MessagesPublished.WithLabelValues(p.vhost, routingKey, exchange).Inc()
		p.metrics.PublishDuration.WithLabelValues(p.vhost, routingKey).Observe(time.Since(start).Seconds())
	}

	return nil
}

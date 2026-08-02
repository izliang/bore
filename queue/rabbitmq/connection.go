package rabbitmq

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"maps"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/izliang/bore/logit"
	amqp "github.com/rabbitmq/amqp091-go"
)

// connectionManager owns the AMQP connection, handles automatic reconnection
// with exponential backoff, and provides channel creation.
//
// It is safe for concurrent use: all exported methods use atomics or mutex.
type connectionManager struct {
	url              string
	logger           logit.Logger
	clientProperties amqp.Table
	tlsConfig        *tls.Config

	mu         sync.Mutex
	conn       *amqp.Connection
	closed     atomic.Bool
	connectErr error

	// Reconnect configuration
	reconnectBackoff    time.Duration
	maxReconnectBackoff time.Duration
	reconnectNotify     chan error // receives close errors to trigger reconnect

	onReconnect func() // callback invoked after successful reconnect

	heartbeat time.Duration // 心跳检测时间，如果consumer的执行时间非常久，会导致TCP半开连接导致 永久阻塞。rabbitmq清理掉连接。
}

// newConnectionManager creates a new connection manager and establishes the
// initial connection. It returns an error if the initial connection fails.
func newConnectionManager(url string, cfg *connectionConfig, logger logit.Logger) (*connectionManager, error) {
	if logger == nil {
		logger = logit.NewSimple(nil)
	}
	cm := &connectionManager{
		url:                 url,
		logger:              logger,
		clientProperties:    buildClientProperties(cfg.clientProperties),
		tlsConfig:           cloneTLSConfig(cfg.tlsConfig),
		reconnectBackoff:    cfg.reconnectInitial,
		maxReconnectBackoff: cfg.reconnectMax,
		reconnectNotify:     make(chan error, 1),
		heartbeat:           cfg.heartbeat,
	}
	// 默认10秒
	if cm.heartbeat <= 0 {
		cm.heartbeat = 10 * time.Second
	}
	logger.Info(context.Background(), "connectionManager dial", logit.StringField("url", url),
		logit.JSONDetailAbsField("clientProperties", cm.clientProperties))
	conn, err := cm.dial()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errConnectionFailed, err)
	}
	cm.conn = conn

	return cm, nil
}

func (cm *connectionManager) dial() (*amqp.Connection, error) {
	cfg := amqp.Config{
		Properties: cm.clientProperties,
		Heartbeat:  cm.heartbeat,
	}
	if cm.tlsConfig != nil {
		cfg.TLSClientConfig = cloneTLSConfig(cm.tlsConfig)
	}
	conn, err := amqp.DialConfig(cm.url, cfg)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// startReconnectLoop runs background goroutines that monitor connection health
// and reconnects with exponential backoff when the connection drops.
func (cm *connectionManager) startReconnectLoop(ctx context.Context) {
	// Main reconnect loop: listens on reconnectNotify for close errors.
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case err, ok := <-cm.reconnectNotify:
				if !ok {
					return
				}
				cm.logger.Warn(ctx, "rabbitmq connection lost, reconnecting", logit.ErrorField("error", err))
				cm.reconnect(ctx, err)
			}
		}
	}()

	// Watch the current connection for unexpected closes.
	go func() {
		cm.mu.Lock()
		conn := cm.conn
		cm.mu.Unlock()

		if conn == nil {
			return
		}

		closeCh := conn.NotifyClose(make(chan *amqp.Error, 1))
		select {
		case <-ctx.Done():
			return
		case err, ok := <-closeCh:
			if ok && err != nil {
				select {
				case cm.reconnectNotify <- err:
				default:
				}
			}
		}
	}()
}

func (cm *connectionManager) reconnect(ctx context.Context, reason error) {
	if cm.closed.Load() {
		return
	}

	cm.mu.Lock()
	cm.conn = nil
	cm.connectErr = reason
	cm.mu.Unlock()

	backoff := cm.reconnectBackoff

	for {
		if cm.closed.Load() {
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		conn, err := cm.dial()
		if err != nil {
			backoff = time.Duration(math.Min(
				float64(backoff)*2,
				float64(cm.maxReconnectBackoff),
			))
			cm.logger.Warn(ctx, "rabbitmq reconnect attempt failed",
				logit.StringField("next_backoff", backoff.String()),
				logit.ErrorField("error", err),
			)
			continue
		}

		cm.mu.Lock()
		cm.conn = conn
		cm.connectErr = nil
		cm.mu.Unlock()

		cm.logger.Info(ctx, "rabbitmq reconnected successfully")

		// Notify listeners that we reconnected
		if cm.onReconnect != nil {
			cm.onReconnect()
		}

		// Watch the new connection
		closeCh := conn.NotifyClose(make(chan *amqp.Error, 1))
		go func() {
			select {
			case <-ctx.Done():
				return
			case err, ok := <-closeCh:
				if ok && err != nil {
					select {
					case cm.reconnectNotify <- err:
					default:
					}
				}
			}
		}()

		return
	}
}

// channel creates a new AMQP channel from the current connection.
// Returns an error if the connection is not available.
func (cm *connectionManager) channel() (*amqp.Channel, error) {
	if cm.closed.Load() {
		return nil, errQueueClosed
	}

	cm.mu.Lock()
	conn := cm.conn
	err := cm.connectErr
	cm.mu.Unlock()

	if conn == nil || conn.IsClosed() {
		if err == nil {
			err = errConnectionFailed
		}
		return nil, fmt.Errorf("%w: %v", errConnectionFailed, err)
	}

	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("failed to open channel: %w", err)
	}
	return ch, nil
}

// isHealthy returns true if the connection is alive.
func (cm *connectionManager) isHealthy() bool {
	if cm.closed.Load() {
		return false
	}
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return cm.conn != nil && !cm.conn.IsClosed()
}

// close shuts down the connection and stops the reconnect loop.
func (cm *connectionManager) close() error {
	if !cm.closed.CompareAndSwap(false, true) {
		return nil
	}

	cm.mu.Lock()
	conn := cm.conn
	cm.conn = nil
	cm.mu.Unlock()

	if conn != nil && !conn.IsClosed() {
		if err := conn.Close(); err != nil {
			return err
		}
	}
	return nil
}

// connectionConfig holds reconnect tuning parameters.
type connectionConfig struct {
	reconnectInitial time.Duration
	reconnectMax     time.Duration
	clientProperties map[string]any
	tlsConfig        *tls.Config
	heartbeat        time.Duration
}

func buildClientProperties(properties map[string]any) amqp.Table {
	clientProperties := amqp.NewConnectionProperties()
	clientProperties["product"] = "bore/queue"
	maps.Copy(clientProperties, properties)
	return clientProperties
}

// Sentinel errors
var (
	errConnectionFailed = errors.New("rabbitmq: connection failed")
	errQueueClosed      = errors.New("rabbitmq: queue is closed")
)

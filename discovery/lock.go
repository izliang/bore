package discovery

import (
	"context"
	"fmt"
	"sync"
	"time"

	capi "github.com/hashicorp/consul/api"
	"github.com/izliang/bore/logit"
)

// ConsulLock 基于 Consul Session 的分布式锁。
type ConsulLock struct {
	client        *capi.Client
	sessionID     string
	key           string
	lockDelay     time.Duration
	renewInterval time.Duration
	ttl           time.Duration
	stopChan      chan struct{}
	mu            sync.RWMutex
	isLocked      bool
	logger        logit.Logger
	// OnDestroy 锁被销毁时的回调（可选）。
	OnDestroy func()
}

// NewConsulLock 创建分布式锁。
//
// logger 可为 nil，此时不输出日志。
func NewConsulLock(client *capi.Client, cfg ConsulLockConfig, logger logit.Logger) *ConsulLock {
	cfg.defaults()
	return &ConsulLock{
		client:        client,
		key:           cfg.Key,
		lockDelay:     cfg.LockDelay,
		renewInterval: cfg.RenewInterval,
		ttl:           cfg.TTL,
		stopChan:      make(chan struct{}),
		logger:        logger,
	}
}

// Lock 阻塞获取锁，直到成功或 ctx 取消。
func (l *ConsulLock) Lock(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.isLocked {
		return fmt.Errorf("discovery: lock is already held")
	}

	session, err := l.createSession(ctx)
	if err != nil {
		return fmt.Errorf("discovery: create session: %w", err)
	}
	l.sessionID = session

	for {
		select {
		case <-ctx.Done():
			l.cleanup()
			return ctx.Err()
		default:
			acquired, err := l.acquireLock(ctx)
			if err != nil {
				l.cleanup()
				return fmt.Errorf("discovery: acquire lock: %w", err)
			}
			if acquired {
				l.isLocked = true
				go l.autoRenew()
				return nil
			}
			time.Sleep(l.lockDelay)
		}
	}
}

// TryLock 非阻塞尝试获取锁。
func (l *ConsulLock) TryLock(ctx context.Context) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.isLocked {
		return false, fmt.Errorf("discovery: lock is already held")
	}

	session, err := l.createSession(ctx)
	if err != nil {
		return false, fmt.Errorf("discovery: create session: %w", err)
	}
	l.sessionID = session

	acquired, err := l.acquireLock(ctx)
	if err != nil {
		l.cleanup()
		return false, fmt.Errorf("discovery: acquire lock: %w", err)
	}
	if acquired {
		l.isLocked = true
		go l.autoRenew()
		return true, nil
	}

	l.cleanup()
	return false, nil
}

// LockWithTimeout 带超时的阻塞获取锁。
func (l *ConsulLock) LockWithTimeout(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return l.Lock(ctx)
}

// Unlock 释放锁。
func (l *ConsulLock) Unlock() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.isLocked {
		return fmt.Errorf("discovery: lock is not held")
	}

	close(l.stopChan)
	l.stopChan = make(chan struct{})

	if err := l.releaseLock(); err != nil {
		return fmt.Errorf("discovery: release lock: %w", err)
	}

	l.isLocked = false
	return nil
}

// IsLocked 返回是否已获取锁。
func (l *ConsulLock) IsLocked() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.isLocked
}

func (l *ConsulLock) createSession(ctx context.Context) (string, error) {
	session := &capi.SessionEntry{
		TTL:      l.ttl.String(),
		Behavior: capi.SessionBehaviorDelete,
	}
	sessionID, _, err := l.client.Session().Create(session, nil)
	if err != nil {
		return "", err
	}
	return sessionID, nil
}

func (l *ConsulLock) acquireLock(ctx context.Context) (bool, error) {
	kv := &capi.KVPair{
		Key:     l.key,
		Value:   []byte(l.sessionID),
		Session: l.sessionID,
	}
	acquired, _, err := l.client.KV().Acquire(kv, nil)
	return acquired, err
}

func (l *ConsulLock) releaseLock() error {
	kv := &capi.KVPair{
		Key:     l.key,
		Session: l.sessionID,
	}
	_, _, err := l.client.KV().Release(kv, nil)
	if err != nil {
		return err
	}
	_, err = l.client.Session().Destroy(l.sessionID, nil)
	return err
}

func (l *ConsulLock) autoRenew() {
	ticker := time.NewTicker(l.renewInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			l.mu.RLock()
			if !l.isLocked {
				l.mu.RUnlock()
				return
			}
			sessionID := l.sessionID
			l.mu.RUnlock()

			kvPair, _, err := l.client.KV().Get(l.key, nil)
			if err != nil || kvPair == nil {
				l.mu.Lock()
				l.isLocked = false
				l.mu.Unlock()
				l.warnf("autoRenew key not found, destroy lock")
				l.fireDestroy()
				return
			}

			_, _, err = l.client.Session().Renew(sessionID, nil)
			if err != nil {
				l.mu.Lock()
				l.isLocked = false
				l.mu.Unlock()
				l.warnf("autoRenew session renew failed, destroy lock")
				l.fireDestroy()
				return
			}
		case <-l.stopChan:
			return
		}
	}
}

func (l *ConsulLock) cleanup() {
	if l.sessionID != "" {
		l.client.Session().Destroy(l.sessionID, nil)
		l.sessionID = ""
	}
}

func (l *ConsulLock) fireDestroy() {
	if l.OnDestroy != nil {
		l.OnDestroy()
	}
}

func (l *ConsulLock) warnf(format string, args ...interface{}) {
	if l.logger != nil {
		l.logger.Warn(context.Background(), fmt.Sprintf(format, args...))
	}
}

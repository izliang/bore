package config

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/izliang/bore/cfgstore"
	"github.com/izliang/bore/logit"
)

const ParamKey = "QUEUE_CONCURRENCY_CONTROLLER_MAP"

// QueueConcurrencyController 队列并发度控制器
// 从 cfgstore.Config 中读取各队列的并发度配置，并支持 Watch 变更。
//
// 内部采用 fan-out 模式：全局订阅 ParamKey（JSON map），解析后 diff，分发给各队列的 watcher。
type QueueConcurrencyController struct {
	value    map[string]int
	config   *cfgstore.Config
	logger   logit.Logger
	mu       sync.RWMutex
	watchers map[string][]chan int // queueName → watcher channels

	eventCancel func()
}

// NewQueueConcurrencyController 创建队列并发度控制器
func NewQueueConcurrencyController(config *cfgstore.Config, logger logit.Logger) (*QueueConcurrencyController, error) {
	type Data map[string]int
	data, ok, err := cfgstore.GetJSON[Data](config, ParamKey)
	if err != nil {
		return nil, fmt.Errorf("newQueueConcurrencyController get data err: %w", err)
	}
	if !ok {
		data = make(map[string]int)
	}

	qc := &QueueConcurrencyController{
		value:    data,
		config:   config,
		logger:   logger,
		watchers: make(map[string][]chan int),
	}

	eventCh, eventCancel := config.Subscribe(ParamKey)
	qc.eventCancel = eventCancel
	go qc.run(eventCh)

	return qc, nil
}

// Resolve 获取指定队列的并发度
func (q *QueueConcurrencyController) Resolve(queueName string) int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	v, ok := q.value[queueName]
	if !ok {
		return 0
	}
	return v
}

// Watch 订阅指定队列的并发度变更
// 返回变更 channel 和取消订阅函数
func (q *QueueConcurrencyController) Watch(queueName string) (<-chan int, func()) {
	ch := make(chan int, 1)

	q.mu.Lock()
	q.watchers[queueName] = append(q.watchers[queueName], ch)
	q.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			q.mu.Lock()
			defer q.mu.Unlock()
			chs := q.watchers[queueName]
			for i, c := range chs {
				if c == ch {
					q.watchers[queueName] = append(chs[:i], chs[i+1:]...)
					break
				}
			}
			close(ch)
		})
	}

	return ch, cancel
}

// Close 停止控制器，释放订阅资源
func (q *QueueConcurrencyController) Close() {
	q.eventCancel()
}

// run 后台消费 cfgstore 的 ChangeEvent，解析 JSON 并分发给各队列 watcher
func (q *QueueConcurrencyController) run(eventCh <-chan cfgstore.ChangeEvent) {
	defer func() {
		q.mu.Lock()
		defer q.mu.Unlock()
		for _, chs := range q.watchers {
			for _, ch := range chs {
				close(ch)
			}
		}
		q.watchers = nil
	}()

	for event := range eventCh {
		var newValue map[string]int
		switch event.Action {
		case cfgstore.ActionDelete:
			newValue = make(map[string]int)
		default: // ActionInit, ActionAdd, ActionUpdate
			if err := json.Unmarshal([]byte(event.NewValue), &newValue); err != nil {
				q.logger.Error(context.Background(),
					fmt.Errorf("queue concurrency controller unmarshal failed: %w", err),
					logit.StringField("key", event.Key))
				continue
			}
		}

		q.mu.Lock()
		// 检测新增 / 变更
		for k, newV := range newValue {
			oldV, existed := q.value[k]
			if !existed || oldV != newV {
				q.value[k] = newV
				q.logger.Info(context.Background(), "queue concurrency changed",
					logit.StringField("queue", k),
					logit.IntField("old", oldV),
					logit.IntField("new", newV))
				q.notifyLocked(k, newV)
			}
		}
		// 检测删除的队列
		for k, oldV := range q.value {
			if _, exists := newValue[k]; !exists {
				delete(q.value, k)
				q.logger.Info(context.Background(), "queue concurrency removed",
					logit.StringField("queue", k),
					logit.IntField("old", oldV))
				q.notifyLocked(k, 0)
			}
		}
		q.mu.Unlock()
	}
}

// notifyLocked 非阻塞通知指定队列的所有 watcher（调用方需持有 mu）
// 采用 drain-and-replace 策略：先排空 channel，再写入最新值，保证最终一致性
func (q *QueueConcurrencyController) notifyLocked(queueName string, v int) {
	for _, ch := range q.watchers[queueName] {
		// drain
		select {
		case <-ch:
		default:
		}
		// replace
		select {
		case ch <- v:
		default:
		}
	}
}

package config

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/izliang/bore/cfgstore"
	"github.com/izliang/bore/logit"
)

var testLogger = logit.NewSimple(os.Stdout)

func setupMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)

	gormDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      db,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	require.NoError(t, err)

	return gormDB, mock, func() { db.Close() }
}

// paramsRow 构建 params_config 表的 sqlmock 行
func paramsRow(name, value string, isDelete int, updateTime time.Time) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"name", "value", "is_delete", "update_time"}).
		AddRow(name, value, isDelete, updateTime)
}

// expectLoadAll 期望一次全量加载
func expectLoadAll(mock sqlmock.Sqlmock, rows *sqlmock.Rows) {
	mock.ExpectQuery("SELECT .*name.*value.*is_delete.*update_time.*FROM .*params_config").
		WillReturnRows(rows)
}

// expectLoadSince 期望一次增量加载
func expectLoadSince(mock sqlmock.Sqlmock, rows *sqlmock.Rows) {
	mock.ExpectQuery("SELECT .*name.*value.*is_delete.*update_time.*FROM .*params_config.*WHERE update_time > ?").
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(rows)
}

// assertRecv 从 channel 读取，超时则失败
func assertRecv(t *testing.T, ch <-chan int, want int, timeout time.Duration) {
	t.Helper()
	select {
	case v, ok := <-ch:
		if !ok {
			t.Fatalf("channel closed, want %d", want)
		}
		assert.Equal(t, want, v, "unexpected channel value")
	case <-time.After(timeout):
		t.Fatalf("timeout (%v) waiting for channel value %d", timeout, want)
	}
}

// assertNoRecv 断言 channel 在给定时间内没有新值
func assertNoRecv(t *testing.T, ch <-chan int, timeout time.Duration) {
	t.Helper()
	select {
	case v, ok := <-ch:
		if !ok {
			return // closed channel, no new values
		}
		t.Fatalf("unexpected channel value: %d", v)
	case <-time.After(timeout):
	}
}

// assertClosed 断言 channel 已关闭
func assertClosed(t *testing.T, ch <-chan int) {
	t.Helper()
	select {
	case _, ok := <-ch:
		assert.False(t, ok, "channel should be closed")
	default:
		t.Fatal("channel should be closed but is still open/empty")
	}
}

// ---------------------------------------------------------------------------
// E2E: 端到端测试 — 从数据库变更 → cfgstore 通知 → 队列并发度通知
// ---------------------------------------------------------------------------
func TestQueueConcurrencyController_E2E(t *testing.T) {
	gormDB, mock, cleanup := setupMockDB(t)
	defer cleanup()

	ctx := context.Background()
	loader := cfgstore.NewGormLoader(gormDB)

	t0 := time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)
	t2 := t1.Add(time.Hour)
	t3 := t2.Add(time.Hour)
	t4 := t3.Add(time.Hour)
	t5 := t4.Add(time.Hour)

	// ========================================================================
	// Phase 0: 系统初始化 — LoadAll 加载初始配置
	// ========================================================================
	expectLoadAll(mock, paramsRow(ParamKey, `{"queue_a":5,"queue_b":3}`, 0, t0))

	cfg, err := cfgstore.New(ctx, loader, testLogger)
	require.NoError(t, err)
	defer cfg.Stop()

	qc, err := NewQueueConcurrencyController(cfg, testLogger)
	require.NoError(t, err)
	defer qc.Close()

	t.Run("initial_resolve", func(t *testing.T) {
		assert.Equal(t, 5, qc.Resolve("queue_a"))
		assert.Equal(t, 3, qc.Resolve("queue_b"))
		assert.Equal(t, 0, qc.Resolve("nonexistent"))
	})

	// ========================================================================
	// Phase 1: 值变更 — queue_a: 5→10, queue_b 不变
	// ========================================================================
	chA, cancelA := qc.Watch("queue_a")
	defer cancelA()
	chB, cancelB := qc.Watch("queue_b")
	defer cancelB()

	expectLoadSince(mock, paramsRow(ParamKey, `{"queue_a":10,"queue_b":3}`, 0, t1))
	require.NoError(t, cfg.Refresh(ctx))

	t.Run("value_changed", func(t *testing.T) {
		assertRecv(t, chA, 10, time.Second)
		assertNoRecv(t, chB, 200*time.Millisecond)
		assert.Equal(t, 10, qc.Resolve("queue_a"))
		assert.Equal(t, 3, qc.Resolve("queue_b"))
	})

	// ========================================================================
	// Phase 2: 新增队列 + 变更 — queue_b: 3→7, queue_c 新增=2
	// ========================================================================
	chC, cancelC := qc.Watch("queue_c")
	defer cancelC()

	expectLoadSince(mock, paramsRow(ParamKey, `{"queue_a":10,"queue_b":7,"queue_c":2}`, 0, t2))
	require.NoError(t, cfg.Refresh(ctx))

	t.Run("add_and_update", func(t *testing.T) {
		assertRecv(t, chB, 7, time.Second)
		assertRecv(t, chC, 2, time.Second)
		assertNoRecv(t, chA, 200*time.Millisecond)
		assert.Equal(t, 10, qc.Resolve("queue_a"))
		assert.Equal(t, 7, qc.Resolve("queue_b"))
		assert.Equal(t, 2, qc.Resolve("queue_c"))
	})

	// ========================================================================
	// Phase 3: 删除队列 — queue_b 和 queue_c 被移除
	// ========================================================================
	expectLoadSince(mock, paramsRow(ParamKey, `{"queue_a":10}`, 0, t3))
	require.NoError(t, cfg.Refresh(ctx))

	t.Run("queues_removed", func(t *testing.T) {
		assertRecv(t, chB, 0, time.Second)
		assertRecv(t, chC, 0, time.Second)
		assertNoRecv(t, chA, 200*time.Millisecond)
		assert.Equal(t, 10, qc.Resolve("queue_a"))
		assert.Equal(t, 0, qc.Resolve("queue_b"))
		assert.Equal(t, 0, qc.Resolve("queue_c"))
	})

	// ========================================================================
	// Phase 4: 配置键被标记删除 — 所有队列并发度归零
	// ========================================================================
	expectLoadSince(mock, paramsRow(ParamKey, "", 1, t4))
	require.NoError(t, cfg.Refresh(ctx))

	t.Run("config_key_deleted", func(t *testing.T) {
		assertRecv(t, chA, 0, time.Second)
		assert.Equal(t, 0, qc.Resolve("queue_a"))
	})

	// ========================================================================
	// Phase 5: 取消订阅 — cancelA 后 queue_a 不再收到事件，channel 关闭
	// ========================================================================
	cancelA()

	t.Run("cancel_closes_channel", func(t *testing.T) {
		assertClosed(t, chA)
	})

	// 再推送一次变更，验证 chA 不收到值，但 chB/chC 不受影响
	expectLoadSince(mock, paramsRow(ParamKey, `{"queue_a":99,"queue_b":1}`, 0, t5))
	require.NoError(t, cfg.Refresh(ctx))

	t.Run("cancelled_no_events", func(t *testing.T) {
		assertNoRecv(t, chA, 200*time.Millisecond)
		// chB 重新收到了值
		assertRecv(t, chB, 1, time.Second)
		// Resolve 仍可读取最新值
		assert.Equal(t, 99, qc.Resolve("queue_a"))
		assert.Equal(t, 1, qc.Resolve("queue_b"))
	})

	// ========================================================================
	// 验证所有 mock 期望均已消费
	// ========================================================================
	require.NoError(t, mock.ExpectationsWereMet())
}

// ---------------------------------------------------------------------------
// 多 watcher 并发场景：同一队列注册多个 watcher，都应收到通知
// ---------------------------------------------------------------------------
func TestQueueConcurrencyController_MultipleWatchers(t *testing.T) {
	gormDB, mock, cleanup := setupMockDB(t)
	defer cleanup()

	ctx := context.Background()
	loader := cfgstore.NewGormLoader(gormDB)

	t0 := time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)

	expectLoadAll(mock, paramsRow(ParamKey, `{"qa":3}`, 0, t0))

	cfg, err := cfgstore.New(ctx, loader, testLogger)
	require.NoError(t, err)
	defer cfg.Stop()

	qc, err := NewQueueConcurrencyController(cfg, testLogger)
	require.NoError(t, err)
	defer qc.Close()

	// 同一队列注册 3 个 watcher
	ch1, c1 := qc.Watch("qa")
	defer c1()
	ch2, c2 := qc.Watch("qa")
	defer c2()
	ch3, c3 := qc.Watch("qa")
	defer c3()

	// 触发变更
	expectLoadSince(mock, paramsRow(ParamKey, `{"qa":8}`, 0, t1))
	require.NoError(t, cfg.Refresh(ctx))

	// 三个 watcher 都应收到 8
	assertRecv(t, ch1, 8, time.Second)
	assertRecv(t, ch2, 8, time.Second)
	assertRecv(t, ch3, 8, time.Second)

	require.NoError(t, mock.ExpectationsWereMet())
}

// ---------------------------------------------------------------------------
// JSON 解析失败时不应 panic，watcher 不受影响，旧值保留
// ---------------------------------------------------------------------------
func TestQueueConcurrencyController_InvalidJSON(t *testing.T) {
	gormDB, mock, cleanup := setupMockDB(t)
	defer cleanup()

	ctx := context.Background()
	loader := cfgstore.NewGormLoader(gormDB)

	t0 := time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)

	expectLoadAll(mock, paramsRow(ParamKey, `{"qa":1}`, 0, t0))

	cfg, err := cfgstore.New(ctx, loader, testLogger)
	require.NoError(t, err)
	defer cfg.Stop()

	qc, err := NewQueueConcurrencyController(cfg, testLogger)
	require.NoError(t, err)
	defer qc.Close()

	ch, cancel := qc.Watch("qa")
	defer cancel()

	// 推送非法 JSON
	expectLoadSince(mock, paramsRow(ParamKey, `not-json`, 0, t1))
	require.NoError(t, cfg.Refresh(ctx))

	// watcher 不应收到任何值（JSON 解析失败被跳过）
	assertNoRecv(t, ch, 200*time.Millisecond)

	// Resolve 仍返回旧值
	assert.Equal(t, 1, qc.Resolve("qa"))

	require.NoError(t, mock.ExpectationsWereMet())
}

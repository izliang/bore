package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// newMockDB returns a *gorm.DB backed by sqlmock for testing Close/Ping/Metrics.
func newMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	gormDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	require.NoError(t, err)

	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Logf("close mock: %v", err)
		}
	})
	return gormDB, mock
}

// ---- Config ----

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, "mysql", cfg.Driver)
	assert.Equal(t, 10, cfg.MaxIdleConns)
	assert.Equal(t, 100, cfg.MaxOpenConns)
	assert.Equal(t, time.Hour, cfg.ConnMaxLifetime)
	assert.Equal(t, 5*time.Minute, cfg.ConnMaxIdleTime)
	assert.Equal(t, LogWarn, cfg.LogMode)
	assert.Equal(t, 200*time.Millisecond, *cfg.SlowThreshold)
	assert.True(t, cfg.SingularTable)
	assert.Equal(t, 3, cfg.MaxRetries)
	assert.Equal(t, 100*time.Millisecond, cfg.RetryBaseDelay)
	assert.Equal(t, 30*time.Second, cfg.RetryMaxDelay)
}

func TestConfig_normalize_ZeroFields(t *testing.T) {
	cfg := &Config{}
	cfg.normalize()

	assert.Equal(t, "mysql", cfg.Driver)
	assert.Equal(t, 10, cfg.MaxIdleConns)
	assert.Equal(t, 100, cfg.MaxOpenConns)
	assert.Equal(t, time.Hour, cfg.ConnMaxLifetime)
	assert.Equal(t, 5*time.Minute, cfg.ConnMaxIdleTime)
	if assert.NotNil(t, cfg.SlowThreshold) {
		assert.Equal(t, 200*time.Millisecond, *cfg.SlowThreshold)
	}
	assert.Equal(t, 3, cfg.MaxRetries)
	assert.NotNil(t, cfg.Logger)
}

func TestConfig_normalize_PreservesSetFields(t *testing.T) {
	// When the user explicitly sets SlowThreshold to a non-nil zero pointer,
	// normalize respects it and does not overwrite with the default.
	zero := 0 * time.Millisecond
	cfg := &Config{
		Driver:         "sqlite",
		MaxIdleConns:   5,
		MaxOpenConns:   50,
		LogMode:        LogSilent,
		SlowThreshold:  &zero,
		RetryBaseDelay: 500 * time.Millisecond,
	}
	cfg.normalize()

	assert.Equal(t, "sqlite", cfg.Driver)
	assert.Equal(t, 5, cfg.MaxIdleConns)
	assert.Equal(t, 50, cfg.MaxOpenConns)
	assert.Equal(t, LogSilent, cfg.LogMode)
	if assert.NotNil(t, cfg.SlowThreshold) {
		assert.Equal(t, time.Duration(0), *cfg.SlowThreshold)
	}
}

// ---- Open ----

func TestOpenContext_MissingDSN(t *testing.T) {
	db, err := OpenContext(context.Background(), &Config{})
	assert.Nil(t, db)
	assert.ErrorIs(t, err, ErrDSNRequired)
}

func TestOpenContext_NilConfig(t *testing.T) {
	// nil config → defaults applied, DSN still empty → ErrDSNRequired.
	db, err := OpenContext(context.Background(), nil)
	assert.Nil(t, db)
	assert.ErrorIs(t, err, ErrDSNRequired)
}

func TestOpenContext_UnsupportedDriver(t *testing.T) {
	cfg := &Config{
		Driver: "mongodb",
		DSN:    "irrelevant",
	}
	db, err := OpenContext(context.Background(), cfg)
	assert.Nil(t, db)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported driver")
}

func TestOpenContext_ContextCancelled(t *testing.T) {
	cfg := &Config{
		DSN:             "user:pass@tcp(127.0.0.1:3306)/db?charset=utf8mb4&parseTime=True",
		MaxRetries:      5,
		RetryBaseDelay:  500 * time.Millisecond,
		ConnMaxLifetime: time.Hour,
		ConnMaxIdleTime: 5 * time.Minute,
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before entering OpenContext

	start := time.Now()
	db, err := OpenContext(ctx, cfg)
	elapsed := time.Since(start)

	assert.Nil(t, db)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "retry cancelled")
	assert.Less(t, elapsed, 200*time.Millisecond,
		"cancelled context should return quickly, took %v", elapsed)
}

// ---- Close ----

func TestClose_Nil(t *testing.T) {
	assert.NoError(t, Close(nil))
}

func TestClose_Success(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectClose()
	assert.NoError(t, Close(db))
}

// ---- Ping ----

func TestPing_NilDB(t *testing.T) {
	err := Ping(context.Background(), nil)
	assert.ErrorIs(t, err, ErrClosed)
}

func TestPing_Success(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectPing()

	err := Ping(context.Background(), db)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPing_ContextCancelled(t *testing.T) {
	db, _ := newMockDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := Ping(ctx, db)
	assert.Error(t, err)
}

// ---- MustOpen ----

func TestMustOpen_Panics(t *testing.T) {
	assert.Panics(t, func() {
		_ = MustOpen(&Config{}) // DSN missing → error → panic
	})
}

// ---- Sentinel errors ----

func TestSentinelErrors(t *testing.T) {
	assert.EqualError(t, ErrDSNRequired, "db: DSN is required")
	assert.EqualError(t, ErrClosed, "db: database connection is closed")
}

// ---- Retry logic ----

func TestRetry_AllExhausted(t *testing.T) {
	callCount := 0
	err := retry(context.Background(), 2, time.Millisecond, 10*time.Millisecond, func() error {
		callCount++
		return errors.New("transient")
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "all 3 retries exhausted")
	assert.Equal(t, 3, callCount)
}

func TestRetry_SuccessOnSecondAttempt(t *testing.T) {
	callCount := 0
	err := retry(context.Background(), 3, time.Millisecond, 10*time.Millisecond, func() error {
		callCount++
		if callCount == 1 {
			return errors.New("transient")
		}
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 2, callCount)
}

func TestRetry_NoRetries(t *testing.T) {
	callCount := 0
	err := retry(context.Background(), 0, time.Millisecond, 10*time.Millisecond, func() error {
		callCount++
		return errors.New("fail")
	})
	assert.Error(t, err)
	assert.Equal(t, 1, callCount)
}

func TestRetry_ContextDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()

	callCount := 0
	err := retry(ctx, 10, 500*time.Millisecond, time.Second, func() error {
		callCount++
		return errors.New("transient")
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "retry cancelled")
	assert.Equal(t, 1, callCount)
}

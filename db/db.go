package db

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/izliang/bore/logit"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

var gormLogLevels = map[LogMode]logger.LogLevel{
	LogSilent: logger.Silent,
	LogError:  logger.Error,
	LogWarn:   logger.Warn,
	LogInfo:   logger.Info,
}

// Open creates a database connection with the given Config.
// It retries the initial dial according to Config.MaxRetries with exponential
// backoff, configures the connection pool, sets up GORM logging via
// logit.GormAdapter, and registers any plugins.
//
// Open uses context.Background() for the retry deadline. Prefer OpenContext when
// the caller has a context with a deadline or cancellation signal.
func Open(cfg *Config) (*gorm.DB, error) {
	return OpenContext(context.Background(), cfg)
}

// OpenContext is like Open but accepts a context for retry cancellation.
// When ctx is cancelled or expires, the retry loop stops immediately and the
// function returns ctx.Err().
func OpenContext(ctx context.Context, cfg *Config) (*gorm.DB, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	} else {
		// Shallow copy so normalize doesn't mutate the caller's struct.
		clone := *cfg
		cfg = &clone
	}
	cfg.normalize()

	if cfg.DSN == "" {
		return nil, ErrDSNRequired
	}

	// Build GORM logger adapter.
	gormLogger := buildGormLogger(cfg)

	dialector, err := buildDialector(cfg)
	if err != nil {
		return nil, err
	}

	gormCfg := &gorm.Config{
		Logger: gormLogger,
		NamingStrategy: schema.NamingStrategy{
			SingularTable: cfg.SingularTable,
		},
	}

	var db *gorm.DB

	// Retry loop.
	err = retry(ctx, cfg.MaxRetries, cfg.RetryBaseDelay, cfg.RetryMaxDelay, func() error {
		var dialErr error
		db, dialErr = gorm.Open(dialector, gormCfg)
		return dialErr
	})
	if err != nil {
		return nil, fmt.Errorf("db: open: %w", err)
	}

	// Configure connection pool.
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("db: get sql.DB: %w", err)
	}
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	// Register custom plugins.
	for _, p := range cfg.Plugins {
		if err := db.Use(p); err != nil {
			return nil, fmt.Errorf("db: register plugin: %w", err)
		}
	}

	return db, nil
}

// MustOpen is like Open but panics on error. Intended for initialisation code
// where a missing database is a hard failure.
func MustOpen(cfg *Config) *gorm.DB {
	db, err := Open(cfg)
	if err != nil {
		panic(fmt.Sprintf("db: MustOpen: %v", err))
	}
	return db
}

// Close closes the underlying sql.DB connections. Returns nil when db is nil
// (idempotent for convenience in defer), or an error if the close fails.
func Close(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("db: close: %w", err)
	}
	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("db: close: %w", err)
	}
	return nil
}

// Ping verifies the database connection is alive. It returns ErrClosed when db
// is nil and propagates sql.DB.Ping errors otherwise.
func Ping(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return ErrClosed
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("db: ping: %w", err)
	}
	return sqlDB.PingContext(ctx)
}

// buildGormLogger constructs a logit.GormAdapter with the configured log mode
// and slow threshold.
func buildGormLogger(cfg *Config) logger.Interface {
	adapter := logit.NewGormAdapter(cfg.Logger)
	adapter.SlowThreshold = *cfg.SlowThreshold
	adapter.IgnoreRecordNotFoundError = cfg.IgnoreRecordNotFoundError
	return adapter.LogMode(gormLogLevels[cfg.LogMode])
}

// buildDialector returns a GORM Dialector for the configured driver.
func buildDialector(cfg *Config) (gorm.Dialector, error) {
	switch cfg.Driver {
	case "mysql":
		return mysql.Open(cfg.DSN), nil
	default:
		return nil, fmt.Errorf("db: unsupported driver %q", cfg.Driver)
	}
}

// retry executes fn up to maxRetries times with exponential backoff.
// Returns the last error if all attempts fail. Respects ctx cancellation.
func retry(ctx context.Context, maxRetries int, base, max time.Duration, fn func() error) error {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
		}

		if attempt == maxRetries {
			break
		}

		// Exponential backoff: base * 2^attempt, capped at max.
		delay := time.Duration(float64(base) * math.Pow(2, float64(attempt)))
		if delay > max {
			delay = max
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("db: retry cancelled: %w", ctx.Err())
		case <-time.After(delay):
		}
	}

	return fmt.Errorf("db: all %d retries exhausted: %w", maxRetries+1, lastErr)
}

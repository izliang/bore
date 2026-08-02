// Package db provides database connection lifecycle management with built-in
// retry, context propagation, structured logging (logit), and Prometheus metrics.
//
// See doc.go for comprehensive usage examples.
package db

import (
	"time"

	"github.com/izliang/bore/logit"
	"gorm.io/gorm"
)

// LogMode mirrors GORM log levels for safe config serialisation.
type LogMode int

const (
	// LogSilent suppresses all GORM logs.
	LogSilent LogMode = iota
	// LogError only logs GORM errors.
	LogError
	// LogWarn logs GORM errors + slow queries.
	LogWarn
	// LogInfo logs all SQL (verbose, not recommended for production).
	LogInfo
)

// Config holds all database connection parameters.
//
// Zero-value fields receive sensible defaults during Open, so callers may set
// only the fields they need and leave the rest at zero.
type Config struct {
	// Driver is the GORM dialect: "mysql", "postgres", "sqlite", "sqlserver".
	// When empty, Open defaults to "mysql".
	Driver string

	// DSN is the database source name. Required.
	DSN string

	// --- connection pool ---

	// MaxIdleConns controls the maximum number of idle connections.
	// Default: 10.
	MaxIdleConns int
	// MaxOpenConns controls the maximum number of open connections.
	// Default: 100.
	MaxOpenConns int
	// ConnMaxLifetime is the maximum amount of time a connection may be reused.
	// Default: 1 hour.
	ConnMaxLifetime time.Duration
	// ConnMaxIdleTime is the maximum amount of time an idle connection is kept.
	// Default: 5 minutes.
	ConnMaxIdleTime time.Duration

	// --- logging ---

	// LogMode controls the GORM log verbosity via logit.GormAdapter.
	// Default: LogWarn.
	LogMode LogMode
	// SlowThreshold is the duration after which a SQL query is logged as slow.
	// When nil, the default (200ms) is used.
	// When non-nil and zero, slow query detection is disabled.
	SlowThreshold *time.Duration
	// Logger is the logit.Logger used for GORM log output.
	// When nil, Open defaults to logit.NewSimple(nil) which discards output.
	Logger logit.Logger
	// IgnoreRecordNotFoundError when true suppresses gorm.ErrRecordNotFound from
	// being logged as an error. Same semantic as gorm.Config.IgnoreRecordNotFoundError.
	IgnoreRecordNotFoundError bool

	// --- naming ---

	// SingularTable disables GORM's default table-name pluralisation.
	// Default: true.
	SingularTable bool

	// --- retry ---

	// MaxRetries controls how many times OpenContext retries the initial dial.
	// A value of 0 means no retries (try once).
	// Default: 3.
	MaxRetries int
	// RetryBaseDelay is the initial backoff duration.
	// Default: 100ms.
	RetryBaseDelay time.Duration
	// RetryMaxDelay caps the exponential backoff.
	// Default: 30s.
	RetryMaxDelay time.Duration

	// --- plugins ---

	// Plugins are GORM plugins registered after the connection is established
	// (e.g. gorm.io/plugin/opentelemetry/tracing).
	Plugins []gorm.Plugin
}

// DefaultConfig returns a Config with sensible production defaults.
func DefaultConfig() *Config {
	slow := 200 * time.Millisecond
	return &Config{
		Driver:          "mysql",
		MaxIdleConns:    10,
		MaxOpenConns:    100,
		ConnMaxLifetime: time.Hour,
		ConnMaxIdleTime: 5 * time.Minute,
		LogMode:         LogWarn,
		SlowThreshold:   &slow,
		SingularTable:   true,
		MaxRetries:      3,
		RetryBaseDelay:  100 * time.Millisecond,
		RetryMaxDelay:   30 * time.Second,
	}
}

// normalize fills zero-value fields with defaults so the rest of the package
// never branches on "is this field zero?".
func (c *Config) normalize() {
	if c.Driver == "" {
		c.Driver = "mysql"
	}
	if c.MaxIdleConns == 0 {
		c.MaxIdleConns = 10
	}
	if c.MaxOpenConns == 0 {
		c.MaxOpenConns = 100
	}
	if c.ConnMaxLifetime == 0 {
		c.ConnMaxLifetime = time.Hour
	}
	if c.ConnMaxIdleTime == 0 {
		c.ConnMaxIdleTime = 5 * time.Minute
	}
	if c.SlowThreshold == nil {
		d := 200 * time.Millisecond
		c.SlowThreshold = &d
	}
	if c.MaxRetries == 0 && c.RetryBaseDelay == 0 && c.RetryMaxDelay == 0 {
		// The user did not set any retry field – apply defaults.
		c.MaxRetries = 3
		c.RetryBaseDelay = 100 * time.Millisecond
		c.RetryMaxDelay = 30 * time.Second
	}
	if c.Logger == nil {
		c.Logger = logit.NewSimple(nil) // discards output
	}
}

package chlog

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/izliang/bore/ck"
	"github.com/izliang/bore/logit"
)

// Logger implements logit.Logger by writing structured rows to ClickHouse
// and/or console, depending on the configured Mode.
type Logger struct {
	cfg     *Config
	batcher *batcher // nil in ModeConsole
	stdout  io.Writer
	onError func(error)

	level     logit.Level
	levelMu   sync.RWMutex
	closeOnce sync.Once
	seq       atomic.Uint64
}

// New creates a new chlog Logger. Call Close() when done.
//
// After creating the logger, call Init() to create the database and table
// automatically from the Schema defined in Config.
func New(cfg Config) (*Logger, error) {
	if err := cfg.normalize(); err != nil {
		return nil, err
	}

	l := &Logger{
		cfg:     &cfg,
		stdout:  cfg.Stdout,
		onError: cfg.OnError,
		level:   cfg.Level,
	}

	if cfg.usesCK() {
		l.batcher = newBatcher(cfg.DB, cfg.Database, cfg.Table, cfg.BatchSize, cfg.FlushInterval, cfg.OnError)
	}

	return l, nil
}

// Init creates the database and table in ClickHouse using the Schema from Config.
// Uses CREATE IF NOT EXISTS — safe to call multiple times.
//
// Only meaningful for ModeBoth and ModeCK; no-op for ModeConsole or when
// Schema is nil.
func (l *Logger) Init(ctx context.Context) error {
	if !l.cfg.usesCK() || l.cfg.Schema == nil {
		return nil
	}

	createDB := ck.CreateDatabaseSQL(l.cfg.Database)
	createTable := ck.CreateTableSQL(*l.cfg.Schema)

	if l.stdout != nil && l.stdout != io.Discard {
		fmt.Fprintf(l.stdout, "\n[chlog] init: %s\n[chlog] init: %s\n\n", createDB, createTable)
	}

	if _, err := l.cfg.DB.ExecContext(ctx, createDB); err != nil {
		return fmt.Errorf("chlog: create database %q: %w", l.cfg.Database, err)
	}

	if _, err := l.cfg.DB.ExecContext(ctx, createTable); err != nil {
		return fmt.Errorf("chlog: create table %q.%q: %w", l.cfg.Database, l.cfg.Table, err)
	}

	return nil
}

// Close flushes remaining CK rows (if any) and stops the batcher.
// No-op in ModeConsole.
func (l *Logger) Close() {
	l.closeOnce.Do(func() {
		if l.batcher != nil {
			l.batcher.close()
		}
	})
}

// Write implements io.Writer. It writes the raw bytes to both CK (if configured)
// and stdout.
func (l *Logger) Write(p []byte) (n int, err error) {
	msg := strings.TrimRight(string(p), "\n\r")

	if l.batcher != nil {
		r := buildRow(l.cfg, logit.Info, msg, nil, nil, l.seq.Add(1)-1)
		l.batcher.push(r)
	}
	if l.stdout != nil {
		l.stdout.Write(p)
	}
	return len(p), nil
}

// Trace logs at trace level.
func (l *Logger) Trace(ctx context.Context, msg string, fields ...logit.FieldFunc) {
	l.log(logit.Trace, ctx, msg, nil, fields...)
}

// Info logs at info level.
func (l *Logger) Info(ctx context.Context, msg string, fields ...logit.FieldFunc) {
	l.log(logit.Info, ctx, msg, nil, fields...)
}

// Debug logs at debug level.
func (l *Logger) Debug(ctx context.Context, msg string, fields ...logit.FieldFunc) {
	l.log(logit.Debug, ctx, msg, nil, fields...)
}

// Warn logs at warn level.
func (l *Logger) Warn(ctx context.Context, msg string, fields ...logit.FieldFunc) {
	l.log(logit.Warn, ctx, msg, nil, fields...)
}

// Error logs an error. If err is nil, nothing is logged.
func (l *Logger) Error(ctx context.Context, err error, fields ...logit.FieldFunc) {
	if err == nil {
		return
	}
	l.log(logit.Error, ctx, "", err, fields...)
}

func (l *Logger) log(level logit.Level, ctx context.Context, msg string, err error, fields ...logit.FieldFunc) {
	if !l.shouldLog(level) {
		return
	}

	if err != nil {
		fields = append([]logit.FieldFunc{logit.ErrorField("error", err)}, fields...)
	}

	// Build the row (used for both CK and console output).
	r := buildRow(l.cfg, level, msg, ctx, fields, l.seq.Add(1)-1)

	// Push to CK batcher if configured.
	if l.batcher != nil {
		l.batcher.push(r)
	}

	// Write formatted line to console.
	if l.stdout != nil && l.stdout != io.Discard {
		line := buildRaw(time.Now().In(l.cfg.Location), level, msg, ctx, fields)
		fmt.Fprintln(l.stdout, line)
	}
}

func (l *Logger) shouldLog(level logit.Level) bool {
	l.levelMu.RLock()
	defer l.levelMu.RUnlock()

	rank := map[logit.Level]int{
		logit.Trace: 0,
		logit.Debug: 10,
		logit.Info:  20,
		logit.Warn:  50,
		logit.Error: 100,
	}

	lv, ok := rank[level]
	if !ok {
		lv = rank[logit.Info]
	}
	minLv, ok := rank[l.level]
	if !ok {
		minLv = rank[logit.Info]
	}
	return lv >= minLv
}

// withLevel returns a logger scoped to the given minimum level.
func (l *Logger) withLevel(level logit.Level) logit.Logger {
	cp := *l
	cp.level = level
	return &cp
}

// Ensure Logger implements logit.Logger.
var _ logit.Logger = (*Logger)(nil)

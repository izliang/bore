// Package db provides database connection lifecycle management with built-in
// retry, context propagation, structured logging (logit), and Prometheus metrics.
//
// # Quick start
//
//	cfg := db.DefaultConfig()
//	cfg.DSN = "user:pass@tcp(host:3306)/mydb?charset=utf8mb4&parseTime=True"
//	gormDB, err := db.Open(cfg)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer db.Close(gormDB)
//
//	// With context-based cancellation / timeout:
//	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
//	defer cancel()
//	gormDB, err := db.OpenContext(ctx, cfg)
//
//	// Inject into appctx for singleton access:
//	appCtx := appctx.New(appctx.WithGORM(gormDB))
//
// # Connection retry
//
// OpenContext retries the initial dial with exponential backoff governed by
// Config.MaxRetries, Config.RetryBaseDelay, and Config.RetryMaxDelay.
//
// Backoff formula: RetryBaseDelay × 2^attempt, capped at RetryMaxDelay.
// When ctx is cancelled or expires the retry loop exits immediately without
// waiting for the next backoff interval.
//
// # Logging
//
// GORM log output is routed through logit.GormAdapter to a logit.Logger.
// Configuration knobs:
//   - Config.LogMode: verbosity level — LogSilent, LogError, LogWarn, or LogInfo.
//   - Config.SlowThreshold: queries exceeding this duration are logged as slow
//     warnings. Set to a non-nil zero pointer to disable slow-query detection.
//   - Config.Logger: the logit.Logger backend. When nil, output is discarded.
//
// # Metrics
//
// DBStats implements prometheus.Collector and exposes connection-pool statistics:
//   - db_connections_max_open — Max configured open connections
//   - db_connections_open — Established connections (in-use + idle)
//   - db_connections_in_use — Connections currently in use
//   - db_connections_idle — Idle connections
//   - db_wait_count_total — Total number of waits for a new connection
//   - db_wait_duration_seconds_total — Total time blocked waiting for a connection
//   - db_connections_max_idle_closed_total — Connections closed due to SetMaxIdleConns
//   - db_connections_max_lifetime_closed_total — Connections closed due to SetConnMaxLifetime
//
// Standalone registration:
//
//	dbStats := db.NewDBStats(gormDB, db.WithDBName("primary"))
//	prometheus.MustRegister(dbStats)
//
// Registration with bore metrics:
//
//	reg := prometheus.NewRegistry()
//	dbStats := db.NewDBStats(gormDB, db.WithDBName("primary"))
//	metrics.MustRegister(reg, dbStats)
//
// # Sentinel errors
//
//   - ErrDSNRequired — Config.DSN is empty
//   - ErrClosed — operating on a closed or nil *gorm.DB
//
// # End-to-end example
//
//	package main
//
//	import (
//	    "context"
//	    "log"
//	    "os"
//	    "time"
//
//	    "github.com/izliang/bore/db"
//	    "github.com/izliang/bore/logit"
//	    "github.com/prometheus/client_golang/prometheus"
//	    "gorm.io/gorm"
//	)
//
//	type User struct {
//	    gorm.Model
//	    Name  string
//	    Email string
//	}
//
//	func main() {
//	    // 1. Create a log backend (use a zap/slog adapter in production).
//	    logWriter := logit.NewSimple(os.Stdout)
//
//	    // 2. Build configuration.
//	    slow := 100 * time.Millisecond
//	    cfg := &db.Config{
//	        DSN:              "user:pass@tcp(127.0.0.1:3306)/test?charset=utf8mb4&parseTime=True",
//	        MaxIdleConns:     10,
//	        MaxOpenConns:     50,
//	        ConnMaxLifetime:  30 * time.Minute,
//	        ConnMaxIdleTime:  5 * time.Minute,
//	        LogMode:          db.LogWarn,
//	        SlowThreshold:    &slow,
//	        Logger:           logWriter,
//	        SingularTable:    true,
//	        MaxRetries:       3,
//	        RetryBaseDelay:   200 * time.Millisecond,
//	    }
//
//	    // 3. Open with a timeout.
//	    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
//	    defer cancel()
//	    gormDB, err := db.OpenContext(ctx, cfg)
//	    if err != nil {
//	        log.Fatalf("open db: %v", err)
//	    }
//	    defer db.Close(gormDB)
//
//	    // 4. Health check.
//	    if err := db.Ping(context.Background(), gormDB); err != nil {
//	        log.Fatalf("ping db: %v", err)
//	    }
//
//	    // 5. Register metrics.
//	    dbStats := db.NewDBStats(gormDB, db.WithDBName("primary"))
//	    prometheus.MustRegister(dbStats)
//
//	    // 6. Auto-migrate.
//	    _ = gormDB.AutoMigrate(&User{})
//
//	    // 7. Basic CRUD.
//	    user := User{Name: "Alice", Email: "alice@example.com"}
//	    gormDB.Create(&user)
//	    gormDB.First(&user, user.ID)
//	    gormDB.Model(&user).Update("Email", "new@example.com")
//	    gormDB.Delete(&user, user.ID)
//	}
//
// # Multiple databases
//
//	cfg1 := db.DefaultConfig()
//	cfg1.DSN = "user:pass@tcp(host1:3306)/db1?charset=utf8mb4&parseTime=True"
//	primaryDB, _ := db.Open(cfg1)
//
//	cfg2 := db.DefaultConfig()
//	cfg2.DSN = "user:pass@tcp(host2:3306)/db2?charset=utf8mb4&parseTime=True"
//	analyticsDB, _ := db.Open(cfg2)
//
//	// Distinguish metrics with db_name labels.
//	prometheus.MustRegister(db.NewDBStats(primaryDB, db.WithDBName("primary")))
//	prometheus.MustRegister(db.NewDBStats(analyticsDB, db.WithDBName("analytics")))
//
// # Plugins
//
//	import "gorm.io/plugin/opentelemetry/tracing"
//
//	cfg := db.DefaultConfig()
//	cfg.DSN = "user:pass@tcp(127.0.0.1:3306)/mydb?charset=utf8mb4&parseTime=True"
//	cfg.Plugins = []gorm.Plugin{tracing.NewPlugin()}
//	gormDB, _ := db.Open(cfg)
//
// # Panic on startup
//
// Use MustOpen when a missing database is a hard failure (init, main):
//
//	gormDB := db.MustOpen(cfg)
//	defer db.Close(gormDB)
//
// # Testing with sqlmock
//
//	func TestRepository(t *testing.T) {
//	    sqlDB, mock, _ := sqlmock.New()
//	    defer sqlDB.Close()
//
//	    gormDB, _ := gorm.Open(mysql.New(mysql.Config{
//	        Conn:                      sqlDB,
//	        SkipInitializeWithVersion: true,
//	    }), &gorm.Config{})
//
//	    mock.ExpectPing()
//	    assert.NoError(t, db.Ping(context.Background(), gormDB))
//
//	    mock.ExpectClose()
//	    assert.NoError(t, db.Close(gormDB))
//	}
package db

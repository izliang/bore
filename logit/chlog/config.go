// Package chlog is a logit.Logger adapter that writes structured logs to ClickHouse
// and/or console, depending on the configured Mode.
//
// Unlike traditional loggers with fixed columns, chlog lets users define exactly
// which columns their ClickHouse table has by mapping lctx context keys and log
// field keys to named columns. A raw formatted log line can also be written to a
// configurable column for human-readable inspection.
//
// Usage (ClickHouse + console):
//
//	db, _ := sql.Open("clickhouse", "tcp://host:9000/mydb?username=admin&password=secret")
//	log, _ := chlog.New(chlog.Config{
//	    Mode:     chlog.ModeBoth,
//	    DB:       db,
//	    Database: "mydb",
//	    Table:    "app_logs",
//	    Schema: &ck.TableDef{
//	        Columns: []ck.ColumnDef{
//	            {Name: "ts", Type: "DateTime64(3)"},
//	            {Name: "raw", Type: "String"},
//	            {Name: "request_id", Type: "String", Searchable: true},
//	            {Name: "cost_ms", Type: "Int64"},
//	        },
//	        PartitionBy: "toYYYYMMDD(ts)",
//	        TTLDays:     30,
//	    },
//	    RawColumn: "raw",
//	    Columns: []chlog.ColumnMapping{
//	        {Key: "rid", Column: "request_id", Type: "String"},
//	        {Key: "cost", Column: "cost_ms", Type: "Int64"},
//	    },
//	})
//	defer log.Close()
//
//	if err := log.Init(context.Background()); err != nil {
//	    log.Fatal(err)
//	}
//
// Usage (console only — no ClickHouse needed):
//
//	log, _ := chlog.New(chlog.Config{
//	    Mode:   chlog.ModeConsole,
//	    Stdout: os.Stdout,
//	    Columns: []chlog.ColumnMapping{
//	        {Key: "rid", Column: "request_id", Type: "String"},
//	    },
//	})
package chlog

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/izliang/bore/ck"
	"github.com/izliang/bore/logit"
)

// Mode controls where log output is sent.
type Mode int

const (
	// ModeBoth writes to both ClickHouse and console.
	ModeBoth Mode = iota

	// ModeCK writes only to ClickHouse — no console output.
	ModeCK

	// ModeConsole writes only to console — no ClickHouse connection needed.
	ModeConsole
)

// String returns a human-readable mode name.
func (m Mode) String() string {
	switch m {
	case ModeBoth:
		return "both"
	case ModeCK:
		return "ck"
	case ModeConsole:
		return "console"
	default:
		return "unknown"
	}
}

// ColumnMapping defines how a log field or lctx key maps to a ClickHouse column.
//
// When a log entry is written, chlog collects keys from:
//  1. lctx pairs (context key-values) — highest priority
//  2. log field funcs (FieldFunc key-value pairs)
//
// If a key matches ColumnMapping.Key, the value is converted according to
// ColumnMapping.Type and written to the named column.
type ColumnMapping struct {
	Key       string // lctx pair key or field func key to match
	Column    string // ClickHouse column name
	Type      string // ClickHouse type for value conversion: "String", "Int64", "Float64", "Bool"
	SkipEmpty bool   // if true, skip writing to CK when the field value is empty (stdout always includes it)
}

// Config controls the chlog logger behavior.
type Config struct {
	// Mode controls output target. Default ModeBoth.
	// ModeConsole does not need DB/Database/Table/Schema.
	Mode Mode

	// DB is the ClickHouse database/sql connection.
	// Required for ModeBoth and ModeCK.
	DB *sql.DB

	// Database and Table identify the target ClickHouse table.
	// Required for ModeBoth and ModeCK.
	Database string
	Table    string

	// Schema defines the ClickHouse table DDL. When set, Init() creates
	// the database and table automatically.
	//
	// Database and Table are automatically populated from Config —
	// you only need to provide Columns, Engine, PartitionBy, etc.
	//
	// Every column name referenced by TimeColumn, RawColumn, or
	// ColumnMapping.Column must exist in Schema.Columns.
	// Required for ModeBoth and ModeCK.
	Schema *ck.TableDef

	// TimeColumn is the column name for the log timestamp. Default "ts".
	TimeColumn string

	// RawColumn is the column name for the full formatted log line.
	// Empty means skip writing the raw CK column.
	RawColumn string

	// Columns maps lctx/field keys to ClickHouse columns with type conversion.
	Columns []ColumnMapping

	// Level is the minimum log level to emit. Default logit.Info.
	Level logit.Level

	// BatchSize is the number of rows to buffer before flushing. Default 1000.
	BatchSize int

	// FlushInterval is the maximum time between flushes. Default 5s.
	FlushInterval time.Duration

	// Stdout is the console writer. Default os.Stdout for ModeBoth and ModeConsole.
	// Ignored in ModeCK.
	Stdout io.Writer

	// OnError is called for async write errors (e.g. DB flush failures).
	OnError func(error)

	// Location is the timezone for the TimeColumn. Default Asia/Shanghai.
	Location *time.Location

	// SeqColumn is the column name for a monotonically increasing sequence number.
	// When set, each log entry receives an auto-incrementing UInt64 value,
	// serving as a tiebreaker for rows with identical timestamps.
	// The column must exist in Schema.Columns with type UInt64.
	// Empty means no sequence column is written.
	SeqColumn string
}

func (c *Config) normalize() error {
	if c.TimeColumn == "" {
		c.TimeColumn = "ts"
	}
	if c.Location == nil {
		loc, err := time.LoadLocation("Asia/Shanghai")
		if err != nil {
			loc = time.UTC
		}
		c.Location = loc
	}
	if c.Level == "" {
		c.Level = logit.Info
	}

	switch c.Mode {
	case ModeBoth:
		if err := c.validateCKRequired(); err != nil {
			return fmt.Errorf("chlog: ModeBoth: %w", err)
		}
		if c.Stdout == nil {
			c.Stdout = os.Stdout
		}

	case ModeCK:
		if err := c.validateCKRequired(); err != nil {
			return fmt.Errorf("chlog: ModeCK: %w", err)
		}
		if c.Stdout == nil {
			c.Stdout = io.Discard
		}

	case ModeConsole:
		if c.Stdout == nil {
			c.Stdout = os.Stdout
		}

	default:
		return fmt.Errorf("chlog: unknown mode %d", c.Mode)
	}

	for _, m := range c.Columns {
		if m.Key == "" || m.Column == "" {
			return fmt.Errorf("chlog: ColumnMapping requires both Key and Column, got Key=%q Column=%q", m.Key, m.Column)
		}
		if m.Type == "" {
			m.Type = "String"
		}
	}

	// If Schema is provided, validate column consistency.
	if c.Schema != nil {
		if err := c.validateSchemaColumns(); err != nil {
			return err
		}
		// Auto-populate Database/Table from Config.
		c.Schema.Database = c.Database
		c.Schema.Table = c.Table
	}

	return nil
}

// validateCKRequired checks fields needed for ClickHouse modes.
func (c *Config) validateCKRequired() error {
	if c.DB == nil {
		return errors.New("DB is required")
	}
	if c.Database == "" {
		return errors.New("database is required")
	}
	if c.Table == "" {
		return errors.New("table is required")
	}
	if c.Schema == nil {
		return errors.New("schema is required")
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 1000
	}
	if c.FlushInterval <= 0 {
		c.FlushInterval = 5 * time.Second
	}
	return nil
}

// validateSchemaColumns checks that all column names referenced in Config
// exist in the Schema's Columns.
func (c *Config) validateSchemaColumns() error {
	colSet := make(map[string]bool, len(c.Schema.Columns))
	for _, col := range c.Schema.Columns {
		colSet[col.Name] = true
	}

	// TimeColumn must exist.
	if !colSet[c.TimeColumn] {
		return fmt.Errorf("chlog: TimeColumn %q not found in Schema.Columns", c.TimeColumn)
	}

	// RawColumn must exist if set.
	if c.RawColumn != "" && !colSet[c.RawColumn] {
		return fmt.Errorf("chlog: RawColumn %q not found in Schema.Columns", c.RawColumn)
	}

	// Every mapped column must exist.
	for _, m := range c.Columns {
		if !colSet[m.Column] {
			return fmt.Errorf("chlog: Column %q (mapped from key %q) not found in Schema.Columns", m.Column, m.Key)
		}
	}

	// SeqColumn must exist if set.
	if c.SeqColumn != "" && !colSet[c.SeqColumn] {
		return fmt.Errorf("chlog: SeqColumn %q not found in Schema.Columns", c.SeqColumn)
	}

	return nil
}

// usesCK reports whether this config writes to ClickHouse.
func (c *Config) usesCK() bool {
	return c.Mode == ModeBoth || c.Mode == ModeCK
}

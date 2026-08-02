package chlog

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"strings"
	"testing"
	"time"

	"github.com/izliang/bore/ck"
	"github.com/izliang/bore/logit"
	"github.com/izliang/bore/logit/lctx"
)

// --- dummy driver ---

func testDB() *sql.DB {
	db, err := sql.Open("chlog_test", "")
	if err != nil {
		panic(err)
	}
	return db
}

func init() {
	sql.Register("chlog_test", &dummyDriver{})
}

type dummyDriver struct{}

func (d *dummyDriver) Open(name string) (driver.Conn, error) { return &dummyConn{}, nil }

type dummyConn struct{}

func (c *dummyConn) Prepare(query string) (driver.Stmt, error) { return &dummyStmt{}, nil }
func (c *dummyConn) Close() error                              { return nil }
func (c *dummyConn) Begin() (driver.Tx, error)                 { return &dummyTx{}, nil }

type dummyStmt struct{}

func (s *dummyStmt) Close() error                                    { return nil }
func (s *dummyStmt) NumInput() int                                   { return -1 }
func (s *dummyStmt) Exec(args []driver.Value) (driver.Result, error) { return dummyResult{}, nil }
func (s *dummyStmt) Query(args []driver.Value) (driver.Rows, error)  { return nil, nil }

type dummyTx struct{}

func (t *dummyTx) Commit() error   { return nil }
func (t *dummyTx) Rollback() error { return nil }

type dummyResult struct{}

func (r dummyResult) LastInsertId() (int64, error) { return 0, nil }
func (r dummyResult) RowsAffected() (int64, error) { return 1, nil }

// --- test helpers ---

// testSchema returns a minimal ck.TableDef that matches testCfg columns.
func testSchema() *ck.TableDef {
	return &ck.TableDef{
		Columns: []ck.ColumnDef{
			{Name: "ts", Type: "DateTime64(3)"},
			{Name: "raw", Type: "String"},
			{Name: "request_id", Type: "String", Searchable: true},
			{Name: "cost_ms", Type: "Int64"},
			{Name: "rate_pct", Type: "Float64"},
			{Name: "success", Type: "Bool"},
		},
		PartitionBy: "toYYYYMMDD(ts)",
		TTLDays:     30,
	}
}

func testCfg() *Config {
	cfg := &Config{
		Mode:       ModeBoth,
		DB:         testDB(),
		Database:   "testdb",
		Table:      "logs",
		Schema:     testSchema(),
		TimeColumn: "ts",
		RawColumn:  "raw",
		Columns: []ColumnMapping{
			{Key: "rid", Column: "request_id", Type: "String"},
			{Key: "cost", Column: "cost_ms", Type: "Int64"},
			{Key: "rate", Column: "rate_pct", Type: "Float64"},
			{Key: "ok", Column: "success", Type: "Bool"},
		},
		Level:  logit.Info,
		Stdout: &bytes.Buffer{},
	}
	_ = cfg.normalize()
	return cfg
}

// --- Config normalization ---

func TestConfigNormalize_ModeBoth(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		cfg := &Config{
			Mode:     ModeBoth,
			DB:       testDB(),
			Database: "testdb",
			Table:    "logs",
			Schema:   testSchema(),
			Columns:  []ColumnMapping{{Key: "rid", Column: "request_id"}},
		}
		if err := cfg.normalize(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Stdout == nil {
			t.Error("ModeBoth should default Stdout")
		}
	})

	t.Run("missing Schema", func(t *testing.T) {
		cfg := &Config{
			Mode:     ModeBoth,
			DB:       testDB(),
			Database: "db",
			Table:    "t",
		}
		if err := cfg.normalize(); err == nil || !strings.Contains(err.Error(), "schema") {
			t.Errorf("expected schema error, got: %v", err)
		}
	})

	t.Run("missing DB", func(t *testing.T) {
		cfg := &Config{Mode: ModeBoth, Database: "db", Table: "t", Schema: testSchema()}
		if err := cfg.normalize(); err == nil || !strings.Contains(err.Error(), "DB") {
			t.Errorf("expected DB error, got: %v", err)
		}
	})
}

func TestConfigNormalize_ModeCK(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		cfg := &Config{
			Mode:     ModeCK,
			DB:       testDB(),
			Database: "testdb",
			Table:    "logs",
			Schema:   testSchema(),
		}
		if err := cfg.normalize(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestConfigNormalize_ModeConsole(t *testing.T) {
	t.Run("valid without DB", func(t *testing.T) {
		cfg := &Config{
			Mode:    ModeConsole,
			Columns: []ColumnMapping{{Key: "rid", Column: "request_id"}},
		}
		if err := cfg.normalize(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("DB and Schema not required", func(t *testing.T) {
		cfg := &Config{Mode: ModeConsole}
		if err := cfg.normalize(); err != nil {
			t.Fatalf("ModeConsole should not require DB or Schema: %v", err)
		}
	})
}

func TestConfigNormalize_Defaults(t *testing.T) {
	cfg := &Config{
		Mode:     ModeBoth,
		DB:       testDB(),
		Database: "db",
		Table:    "t",
		Schema:   testSchema(),
	}
	if err := cfg.normalize(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.TimeColumn != "ts" {
		t.Errorf("default TimeColumn: got %q", cfg.TimeColumn)
	}
	if cfg.Level != logit.Info {
		t.Errorf("default Level: got %q", cfg.Level)
	}
	if cfg.BatchSize != 1000 {
		t.Errorf("default BatchSize: got %d", cfg.BatchSize)
	}
	if cfg.FlushInterval != 5*time.Second {
		t.Errorf("default FlushInterval: got %v", cfg.FlushInterval)
	}
}

func TestConfigNormalize_SchemaColumnValidation(t *testing.T) {
	t.Run("TimeColumn not in schema", func(t *testing.T) {
		cfg := &Config{
			Mode:       ModeBoth,
			DB:         testDB(),
			Database:   "db",
			Table:      "t",
			Schema:     testSchema(),
			TimeColumn: "nope",
		}
		if err := cfg.normalize(); err == nil || !strings.Contains(err.Error(), "TimeColumn") {
			t.Errorf("expected TimeColumn mismatch error, got: %v", err)
		}
	})

	t.Run("RawColumn not in schema", func(t *testing.T) {
		cfg := &Config{
			Mode:      ModeBoth,
			DB:        testDB(),
			Database:  "db",
			Table:     "t",
			Schema:    testSchema(),
			RawColumn: "missing_raw",
		}
		if err := cfg.normalize(); err == nil || !strings.Contains(err.Error(), "RawColumn") {
			t.Errorf("expected RawColumn mismatch error, got: %v", err)
		}
	})

	t.Run("mapped column not in schema", func(t *testing.T) {
		cfg := &Config{
			Mode:     ModeBoth,
			DB:       testDB(),
			Database: "db",
			Table:    "t",
			Schema:   testSchema(),
			Columns:  []ColumnMapping{{Key: "x", Column: "nonexistent"}},
		}
		if err := cfg.normalize(); err == nil || !strings.Contains(err.Error(), "nonexistent") {
			t.Errorf("expected column mismatch error, got: %v", err)
		}
	})

	t.Run("Schema auto-populates Database and Table", func(t *testing.T) {
		cfg := &Config{
			Mode:     ModeBoth,
			DB:       testDB(),
			Database: "mydb",
			Table:    "mytable",
			Schema:   testSchema(),
		}
		if err := cfg.normalize(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Schema.Database != "mydb" {
			t.Errorf("Schema.Database: got %q, want mydb", cfg.Schema.Database)
		}
		if cfg.Schema.Table != "mytable" {
			t.Errorf("Schema.Table: got %q, want mytable", cfg.Schema.Table)
		}
	})
}

func TestConfig_UsesCK(t *testing.T) {
	tests := []struct {
		mode Mode
		want bool
	}{
		{ModeBoth, true},
		{ModeCK, true},
		{ModeConsole, false},
	}
	for _, tt := range tests {
		cfg := &Config{Mode: tt.mode}
		if got := cfg.usesCK(); got != tt.want {
			t.Errorf("Mode %s: usesCK() = %v, want %v", tt.mode, got, tt.want)
		}
	}
}

// --- convertValue ---

func TestConvertValue(t *testing.T) {
	tests := []struct {
		name string
		val  string
		typ  string
		want any
	}{
		{"string passthrough", "hello", "String", "hello"},
		{"int64 valid", "42", "Int64", int64(42)},
		{"int64 invalid fallback", "abc", "Int64", "abc"},
		{"int64 negative", "-7", "Int64", int64(-7)},
		{"float64 valid", "3.14", "Float64", 3.14},
		{"float64 invalid fallback", "x", "Float64", "x"},
		{"bool true", "true", "Bool", true},
		{"bool false", "false", "Bool", false},
		{"bool 1", "1", "Bool", true},
		{"bool invalid fallback", "yes", "Bool", "yes"},
		{"empty type defaults to string", "hi", "", "hi"},
		{"unknown type defaults to string", "hi", "Unknown", "hi"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertValue(tt.val, tt.typ)
			if got != tt.want {
				t.Errorf("convertValue(%q, %q) = %v (%T), want %v (%T)",
					tt.val, tt.typ, got, got, tt.want, tt.want)
			}
		})
	}
}

// --- buildRow ---

func TestBuildRow_BasicColumns(t *testing.T) {
	cfg := testCfg()
	r := buildRow(cfg, logit.Info, "hello", nil, nil, 0)

	if _, ok := r[cfg.TimeColumn]; !ok {
		t.Errorf("missing TimeColumn %q", cfg.TimeColumn)
	}
	if v, ok := r[cfg.RawColumn].(string); !ok || !strings.Contains(v, "hello") {
		t.Errorf("RawColumn missing or wrong: %v", r[cfg.RawColumn])
	}
}

func TestBuildRow_LctxFields(t *testing.T) {
	cfg := testCfg()
	ctx := lctx.PushPair(context.Background(),
		pairFormatter{{Key: "rid", Value: "req-123"}},
	)
	r := buildRow(cfg, logit.Info, "msg", ctx, nil, 0)

	if v, ok := r["request_id"]; !ok || v != "req-123" {
		t.Errorf("request_id: got %v, want req-123", v)
	}
}

func TestBuildRow_FieldFuncs(t *testing.T) {
	cfg := testCfg()
	r := buildRow(cfg, logit.Info, "msg", nil, []logit.FieldFunc{
		logit.Int64Field("cost", 42),
		logit.Float64Field("rate", 0.95),
		logit.BoolField("ok", true),
	}, 0)

	if v, ok := r["cost_ms"].(int64); !ok || v != 42 {
		t.Errorf("cost_ms: got %v (%T), want 42", r["cost_ms"], r["cost_ms"])
	}
	if v, ok := r["rate_pct"].(float64); !ok || v != 0.95 {
		t.Errorf("rate_pct: got %v (%T), want 0.95", r["rate_pct"], r["rate_pct"])
	}
	if v, ok := r["success"].(bool); !ok || !v {
		t.Errorf("success: got %v (%T), want true", r["success"], r["success"])
	}
}

func TestBuildRow_LctxPriorityOverField(t *testing.T) {
	cfg := testCfg()
	ctx := lctx.PushPair(context.Background(),
		pairFormatter{{Key: "rid", Value: "ctx-val"}},
	)
	r := buildRow(cfg, logit.Info, "msg", ctx, []logit.FieldFunc{
		logit.StringField("rid", "field-val"),
	}, 0)

	if v, ok := r["request_id"]; !ok || v != "ctx-val" {
		t.Errorf("request_id: got %v, want ctx-val (lctx should win)", v)
	}
}

func TestBuildRow_NoRawColumn(t *testing.T) {
	cfg := testCfg()
	cfg.RawColumn = ""
	r := buildRow(cfg, logit.Info, "msg", nil, nil, 0)

	if _, ok := r["raw"]; ok {
		t.Errorf("raw column should not exist when RawColumn is empty")
	}
}

func TestBuildRow_NilContext(t *testing.T) {
	cfg := testCfg()
	r := buildRow(cfg, logit.Info, "msg", nil, []logit.FieldFunc{
		logit.StringField("rid", "val"),
	}, 0)

	if v, ok := r["request_id"]; !ok || v != "val" {
		t.Errorf("request_id: got %v, want val", v)
	}
}

func TestBuildRow_UnmappedFieldIgnored(t *testing.T) {
	cfg := testCfg()
	r := buildRow(cfg, logit.Info, "msg", nil, []logit.FieldFunc{
		logit.StringField("unmapped_key", "some_val"),
	}, 0)

	if _, ok := r["unmapped_key"]; ok {
		t.Errorf("unmapped field should not appear as column")
	}
}

// --- buildRow: SkipEmpty ---

func TestBuildRow_SkipEmpty_SkipsCKColumn(t *testing.T) {
	cfg := testCfg()
	// Override "request_id" mapping to skip empty values.
	cfg.Columns = []ColumnMapping{
		{Key: "rid", Column: "request_id", Type: "String", SkipEmpty: true},
	}
	r := buildRow(cfg, logit.Info, "msg", nil, nil, 0)

	// No lctx or field provided for "rid" — should not appear in CK row.
	if _, ok := r["request_id"]; ok {
		t.Errorf("request_id should be skipped when SkipEmpty and value is empty")
	}
}

func TestBuildRow_SkipEmpty_WritesNonEmpty(t *testing.T) {
	cfg := testCfg()
	cfg.Columns = []ColumnMapping{
		{Key: "rid", Column: "request_id", Type: "String", SkipEmpty: true},
	}
	ctx := lctx.PushPair(context.Background(),
		pairFormatter{{Key: "rid", Value: "req-1"}},
	)
	r := buildRow(cfg, logit.Info, "msg", ctx, nil, 0)

	if v, ok := r["request_id"]; !ok || v != "req-1" {
		t.Errorf("request_id should be written when non-empty, got %v", v)
	}
}

func TestBuildRow_SkipEmpty_AlwaysInRaw(t *testing.T) {
	cfg := testCfg()
	cfg.Columns = []ColumnMapping{
		{Key: "rid", Column: "request_id", Type: "String", SkipEmpty: true},
	}
	// No lctx or field — value is empty, column skipped from CK row
	// but buildRaw (used for stdout) should still include it.
	r := buildRow(cfg, logit.Info, "msg", nil, nil, 0)

	if _, ok := r["request_id"]; ok {
		t.Errorf("request_id should be skipped from CK row")
	}
	// Raw line always includes all field info regardless of SkipEmpty.
	if raw, ok := r["raw"].(string); ok {
		// raw is built from the original fields, not affected by SkipEmpty
		if !strings.Contains(raw, "msg[msg]") {
			t.Errorf("raw should still contain msg: %s", raw)
		}
	}
}

func TestBuildRow_SkipEmpty_OnlySkippedWhenEmpty(t *testing.T) {
	cfg := testCfg()
	cfg.Columns = []ColumnMapping{
		{Key: "rid", Column: "request_id", Type: "String", SkipEmpty: true},
		{Key: "cost", Column: "cost_ms", Type: "Int64", SkipEmpty: true},
	}
	// cost is provided, rid is empty → cost should be in row, rid skipped.
	r := buildRow(cfg, logit.Info, "msg", nil, []logit.FieldFunc{
		logit.Int64Field("cost", 100),
	}, 0)

	if _, ok := r["request_id"]; ok {
		t.Errorf("request_id should be skipped when empty")
	}
	if v, ok := r["cost_ms"].(int64); !ok || v != 100 {
		t.Errorf("cost_ms should be written when non-empty, got %v", v)
	}
}

// --- groupByColumns ---

func TestGroupByColumns_Homogeneous(t *testing.T) {
	rows := []row{
		{"a": "1", "b": "2"},
		{"a": "3", "b": "4"},
	}
	groups := groupByColumns(rows)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if len(groups[0].rows) != 2 {
		t.Fatalf("expected 2 rows in group, got %d", len(groups[0].rows))
	}
}

func TestGroupByColumns_Heterogeneous(t *testing.T) {
	// SkipEmpty causes rows with different column sets.
	rows := []row{
		{"ts": "t1", "raw": "r1", "task_id": "abc"},
		{"ts": "t2", "raw": "r2"}, // task_id skipped
		{"ts": "t3", "raw": "r3", "task_id": "def"},
	}
	groups := groupByColumns(rows)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups (with and without task_id), got %d", len(groups))
	}
	// One group should have 2 rows, the other 1.
	sum := 0
	for _, g := range groups {
		sum += len(g.rows)
	}
	if sum != 3 {
		t.Fatalf("total rows = %d, want 3", sum)
	}
}

// --- buildRaw ---

func TestBuildRaw(t *testing.T) {
	ctx := lctx.PushPair(context.Background(),
		pairFormatter{{Key: "rid", Value: "abc"}},
	)
	raw := buildRaw(time.Now(), logit.Info, "test message", ctx,
		[]logit.FieldFunc{logit.Int64Field("cost", 100)},
	)

	if !strings.Contains(raw, "INFO") {
		t.Errorf("missing level: %s", raw)
	}
	if !strings.Contains(raw, "msg[test message]") {
		t.Errorf("missing msg: %s", raw)
	}
	if !strings.Contains(raw, "rid[abc]") {
		t.Errorf("missing lctx field: %s", raw)
	}
	if !strings.Contains(raw, "cost[100]") {
		t.Errorf("missing field: %s", raw)
	}
}

// --- Logger: level filtering ---

func TestLogger_LevelFiltering(t *testing.T) {
	l := &Logger{level: logit.Info}

	if l.shouldLog(logit.Info) != true {
		t.Error("Info should pass Info filter")
	}
	if l.shouldLog(logit.Warn) != true {
		t.Error("Warn should pass Info filter")
	}
	if l.shouldLog(logit.Error) != true {
		t.Error("Error should pass Info filter")
	}
	if l.shouldLog(logit.Debug) != false {
		t.Error("Debug should not pass Info filter")
	}
	if l.shouldLog(logit.Trace) != false {
		t.Error("Trace should not pass Info filter")
	}
}

func TestLogger_LevelFiltering_Warn(t *testing.T) {
	l := &Logger{level: logit.Warn}

	if l.shouldLog(logit.Debug) != false {
		t.Error("Debug should not pass Warn filter")
	}
	if l.shouldLog(logit.Info) != false {
		t.Error("Info should not pass Warn filter")
	}
	if l.shouldLog(logit.Warn) != true {
		t.Error("Warn should pass Warn filter")
	}
	if l.shouldLog(logit.Error) != true {
		t.Error("Error should pass Warn filter")
	}
}

// --- Logger: mode-specific behavior ---

func TestLogger_ModeConsole_NoDB(t *testing.T) {
	var buf bytes.Buffer
	cfg := Config{
		Mode:   ModeConsole,
		Stdout: &buf,
		Level:  logit.Debug,
		Columns: []ColumnMapping{
			{Key: "rid", Column: "request_id"},
		},
	}
	log, err := New(cfg)
	if err != nil {
		t.Fatalf("New(ModeConsole): %v", err)
	}
	defer log.Close()

	ctx := lctx.PushPair(context.Background(),
		pairFormatter{{Key: "rid", Value: "req-1"}},
	)
	log.Info(ctx, "console test", logit.Int64Field("cost", 10))

	out := buf.String()
	if !strings.Contains(out, "INFO") {
		t.Errorf("expected INFO level in output: %s", out)
	}
	if !strings.Contains(out, "msg[console test]") {
		t.Errorf("expected msg in output: %s", out)
	}
	if !strings.Contains(out, "rid[req-1]") {
		t.Errorf("expected lctx field in output: %s", out)
	}
	if !strings.Contains(out, "cost[10]") {
		t.Errorf("expected field in output: %s", out)
	}
}

func TestLogger_ModeConsole_LevelFilter(t *testing.T) {
	var buf bytes.Buffer
	cfg := Config{
		Mode:   ModeConsole,
		Stdout: &buf,
		Level:  logit.Warn,
	}
	log, err := New(cfg)
	if err != nil {
		t.Fatalf("New(ModeConsole): %v", err)
	}
	defer log.Close()

	log.Debug(context.Background(), "should be dropped")
	log.Info(context.Background(), "should be dropped")
	log.Warn(context.Background(), "should appear")

	out := buf.String()
	if strings.Contains(out, "should be dropped") {
		t.Errorf("filtered messages should not appear: %s", out)
	}
	if !strings.Contains(out, "should appear") {
		t.Errorf("Warn message should appear: %s", out)
	}
}

func TestLogger_ModeBoth_StdoutOutput(t *testing.T) {
	var buf bytes.Buffer
	cfg := Config{
		Mode:     ModeBoth,
		DB:       testDB(),
		Database: "testdb",
		Table:    "logs",
		Schema:   testSchema(),
		Stdout:   &buf,
		Level:    logit.Debug,
	}
	log, err := New(cfg)
	if err != nil {
		t.Fatalf("New(ModeBoth): %v", err)
	}
	defer log.Close()

	log.Info(context.Background(), "both mode test")

	out := buf.String()
	if !strings.Contains(out, "INFO") || !strings.Contains(out, "both mode test") {
		t.Errorf("stdout output missing: %s", out)
	}
}

func TestLogger_ModeCK_NoStdout(t *testing.T) {
	cfg := Config{
		Mode:     ModeCK,
		DB:       testDB(),
		Database: "testdb",
		Table:    "logs",
		Schema:   testSchema(),
		Level:    logit.Debug,
	}
	log, err := New(cfg)
	if err != nil {
		t.Fatalf("New(ModeCK): %v", err)
	}
	defer log.Close()

	// ModeCK should not panic when writing — Stdout is io.Discard.
	log.Info(context.Background(), "ck only test")
}

// --- Logger: Init ---

func TestLogger_Init_ModeConsole_NoOp(t *testing.T) {
	cfg := Config{
		Mode:   ModeConsole,
		Stdout: &bytes.Buffer{},
	}
	log, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer log.Close()

	if err := log.Init(context.Background()); err != nil {
		t.Errorf("Init should be no-op for ModeConsole: %v", err)
	}
}

func TestLogger_Init_WithSchema(t *testing.T) {
	cfg := Config{
		Mode:     ModeBoth,
		DB:       testDB(),
		Database: "testdb",
		Table:    "logs",
		Schema:   testSchema(),
		Level:    logit.Debug,
		Stdout:   &bytes.Buffer{},
	}
	log, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer log.Close()

	// Init should execute CREATE DATABASE and CREATE TABLE via the dummy driver.
	if err := log.Init(context.Background()); err != nil {
		t.Errorf("Init with schema: %v", err)
	}
}

// --- Logger: Write (io.Writer) ---

func TestLogger_Write_ModeConsole(t *testing.T) {
	var buf bytes.Buffer
	cfg := Config{
		Mode:   ModeConsole,
		Stdout: &buf,
	}
	log, err := New(cfg)
	if err != nil {
		t.Fatalf("New(ModeConsole): %v", err)
	}
	defer log.Close()

	n, err := log.Write([]byte("raw write\n"))
	if err != nil || n != 10 {
		t.Fatalf("Write = (%d, %v)", n, err)
	}
	if !strings.Contains(buf.String(), "raw write") {
		t.Errorf("Write output: %s", buf.String())
	}
}

// --- Helpers ---

type pairFormatter []lctx.Pair

func (p pairFormatter) Values() []lctx.Pair { return p }

// compile-time check: Logger implements logit.Logger.
var _ logit.Logger = (*Logger)(nil)

package ck

import (
	"testing"
	"time"
)

// ----- schema -----

func testDef() TableDef {
	return TableDef{
		Database: "testdb",
		Table:    "events",
		Columns: []ColumnDef{
			{Name: "ts", Type: "DateTime64(3)"},
			{Name: "user_id", Type: "String", Searchable: true},
			{Name: "event", Type: "String", Searchable: true},
			{Name: "cost_ms", Type: "Int64"},
		},
		PartitionBy: "toYYYYMMDD(ts)",
		TTLDays:     30,
	}
}

func TestCreateTableSQL(t *testing.T) {
	sql := CreateTableSQL(testDef())
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS `testdb`.`events`",
		"ENGINE = MergeTree",
		"PARTITION BY toYYYYMMDD(ts)",
		"ORDER BY (user_id, event)",
		"TTL toDateTime(`ts`) + INTERVAL 30 DAY",
		"INDEX idx_user_id `user_id` TYPE bloom_filter GRANULARITY 1",
		"INDEX idx_event `event` TYPE bloom_filter GRANULARITY 1",
	} {
		if !contains(sql, want) {
			t.Errorf("missing %q in:\n%s", want, sql)
		}
	}
}

func TestCreateTableSQL_ExplicitOrderBy(t *testing.T) {
	def := testDef()
	def.OrderBy = []string{"ts", "user_id"}
	sql := CreateTableSQL(def)
	if !contains(sql, "ORDER BY (ts, user_id)") {
		t.Errorf("expected explicit ORDER BY:\n%s", sql)
	}
}

func TestCreateTableSQL_NoTTL(t *testing.T) {
	def := testDef()
	def.TTLDays = 0
	if contains(CreateTableSQL(def), "TTL") {
		t.Error("should not have TTL")
	}
}

func TestCreateTableSQL_CustomEngine(t *testing.T) {
	def := testDef()
	def.Engine = "ReplacingMergeTree"
	if !contains(CreateTableSQL(def), "ENGINE = ReplacingMergeTree") {
		t.Error("expected custom engine")
	}
}

func TestCreateTableSQL_ColumnNameQuoting(t *testing.T) {
	def := TableDef{
		Database: "db", Table: "t",
		Columns: []ColumnDef{{Name: "my-column", Type: "String", Searchable: true}},
	}
	sql := CreateTableSQL(def)
	if !contains(sql, "`my-column` String") || !contains(sql, "INDEX idx_my-column `my-column`") {
		t.Errorf("column not quoted:\n%s", sql)
	}
}

func TestTableDef_Validate(t *testing.T) {
	if err := testDef().Validate(); err != nil {
		t.Errorf("valid: %v", err)
	}
	if err := (TableDef{Database: "db"}).Validate(); err == nil {
		t.Error("missing table should error")
	}
	if err := (TableDef{Table: "t"}).Validate(); err == nil {
		t.Error("missing database should error")
	}
}

func TestCreateDatabaseSQL(t *testing.T) {
	if sql := CreateDatabaseSQL("mydb"); sql != "CREATE DATABASE IF NOT EXISTS `mydb`" {
		t.Errorf("got %q", sql)
	}
}

// ----- cursor stream -----

func TestCursorStream_FirstFetch(t *testing.T) {
	s := &CursorStream{query: "SELECT * FROM t", cursorCol: "ts", batchSize: 100}
	q, args := s.buildQuery(nil)
	if len(args) != 1 || args[0] != 100 {
		t.Fatalf("args = %v", args)
	}
	if !contains(q, "ORDER BY ts LIMIT ?") {
		t.Errorf("q = %s", q)
	}
	if !contains(q, "SELECT * FROM t") {
		t.Errorf("lost base query: %s", q)
	}
}

func TestCursorStream_SubsequentFetch(t *testing.T) {
	s := &CursorStream{query: "SELECT * FROM t", cursorCol: "ts", batchSize: 200}
	s.cursor.Value = "2026-01-01"
	q, args := s.buildQuery(nil)
	if len(args) != 2 || args[0] != "2026-01-01" || args[1] != 200 {
		t.Fatalf("args = %v", args)
	}
	if !contains(q, "WHERE ts > ?") {
		t.Errorf("missing cursor condition: %s", q)
	}
}

func TestCursorStream_WithWhereClause(t *testing.T) {
	s := &CursorStream{
		query:     "SELECT * FROM t WHERE type = 'click'",
		cursorCol: "ts", batchSize: 50, hasWhere: true,
	}
	s.cursor.Value = "2026-01-01"
	q, args := s.buildQuery(nil)
	if !contains(q, "AND ts > ?") {
		t.Errorf("expected AND clause: %s", q)
	}
	if len(args) != 2 {
		t.Fatalf("args = %v", args)
	}
}

func TestCursorStream_NextArgs(t *testing.T) {
	s := &CursorStream{query: "SELECT * FROM t WHERE type = ?", cursorCol: "ts", batchSize: 100, hasWhere: true}
	s.cursor.Value = "2026-01-01"
	q, args := s.buildQuery([]any{"click"})
	if len(args) != 3 || args[0] != "click" || args[1] != "2026-01-01" || args[2] != 100 {
		t.Fatalf("args = %v", args)
	}
	t.Log(q)
}

func TestCursorStream_Checkpoint(t *testing.T) {
	s := &CursorStream{}
	s.SetCheckpoint(Cursor{Value: "2026-06-06"})
	if s.Checkpoint().Value != "2026-06-06" {
		t.Errorf("checkpoint = %v", s.Checkpoint())
	}
	s.SetCheckpoint(Cursor{}) // reset
	if s.Checkpoint().Value != nil {
		t.Error("expected nil after reset")
	}
}

func TestWatchOptions_Defaults(t *testing.T) {
	o := WatchOptions{}
	o.normalize()
	if o.MinInterval != 100*time.Millisecond {
		t.Errorf("MinInterval = %v", o.MinInterval)
	}
	if o.MaxInterval != 5*time.Second {
		t.Errorf("MaxInterval = %v", o.MaxInterval)
	}
	if o.BackoffFactor != 2.0 {
		t.Errorf("BackoffFactor = %v", o.BackoffFactor)
	}
}

func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// ----- composite cursor (tiebreaker) -----

func TestCursorStream_Composite_FirstFetch(t *testing.T) {
	s := &CursorStream{
		query:      "SELECT * FROM t",
		cursorCol:  "ts",
		tiebreaker: "id",
		batchSize:  100,
	}
	q, args := s.buildQuery(nil)
	if len(args) != 1 || args[0] != 100 {
		t.Fatalf("args = %v", args)
	}
	if !contains(q, "ORDER BY ts, id LIMIT ?") {
		t.Errorf("missing composite ORDER BY: %s", q)
	}
}

func TestCursorStream_Composite_SubsequentFetch(t *testing.T) {
	s := &CursorStream{
		query:      "SELECT * FROM t",
		cursorCol:  "ts",
		tiebreaker: "id",
		batchSize:  200,
	}
	s.cursor.Values = []any{"2026-01-01", uint64(42)}
	q, args := s.buildQuery(nil)
	if len(args) != 3 || args[0] != "2026-01-01" || args[1] != uint64(42) || args[2] != 200 {
		t.Fatalf("args = %v", args)
	}
	if !contains(q, "(ts, id) > (?, ?)") {
		t.Errorf("missing tuple comparison: %s", q)
	}
	if !contains(q, "ORDER BY ts, id") {
		t.Errorf("missing composite ORDER BY: %s", q)
	}
}

func TestCursorStream_Composite_WithWhere(t *testing.T) {
	s := &CursorStream{
		query:      "SELECT * FROM t WHERE type = ?",
		cursorCol:  "ts",
		tiebreaker: "id",
		batchSize:  50,
		hasWhere:   true,
	}
	s.cursor.Values = []any{"2026-01-01", uint64(10)}
	q, args := s.buildQuery([]any{"click"})
	if len(args) != 4 || args[0] != "click" || args[1] != "2026-01-01" || args[2] != uint64(10) || args[3] != 50 {
		t.Fatalf("args = %v", args)
	}
	if !contains(q, "AND (ts, id) > (?, ?)") {
		t.Errorf("expected AND with tuple: %s", q)
	}
	t.Log(q)
}

func TestCursorStream_Composite_Checkpoint(t *testing.T) {
	s := &CursorStream{tiebreaker: "id"}
	s.SetCheckpoint(Cursor{Values: []any{"2026-01-01", uint64(99)}})
	c := s.Checkpoint()
	if len(c.Values) != 2 || c.Values[0] != "2026-01-01" || c.Values[1] != uint64(99) {
		t.Errorf("checkpoint = %v", c)
	}
	if !s.Checkpoint().IsZero() {
		// should not be zero
	}
	s.SetCheckpoint(Cursor{})
	if !s.Checkpoint().IsZero() {
		t.Error("expected zero after reset")
	}
}

func TestCursorStream_NewCursorStream_BackwardCompat(t *testing.T) {
	// Without tiebreaker — backward compatible, no Values field.
	s := &CursorStream{query: "SELECT * FROM t", cursorCol: "ts", batchSize: 100}
	s.cursor.Value = "2026-01-01"
	q, args := s.buildQuery(nil)
	if !contains(q, "WHERE ts > ?") {
		t.Errorf("single-col mode should use simple >: %s", q)
	}
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(args))
	}
}

func TestCursor_IsZero(t *testing.T) {
	if !(Cursor{}).IsZero() {
		t.Error("zero cursor should be zero")
	}
	if (Cursor{Value: "x"}).IsZero() {
		t.Error("cursor with Value should not be zero")
	}
	if (Cursor{Values: []any{"x", uint64(1)}}).IsZero() {
		t.Error("cursor with Values should not be zero")
	}
}

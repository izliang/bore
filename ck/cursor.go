package ck

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"
)

// Cursor represents a position in a cursor-based stream.
//
// Single-column mode:
//
//	Cursor{Value: "2026-01-01"}
//
// Composite mode (cursorCol + tiebreaker):
//
//	Cursor{Values: []any{"2026-01-01", uint64(42)}}
//
// JSON-serializable for external checkpoint storage.
type Cursor struct {
	// Value holds the single-column cursor position.
	// Used when no tiebreaker is configured.
	Value any `json:"value,omitempty"`

	// Values holds the composite cursor position [cursorCol, tiebreaker].
	// Used when a tiebreaker column is configured.
	Values []any `json:"values,omitempty"`
}

// IsZero reports whether the cursor has never been advanced.
func (c Cursor) IsZero() bool {
	return c.Value == nil && len(c.Values) == 0
}

// CursorStream provides cursor-based incremental queries.
//
// First call to Next() fetches all existing rows. Subsequent calls
// fetch only rows newer than the cursor position.
//
// Single-column mode:
//
//	stream := conn.NewCursorStream("SELECT * FROM events WHERE type=?", "ts", 500)
//
// Composite mode (recommended when cursorCol may have duplicates):
//
//	stream := conn.NewCursorStream("SELECT * FROM events WHERE type=?", "ts", 500, "id")
//
// Usage:
//
//	for {
//	    batch, err := stream.Next(ctx, "click")
//	    if err != nil { return err }
//	    if len(batch) == 0 { break }
//	    for _, row := range batch { ... }
//	}
//	saveCheckpoint(stream.Checkpoint())
//	go stream.Poll(ctx, WatchOptions{}, func(batch []Row) error { ... }, "click")
type CursorStream struct {
	conn       *Conn
	query      string // SELECT ... FROM ... WHERE ... (without ORDER BY / LIMIT)
	cursorCol  string
	tiebreaker string // optional second column for deterministic ordering within same cursorCol value
	batchSize  int
	cursor     Cursor
	hasWhere   bool
}

// NewCursorStream creates a CursorStream.
//
// baseQuery: SELECT-FROM-WHERE portion. Do NOT include ORDER BY or LIMIT.
// cursorCol: ordered column for pagination (e.g. "ts", "created_at").
// batchSize: rows per Next() call.
// tiebreaker: optional second column for deterministic ordering when cursorCol
// values may repeat. Use a unique monotonically increasing column like "id"
// or "seq". When set, ClickHouse tuple comparison is used:
//
//	WHERE (cursorCol, tiebreaker) > (?, ?)
//
// This eliminates the classic "same ts → skipped rows" bug with strict >.
//
// Example:
//
//	// Single-column (simple, but may skip rows with same ts).
//	s := conn.NewCursorStream("SELECT * FROM events", "ts", 1000)
//
//	// Composite (recommended for production).
//	s := conn.NewCursorStream("SELECT * FROM events", "ts", 500, "seq")
//
//	// With WHERE and args.
//	s := conn.NewCursorStream("SELECT * FROM events WHERE type = ?", "ts", 500, "id")
func (c *Conn) NewCursorStream(baseQuery, cursorCol string, batchSize int, tiebreaker ...string) *CursorStream {
	q := strings.TrimSpace(baseQuery)
	cs := &CursorStream{
		conn:      c,
		query:     q,
		cursorCol: cursorCol,
		batchSize: batchSize,
		hasWhere:  strings.Contains(strings.ToUpper(q), "WHERE"),
	}
	if len(tiebreaker) > 0 && tiebreaker[0] != "" {
		cs.tiebreaker = tiebreaker[0]
	}
	return cs
}

// Next fetches the next batch. First call gets all existing rows.
// Subsequent calls get rows newer than the cursor position.
// Returns empty slice when no more data is available.
//
// Query args (e.g. for ? placeholders in the base query) are passed
// through to every call.
func (s *CursorStream) Next(ctx context.Context, args ...any) ([]Row, error) {
	q, allArgs := s.buildQuery(args)
	it, err := s.conn.queryRows(ctx, q, allArgs...)
	if err != nil {
		return nil, err
	}
	defer it.close()

	var batch []Row
	var lastCursor, lastTie any
	for {
		row, err := it.next()
		if err != nil {
			return nil, err
		}
		if row == nil {
			break
		}
		batch = append(batch, row)
		lastCursor = row[s.cursorCol]
		if s.tiebreaker != "" {
			lastTie = row[s.tiebreaker]
		}
	}
	if len(batch) > 0 {
		if s.tiebreaker != "" {
			s.cursor.Values = []any{lastCursor, lastTie}
		} else {
			s.cursor.Value = lastCursor
		}
	}
	return batch, nil
}

func (s *CursorStream) buildQuery(userArgs []any) (string, []any) {
	orderClause := s.orderClause()

	if s.cursor.IsZero() {
		return s.query + orderClause + " LIMIT ?",
			append(userArgs, s.batchSize)
	}

	clause := " WHERE "
	if s.hasWhere {
		clause = " AND "
	}

	if s.tiebreaker != "" {
		// Composite: WHERE (ts, id) > (?, ?)
		clause += fmt.Sprintf("(%s, %s) > (?, ?)", s.cursorCol, s.tiebreaker)
		q := s.query + clause + orderClause + " LIMIT ?"
		args := append(userArgs, s.cursor.Values[0], s.cursor.Values[1], s.batchSize)
		return q, args
	}

	// Single-column: WHERE ts > ?
	clause += s.cursorCol + " > ?"
	q := s.query + clause + orderClause + " LIMIT ?"
	args := append(userArgs, s.cursor.Value, s.batchSize)
	return q, args
}

func (s *CursorStream) orderClause() string {
	if s.tiebreaker != "" {
		return " ORDER BY " + s.cursorCol + ", " + s.tiebreaker
	}
	return " ORDER BY " + s.cursorCol
}

// Checkpoint returns the current cursor position. Safe for JSON serialization.
func (s *CursorStream) Checkpoint() Cursor { return s.cursor }

// SetCheckpoint restores a previously saved cursor. A zero-value Cursor
// resets to the beginning.
func (s *CursorStream) SetCheckpoint(c Cursor) { s.cursor = c }

// SQL returns the query that would be executed on the next Next() call,
// with placeholder args expanded for debugging.
func (s *CursorStream) SQL(args ...any) string {
	q, a := s.buildQuery(args)
	return fmt.Sprintf("%s %v", q, a)
}

// ==========================================================================
// Poll — continuously fetch new data
// ==========================================================================

// WatchOptions controls adaptive polling.
type WatchOptions struct {
	MinInterval   time.Duration // default 100ms
	MaxInterval   time.Duration // default 5s
	BackoffFactor float64       // default 2.0
	NoJitter      bool
}

func (o *WatchOptions) normalize() {
	if o.MinInterval <= 0 {
		o.MinInterval = 100 * time.Millisecond
	}
	if o.MaxInterval <= 0 {
		o.MaxInterval = 5 * time.Second
	}
	if o.BackoffFactor <= 0 {
		o.BackoffFactor = 2.0
	}
}

// Poll continuously fetches new data with adaptive backoff.
// Each batch is delivered to fn. Blocks until ctx is cancelled.
// Query args are passed through to every Next() call.
//
// Adaptive algorithm:
//   - Data flowing → back-to-back batches, reset to MinInterval.
//   - No data → interval grows: min(interval × factor, max).
//   - Optional jitter (±25%) prevents thundering herd.
//
// Usage:
//
//	go stream.Poll(ctx, ck.WatchOptions{}, func(batch []ck.Row) error {
//	    for _, row := range batch { handle(row) }
//	    return nil
//	}, "click")
func (s *CursorStream) Poll(ctx context.Context, opts WatchOptions, fn func([]Row) error, args ...any) error {
	opts.normalize()
	interval := opts.MinInterval

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		anyData := false
		for {
			batch, err := s.Next(ctx, args...)
			if err != nil {
				return err
			}
			if len(batch) == 0 {
				break
			}
			anyData = true
			if err := fn(batch); err != nil {
				return err
			}
		}

		if anyData {
			interval = opts.MinInterval
		}

		wait := interval
		if !opts.NoJitter {
			wait = time.Duration(float64(wait) * (0.75 + rand.Float64()*0.5))
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}

		if !anyData {
			interval = time.Duration(float64(interval) * opts.BackoffFactor)
			if interval > opts.MaxInterval {
				interval = opts.MaxInterval
			}
		}
	}
}

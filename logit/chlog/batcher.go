package chlog

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/izliang/bore/logit"
	"github.com/izliang/bore/logit/lctx"
)

// row is a single log entry queued for writing.
type row map[string]any

// batcher collects rows and flushes them to ClickHouse in batches.
type batcher struct {
	db        *sql.DB
	database  string
	table     string
	batchSize int
	interval  time.Duration
	onError   func(error)

	ch     chan row
	wg     sync.WaitGroup
	closed bool
	mu     sync.Mutex
}

func newBatcher(db *sql.DB, database, table string, batchSize int, interval time.Duration, onError func(error)) *batcher {
	b := &batcher{
		db:        db,
		database:  database,
		table:     table,
		batchSize: batchSize,
		interval:  interval,
		onError:   onError,
		ch:        make(chan row, batchSize*2),
	}
	b.wg.Add(1)
	go b.loop()
	return b
}

func (b *batcher) push(r row) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.mu.Unlock()
	b.ch <- r
}

// close stops the batcher and flushes remaining rows.
func (b *batcher) close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	b.mu.Unlock()
	close(b.ch)
	b.wg.Wait()
}

func (b *batcher) loop() {
	defer b.wg.Done()

	ticker := time.NewTicker(b.interval)
	defer ticker.Stop()

	buf := make([]row, 0, b.batchSize)

	flush := func() {
		if len(buf) == 0 {
			return
		}
		if err := b.flush(buf); err != nil && b.onError != nil {
			b.onError(err)
		}
		buf = buf[:0]
	}

	for {
		select {
		case r, ok := <-b.ch:
			if !ok {
				flush()
				return
			}
			buf = append(buf, r)
			if len(buf) >= b.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

func (b *batcher) flush(rows []row) error {
	if len(rows) == 0 {
		return nil
	}

	// Group rows by their column set — SkipEmpty may cause rows within
	// the same batch to have different columns. Each group gets its own INSERT.
	groups := groupByColumns(rows)
	for _, g := range groups {
		if err := b.flushGroup(g.cols, g.rows); err != nil {
			return err
		}
	}
	return nil
}

type columnGroup struct {
	cols []string
	rows []row
}

func groupByColumns(rows []row) []columnGroup {
	seen := make(map[string]*columnGroup)
	order := make([]*columnGroup, 0)

	for _, r := range rows {
		colList := make([]string, 0, len(r))
		for c := range r {
			colList = append(colList, c)
		}
		// Sort for stable grouping.
		for i := 0; i < len(colList)-1; i++ {
			for j := i + 1; j < len(colList); j++ {
				if colList[i] > colList[j] {
					colList[i], colList[j] = colList[j], colList[i]
				}
			}
		}
		fp := strings.Join(colList, "\x00")

		g, ok := seen[fp]
		if !ok {
			g = &columnGroup{cols: colList}
			seen[fp] = g
			order = append(order, g)
		}
		g.rows = append(g.rows, r)
	}

	out := make([]columnGroup, len(order))
	for i, g := range order {
		out[i] = *g
	}
	return out
}

func (b *batcher) flushGroup(cols []string, rows []row) error {
	quoted := make([]string, len(cols))
	placeholders := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = "`" + c + "`"
		placeholders[i] = "?"
	}

	query := fmt.Sprintf(
		"INSERT INTO `%s`.`%s` (%s) VALUES (%s)",
		b.database, b.table,
		strings.Join(quoted, ", "),
		strings.Join(placeholders, ", "),
	)

	for _, r := range rows {
		vals := make([]any, len(cols))
		for i, c := range cols {
			vals[i] = r[c]
		}
		if _, err := b.db.Exec(query, vals...); err != nil {
			return fmt.Errorf("chlog: flush INSERT: %w", err)
		}
	}
	return nil
}

// collectLctxPairs extracts all lctx key-value pairs from the context.
// Returns nil when ctx is nil.
func collectLctxPairs(ctx context.Context) map[string]string {
	if ctx == nil {
		return nil
	}
	pairs := lctx.ListPair(ctx)
	if len(pairs) == 0 {
		return nil
	}
	out := make(map[string]string, len(pairs))
	for _, p := range pairs {
		out[p.Key] = p.Value
	}
	return out
}

// collectFieldVals calls each FieldFunc and collects key→value pairs.
func collectFieldVals(fields []logit.FieldFunc) map[string]string {
	if len(fields) == 0 {
		return nil
	}
	out := make(map[string]string, len(fields))
	for _, f := range fields {
		if f == nil {
			continue
		}
		k, v := f().Value()
		out[k] = v
	}
	return out
}

// buildRow assembles a row from a log call.
//
// Order of column population:
//  1. TimeColumn — always set to time.Now()
//  2. RawColumn — full formatted log line, if configured
//  3. Custom Columns — matched from lctx pairs first, then field funcs
func buildRow(cfg *Config, level logit.Level, msg string, ctx context.Context, fields []logit.FieldFunc, seq uint64) row {
	r := make(row)

	// Timestamp column — always written.
	now := time.Now().In(cfg.Location)
	r[cfg.TimeColumn] = now.UTC().Format("2006-01-02 15:04:05.000")

	// Raw formatted line.
	if cfg.RawColumn != "" {
		r[cfg.RawColumn] = buildRaw(now, level, msg, ctx, fields)
	}

	// Sequence column — tiebreaker for identical timestamps.
	if cfg.SeqColumn != "" {
		r[cfg.SeqColumn] = seq
	}
	// Custom columns from lctx pairs and field funcs.
	if len(cfg.Columns) > 0 {
		lctxVals := collectLctxPairs(ctx)
		fieldVals := collectFieldVals(fields)

		for _, m := range cfg.Columns {
			var val string
			var found bool
			if v, ok := lctxVals[m.Key]; ok {
				val = v
				found = true
			} else if v, ok := fieldVals[m.Key]; ok {
				val = v
				found = true
			}
			if found {
				// SkipEmpty: only write to CK when the value is non-empty.
				// Stdout (buildRaw) always includes all fields regardless.
				if m.SkipEmpty && val == "" {
					continue
				}
				r[m.Column] = convertValue(val, m.Type)
			}
		}
	}

	return r
}

// buildRaw produces a formatted log line in key[value] style.
func buildRaw(now time.Time, level logit.Level, msg string, ctx context.Context, fields []logit.FieldFunc) string {
	var b strings.Builder

	b.WriteString(now.Format("2006-01-02 15:04:05.000"))
	b.WriteByte(' ')
	b.WriteByte('[')
	b.WriteString(string(level))
	b.WriteByte(']')

	lctxVals := collectLctxPairs(ctx)
	for k, v := range lctxVals {
		b.WriteByte(' ')
		b.WriteString(k)
		b.WriteByte('[')
		b.WriteString(v)
		b.WriteByte(']')
	}

	b.WriteString(" msg[")
	b.WriteString(msg)
	b.WriteByte(']')

	for _, f := range fields {
		if f == nil {
			continue
		}
		k, v := f().Value()
		b.WriteByte(' ')
		b.WriteString(k)
		b.WriteByte('[')
		b.WriteString(v)
		b.WriteByte(']')
	}

	return b.String()
}

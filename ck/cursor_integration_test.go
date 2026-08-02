//go:build integration
// +build integration

package ck_test

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/izliang/bore/ck"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func uniqueDBName() string {
	return fmt.Sprintf("ck_stream_test_%d", time.Now().UnixNano())
}

// TestCursorStream_FullLifecycle tests the complete streaming lifecycle:
//
//  1. Insert initial random data
//  2. Sync: consume all existing data via Next()
//  3. Start Poll for incremental data in background
//  4. Insert additional random data while polling
//  5. Verify all incremental data is captured
func TestCursorStream_FullLifecycle(t *testing.T) {
	ctx := context.Background()
	host := env("CH_HOST", "192.168.160.65:9001")
	user := env("CH_USER", "default")
	pass := os.Getenv("CH_PASS")

	conn, err := ck.Dial(host, user, pass, "default")
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	if err := conn.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	t.Logf("connected to %s", host)

	// ---------- setup: database + table ----------
	testDB := uniqueDBName()
	t.Logf("test database: %s", testDB)
	//defer func() {
	//	conn.Exec(context.Background(), "DROP DATABASE IF EXISTS `"+testDB+"`")
	//}()

	if err := conn.Exec(ctx, ck.CreateDatabaseSQL(testDB)); err != nil {
		t.Fatalf("CREATE DATABASE: %v", err)
	}

	def := ck.TableDef{
		Database: testDB,
		Table:    "events",
		Columns: []ck.ColumnDef{
			{Name: "ts", Type: "DateTime64(3)", Default: "now64(3)", Searchable: true},
			{Name: "seq", Type: "UInt64", Searchable: true},
			{Name: "user_id", Type: "String"},
			{Name: "event_type", Type: "String"},
			{Name: "payload", Type: "String"},
		},
		PartitionBy: "toYYYYMM(ts)",
		TTLDays:     7,
	}
	if err := conn.Exec(ctx, ck.CreateTableSQL(def)); err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	insertSQL := fmt.Sprintf(
		"INSERT INTO `%s`.events (ts, seq, user_id, event_type, payload) VALUES (?, ?, ?, ?, ?)",
		testDB,
	)

	// ---------- phase 1: insert initial batch ----------
	initialCount := 300
	eventTypes := []string{"click", "page_view", "purchase", "login", "logout"}
	userIDs := make([]string, 20)
	for i := range userIDs {
		userIDs[i] = fmt.Sprintf("user_%d", i)
	}

	t.Logf("phase 1: inserting %d initial rows...", initialCount)
	baseTS := time.Now().Add(-2 * time.Hour)
	for i := 0; i < initialCount; i++ {
		ts := baseTS.Add(time.Duration(i) * time.Second)
		uid := userIDs[rng.Intn(len(userIDs))]
		et := eventTypes[rng.Intn(len(eventTypes))]
		pl := fmt.Sprintf("initial_payload_%d", i)
		if err := conn.Exec(ctx, insertSQL, ts, uint64(i), uid, et, pl); err != nil {
			t.Fatalf("INSERT (initial): %v", err)
		}
	}

	// Let ClickHouse finalize inserts.
	time.Sleep(500 * time.Millisecond)

	// ---------- phase 2: sync — consume all existing data ----------
	t.Log("phase 2: syncing all existing data...")
	stream := conn.NewCursorStream(
		fmt.Sprintf("SELECT ts, seq, user_id, event_type, payload FROM `%s`.events", testDB),
		"ts", 100,
	)

	synced := 0
	var lastSeq uint64
	for {
		batch, err := stream.Next(ctx)
		if err != nil {
			t.Fatalf("Next (sync): %v", err)
		}
		if len(batch) == 0 {
			break
		}
		for _, row := range batch {
			synced++
			// Track the last seq value for verification.
			if s, ok := row["seq"]; ok {
				fmt.Sscanf(s.(string), "%d", &lastSeq)
			}
		}
		t.Logf("  synced batch: %d rows (total: %d)", len(batch), synced)
	}
	t.Logf("sync complete: %d rows (expected %d), cursor at ts=%v",
		synced, initialCount, stream.Checkpoint().Values)

	if synced != initialCount {
		t.Fatalf("synced %d rows, expected %d", synced, initialCount)
	}

	// ---------- phase 3: start polling for incremental data ----------
	t.Log("phase 3: starting Poll for incremental data...")

	type capturedBatch struct {
		rows []ck.Row
		at   time.Time
	}
	var (
		mu               sync.Mutex
		incremental      []capturedBatch
		incrementalCount int
	)
	pollCtx, pollCancel := context.WithCancel(ctx)
	defer pollCancel()

	pollDone := make(chan error, 1)
	go func() {
		pollDone <- stream.Poll(pollCtx, ck.WatchOptions{
			MinInterval:   100 * time.Millisecond,
			MaxInterval:   2 * time.Second,
			BackoffFactor: 2.0,
		}, func(batch []ck.Row) error {
			mu.Lock()
			incremental = append(incremental, capturedBatch{rows: batch, at: time.Now()})
			incrementalCount += len(batch)
			mu.Unlock()
			t.Logf("  poll received batch: %d rows (total incr: %d)", len(batch), incrementalCount)
			return nil
		})
	}()

	// Wait for poll to stabilise (initial full sync already done, so
	// the first poll cycle should find no data and start backing off).
	time.Sleep(500 * time.Millisecond)

	// ---------- phase 4: insert incremental data while polling ----------
	incrementalInsertCount := 80
	t.Logf("phase 4: inserting %d incremental rows while polling...", incrementalInsertCount)
	for i := initialCount; i < initialCount+incrementalInsertCount; i++ {
		ts := baseTS.Add(time.Duration(i) * time.Second)
		uid := userIDs[rng.Intn(len(userIDs))]
		et := eventTypes[rng.Intn(len(eventTypes))]
		pl := fmt.Sprintf("incremental_payload_%d", i)
		if err := conn.Exec(ctx, insertSQL, ts, uint64(i), uid, et, pl); err != nil {
			t.Fatalf("INSERT (incremental): %v", err)
		}
		// Stagger inserts slightly so the poll has time to react.
		if i%20 == 0 {
			time.Sleep(100 * time.Millisecond)
		}
	}

	// Wait for poll to pick up the incremental data.
	// With MinInterval=100ms and small batches, this should happen quickly.
	t.Log("phase 5: waiting for poll to capture incremental data...")
	deadline := time.Now().Add(15 * time.Second)
	for {
		mu.Lock()
		count := incrementalCount
		mu.Unlock()
		if count >= incrementalInsertCount {
			t.Logf("all %d incremental rows captured", count)
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for incremental data: got %d/%d rows",
				count, incrementalInsertCount)
		}
		time.Sleep(200 * time.Millisecond)
	}

	// ---------- phase 5: verify ----------
	pollCancel()
	select {
	case err := <-pollDone:
		if err != nil && err != context.Canceled {
			t.Errorf("Poll error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("Poll did not exit after cancel")
	}

	mu.Lock()
	var batchDetails string
	for i, cb := range incremental {
		if i < 5 || i >= len(incremental)-2 {
			batchDetails += fmt.Sprintf("\n    batch %d: %d rows at %s", i, len(cb.rows), cb.at.Format("15:04:05.000"))
		} else if i == 5 {
			batchDetails += fmt.Sprintf("\n    ... %d batches omitted ...", len(incremental)-7)
		}
	}
	mu.Unlock()
	t.Logf("incremental batches captured: %d batches, %d total rows%s",
		len(incremental), incrementalCount, batchDetails)

	if incrementalCount < incrementalInsertCount {
		t.Errorf("incremental rows: %d, expected >= %d", incrementalCount, incrementalInsertCount)
	}

	t.Log("cleanup: dropping test database")
	conn.Exec(context.Background(), "DROP TABLE IF EXISTS `"+testDB+"`.events")
	t.Log("TestCursorStream_FullLifecycle: PASSED")
}

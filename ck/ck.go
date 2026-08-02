// Package ck provides ClickHouse schema DDL generation, native connection, and cursor-based streaming.
//
// # DDL
//
//	def := ck.TableDef{
//	    Database: "mydb", Table: "events",
//	    Columns: []ck.ColumnDef{
//	        {Name: "ts", Type: "DateTime64(3)", Default: "now64(3)", Searchable: true},
//	        {Name: "user_id", Type: "String", Searchable: true},
//	        {Name: "event", Type: "String"},
//	    },
//	    PartitionBy: "toYYYYMMDD(ts)",
//	    TTLDays:     90,
//	}
//	conn.Exec(ctx, ck.CreateTableSQL(def))
//
// # Connection
//
//	conn, _ := ck.Dial("host:9000", "admin", "secret", "mydb")
//	defer conn.Close()
//
// # Cursor Streaming
//
//	stream := conn.NewCursorStream("SELECT * FROM events WHERE type = ?", "ts", 500)
//
//	// Sync: get all existing data.
//	for {
//	    batch, err := stream.Next(ctx, "click")
//	    if len(batch) == 0 { break }
//	    for _, row := range batch { ... }
//	}
//
//	// Save checkpoint for restart.
//	saveCheckpoint(stream.Checkpoint())
//
//	// Poll: continuously fetch new data.
//	go stream.Poll(ctx, ck.WatchOptions{}, func(batch []ck.Row) error { ... }, "click")
package ck

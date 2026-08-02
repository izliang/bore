package ck

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	chdriver "github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// Conn wraps a native clickhouse-go connection.
type Conn struct {
	conn chdriver.Conn
}

// Dial opens a native ClickHouse connection.
func Dial(addr, user, password, database string) (*Conn, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{addr},
		Auth: clickhouse.Auth{Database: database, Username: user, Password: password},
		Settings: clickhouse.Settings{"max_block_size": 100000},
	})
	if err != nil {
		return nil, fmt.Errorf("ck: dial %s: %w", addr, err)
	}
	return &Conn{conn: conn}, nil
}

// DialOptions opens a connection with full clickhouse.Options control.
func DialOptions(opts *clickhouse.Options) (*Conn, error) {
	conn, err := clickhouse.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("ck: dial: %w", err)
	}
	return &Conn{conn: conn}, nil
}

// Ping verifies the connection is alive.
func (c *Conn) Ping(ctx context.Context) error { return c.conn.Ping(ctx) }

// Close closes the connection.
func (c *Conn) Close() error { return c.conn.Close() }

// Exec executes a DDL/DML statement.
func (c *Conn) Exec(ctx context.Context, query string, args ...any) error {
	return c.conn.Exec(ctx, query, args...)
}

// RawConn exposes the underlying chdriver.Conn.
func (c *Conn) RawConn() chdriver.Conn { return c.conn }

// queryRows executes a query and returns a rowIterator.
func (c *Conn) queryRows(ctx context.Context, query string, args ...any) (*rowIterator, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	rows, err := c.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("ck: query: %w", err)
	}
	return newRowIterator(rows), nil
}

// rowIterator scans rows into Row maps.
type rowIterator struct {
	rows          chdriver.Rows
	columns       []string
	receiverTypes []reflect.Type
}

func newRowIterator(rows chdriver.Rows) *rowIterator {
	columns := rows.Columns()
	colTypes := rows.ColumnTypes()
	n := len(columns)
	receiverTypes := make([]reflect.Type, n)
	for i, ct := range colTypes {
		receiverTypes[i] = ct.ScanType()
	}
	return &rowIterator{rows: rows, columns: columns, receiverTypes: receiverTypes}
}

func (it *rowIterator) next() (Row, error) {
	if !it.rows.Next() {
		return nil, it.rows.Err()
	}
	n := len(it.columns)
	ptrs := make([]any, n)
	receivers := make([]reflect.Value, n)
	for i := range n {
		v := reflect.New(it.receiverTypes[i])
		receivers[i] = v
		ptrs[i] = v.Interface()
	}
	if err := it.rows.Scan(ptrs...); err != nil {
		return nil, fmt.Errorf("ck: scan: %w", err)
	}
	row := make(Row, n)
	for i, col := range it.columns {
		row[col] = formatRowValue(receivers[i].Elem())
	}
	return row, nil
}

func (it *rowIterator) close() error { return it.rows.Close() }

func formatRowValue(v reflect.Value) string {
	switch v.Kind() {
	case reflect.String:
		return v.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return fmt.Sprintf("%d", v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return fmt.Sprintf("%d", v.Uint())
	case reflect.Float32, reflect.Float64:
		return fmt.Sprintf("%v", v.Float())
	case reflect.Bool:
		if v.Bool() {
			return "true"
		}
		return "false"
	}
	if v.Type() == reflect.TypeOf(time.Time{}) {
		return v.Interface().(time.Time).Format("2006-01-02 15:04:05.000")
	}
	return fmt.Sprint(v.Interface())
}

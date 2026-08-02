package ck

import (
	"fmt"
	"regexp"
	"strings"
)

var identRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func validateIdent(name, kind string) error {
	if !identRe.MatchString(name) {
		return fmt.Errorf("ck: invalid %s %q", kind, name)
	}
	return nil
}

// ColumnDef describes a table column.
type ColumnDef struct {
	Name       string // column name
	Type       string // ClickHouse type, e.g. "String", "DateTime64(3)", "LowCardinality(String)"
	Default    string // SQL DEFAULT expression
	Searchable bool   // adds to ORDER BY and creates bloom_filter skipping index
}

// TableDef describes a ClickHouse table.
type TableDef struct {
	Database    string
	Table       string
	Columns     []ColumnDef
	Engine      string   // default "MergeTree"
	PartitionBy string
	OrderBy     []string // explicit ORDER BY; auto-generated from Searchable columns if empty
	TTLDays     int
}

func (d TableDef) engine() string {
	if d.Engine == "" {
		return "MergeTree"
	}
	return d.Engine
}

// Row is a map from column name to value.
type Row map[string]any

// Validate checks the table definition for correctness.
func (d TableDef) Validate() error {
	if d.Database == "" {
		return fmt.Errorf("ck: database name required")
	}
	if d.Table == "" {
		return fmt.Errorf("ck: table name required")
	}
	if err := validateIdent(d.Database, "database"); err != nil {
		return err
	}
	if err := validateIdent(d.Table, "table"); err != nil {
		return err
	}
	for _, col := range d.Columns {
		if err := validateIdent(col.Name, "column"); err != nil {
			return err
		}
	}
	return nil
}

// CreateDatabaseSQL returns "CREATE DATABASE IF NOT EXISTS `db`".
func CreateDatabaseSQL(database string) string {
	return "CREATE DATABASE IF NOT EXISTS " + quoteIdent(database)
}

// CreateTableSQL generates a CREATE TABLE statement from the definition.
//
//   - Searchable columns appear in ORDER BY.
//   - Searchable columns get bloom_filter skipping indexes (inside column list).
//   - Explicit OrderBy overrides the auto-generated one.
//   - TTLDays > 0 adds a TTL clause on the first DateTime column.
func CreateTableSQL(def TableDef) string {
	var b strings.Builder
	q := quoteIdent(def.Database) + "." + quoteIdent(def.Table)
	b.WriteString("CREATE TABLE IF NOT EXISTS ")
	b.WriteString(q)
	b.WriteString(" (\n    ")

	// Collect column definitions and index definitions.
	parts := make([]string, 0, len(def.Columns)*2)
	for _, c := range def.Columns {
		s := quoteIdent(c.Name) + " " + c.Type
		if c.Default != "" {
			s += " DEFAULT " + c.Default
		}
		parts = append(parts, s)

		// Bloom filter index goes inside the column list, not after ENGINE.
		// The column reference must be quoted to handle special characters.
		if c.Searchable && supportsBloomFilter(c.Type) {
			idx := fmt.Sprintf("INDEX idx_%s %s TYPE bloom_filter GRANULARITY 1",
				c.Name, quoteIdent(c.Name))
			parts = append(parts, idx)
		}
	}
	b.WriteString(strings.Join(parts, ",\n    "))

	b.WriteString("\n) ENGINE = ")
	b.WriteString(def.engine())

	if def.PartitionBy != "" {
		b.WriteString("\nPARTITION BY ")
		b.WriteString(def.PartitionBy)
	}

	order := def.OrderBy
	if len(order) == 0 {
		for _, c := range def.Columns {
			if c.Searchable {
				order = append(order, c.Name)
			}
		}
	}
	if len(order) > 0 {
		b.WriteString("\nORDER BY (")
		b.WriteString(strings.Join(order, ", "))
		b.WriteString(")")
	}

	if def.TTLDays > 0 {
		ttlCol := pickTTLCol(def.Columns)
		if ttlCol != "" {
			_, _ = fmt.Fprintf(&b, "\nTTL toDateTime(%s) + INTERVAL %d DAY", quoteIdent(ttlCol), def.TTLDays)
		}
	}

	return b.String()
}

// pickTTLCol returns the name of the first DateTime column, or empty string
// if there are no columns.
func pickTTLCol(cols []ColumnDef) string {
	// Prefer exact DateTime over DateTime64 — ClickHouse TTL requires DateTime/Date.
	for _, c := range cols {
		if c.Type == "DateTime" {
			return c.Name
		}
	}
	for _, c := range cols {
		if strings.HasPrefix(c.Type, "DateTime") {
			return c.Name
		}
	}
	if len(cols) > 0 {
		return cols[0].Name
	}
	return ""
}

func quoteIdent(name string) string {
	return "`" + name + "`"
}

// supportsBloomFilter returns true if the ClickHouse column type supports
// bloom_filter skipping indexes. DateTime/DateTime64/Date types do not.
func supportsBloomFilter(typ string) bool {
	t := strings.ToUpper(strings.TrimSpace(typ))
	// Time-like types don't support bloom_filter.
	if strings.HasPrefix(t, "DATETIME") || strings.HasPrefix(t, "DATE") {
		return false
	}
	return true
}

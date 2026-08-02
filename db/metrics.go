package db

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"gorm.io/gorm"
)

// DBStats implements prometheus.Collector and exposes connection-pool
// statistics from *sql.DB.Stats(). It also implements metrics.Provider,
// allowing it to be composed into a ServiceMetrics struct and registered
// via metrics.MustInit.
//
// Usage (standalone):
//
//	db, _ := db.Open(cfg)
//	dbStats := db.NewDBStats(db, db.WithDBName("primary"))
//	prometheus.MustRegister(dbStats)
//
// Usage (with bore metrics):
//
//	db, _ := db.Open(cfg)
//	reg := prometheus.NewRegistry()
//	dbStats := db.NewDBStats(db, db.WithDBName("primary"))
//	metrics.MustRegister(reg, dbStats)
type DBStats struct {
	db *gorm.DB

	// cached descriptor copies for Describe.
	descs []*prometheus.Desc
}

// dbStatsConfig holds internal configuration set via DBStatsOption.
type dbStatsConfig struct {
	namespace   string
	subsystem   string
	constLabels prometheus.Labels
}

// DBStatsOption customizes DBStats behavior and metric naming.
// Follows the same functional-options pattern used by provider.HTTP.
type DBStatsOption func(*dbStatsConfig)

// WithDBNamespace sets the Prometheus metric namespace prefix.
func WithDBNamespace(namespace string) DBStatsOption {
	return func(c *dbStatsConfig) {
		c.namespace = strings.TrimSpace(namespace)
	}
}

// WithDBSubsystem sets the Prometheus metric subsystem prefix.
func WithDBSubsystem(subsystem string) DBStatsOption {
	return func(c *dbStatsConfig) {
		c.subsystem = strings.TrimSpace(subsystem)
	}
}

// WithDBConstLabels sets constant labels applied to every metric.
// Labels are copied so the caller retains ownership.
func WithDBConstLabels(labels prometheus.Labels) DBStatsOption {
	return func(c *dbStatsConfig) {
		cl := make(prometheus.Labels, len(labels))
		for k, v := range labels {
			cl[k] = v
		}
		c.constLabels = cl
	}
}

// WithDBName sets the "db_name" const label — a convenience wrapper around
// WithDBConstLabels. This is the most common label for differentiating
// multiple database connections in the same registry.
func WithDBName(name string) DBStatsOption {
	return WithDBConstLabels(prometheus.Labels{"db": name})
}

// NewDBStats creates a DBStats for the given *gorm.DB.
//
// Options allow setting Prometheus namespace, subsystem, const labels and
// the conventional "db_name" label. Without WithDBName the metrics carry no
// db_name label; callers should supply one when multiple databases are
// registered in the same registry.
func NewDBStats(db *gorm.DB, opts ...DBStatsOption) *DBStats {
	cfg := &dbStatsConfig{}
	for _, o := range opts {
		o(cfg)
	}

	fqn := fullyQualifiedName(cfg.namespace, cfg.subsystem)

	descs := []*prometheus.Desc{
		prometheus.NewDesc(
			fqn("db_connections_max_open"),
			"Maximum number of open connections to the database.",
			nil, cfg.constLabels,
		),
		prometheus.NewDesc(
			fqn("db_connections_open"),
			"Number of established connections (in use + idle).",
			nil, cfg.constLabels,
		),
		prometheus.NewDesc(
			fqn("db_connections_in_use"),
			"Number of connections currently in use.",
			nil, cfg.constLabels,
		),
		prometheus.NewDesc(
			fqn("db_connections_idle"),
			"Number of idle connections.",
			nil, cfg.constLabels,
		),
		prometheus.NewDesc(
			fqn("db_wait_count_total"),
			"Total number of connections waited for.",
			nil, cfg.constLabels,
		),
		prometheus.NewDesc(
			fqn("db_wait_duration_seconds_total"),
			"Total time blocked waiting for a new connection.",
			nil, cfg.constLabels,
		),
		prometheus.NewDesc(
			fqn("db_connections_max_idle_closed_total"),
			"Total number of connections closed due to SetMaxIdleConns.",
			nil, cfg.constLabels,
		),
		prometheus.NewDesc(
			fqn("db_connections_max_lifetime_closed_total"),
			"Total number of connections closed due to SetConnMaxLifetime.",
			nil, cfg.constLabels,
		),
	}

	return &DBStats{db: db, descs: descs}
}

// GetCollectors implements metrics.Provider, returning the DBStats itself
// as the sole collector. This makes DBStats composable with metrics.MustInit.
func (m *DBStats) GetCollectors() []prometheus.Collector {
	return []prometheus.Collector{m}
}

// Describe implements prometheus.Collector.
func (m *DBStats) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range m.descs {
		ch <- d
	}
}

// Collect implements prometheus.Collector. It samples sql.DB.Stats() and
// emits one metric per descriptor. When the underlying *gorm.DB is nil the
// method is a no-op and emits nothing.
func (m *DBStats) Collect(ch chan<- prometheus.Metric) {
	if m.db == nil {
		return
	}
	sqlDB, err := m.db.DB()
	if err != nil {
		return
	}
	s := sqlDB.Stats()

	emit := func(i int, v float64, vt prometheus.ValueType) {
		ch <- prometheus.MustNewConstMetric(m.descs[i], vt, v)
	}

	emit(0, float64(s.MaxOpenConnections), prometheus.GaugeValue)
	emit(1, float64(s.OpenConnections), prometheus.GaugeValue)
	emit(2, float64(s.InUse), prometheus.GaugeValue)
	emit(3, float64(s.Idle), prometheus.GaugeValue)
	emit(4, float64(s.WaitCount), prometheus.CounterValue)
	emit(5, s.WaitDuration.Seconds(), prometheus.CounterValue)
	emit(6, float64(s.MaxIdleClosed), prometheus.CounterValue)
	emit(7, float64(s.MaxLifetimeClosed), prometheus.CounterValue)
}

// fullyQualifiedName builds a fully-qualified Prometheus metric name from
// namespace and subsystem following the same convention as client_golang:
//
//	namespace_subsystem_name   (both set)
//	namespace_name             (only namespace)
//	subsystem_name             (only subsystem)
//	name                       (neither)
func fullyQualifiedName(namespace, subsystem string) func(name string) string {
	switch {
	case namespace != "" && subsystem != "":
		return func(name string) string { return namespace + "_" + subsystem + "_" + name }
	case namespace != "":
		return func(name string) string { return namespace + "_" + name }
	case subsystem != "":
		return func(name string) string { return subsystem + "_" + name }
	default:
		return func(name string) string { return name }
	}
}

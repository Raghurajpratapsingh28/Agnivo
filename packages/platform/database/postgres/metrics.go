package postgres

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics exposes pool statistics and query instrumentation to Prometheus. Pool
// gauges are sampled lazily at scrape time via a custom collector, so they add
// no steady-state overhead.
type Metrics struct {
	queryDuration *prometheus.HistogramVec
	queryErrors   *prometheus.CounterVec
	txTotal       *prometheus.CounterVec

	poolCollector *poolCollector
	collectors    []prometheus.Collector
}

// NewMetrics builds the database metrics, namespaced by service.
func NewMetrics(service string) *Metrics {
	labels := prometheus.Labels{"service": service}

	queryDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:   "agnivo",
		Subsystem:   "db",
		Name:        "query_duration_seconds",
		Help:        "Database query latency in seconds.",
		Buckets:     []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
		ConstLabels: labels,
	}, []string{"op"})

	queryErrors := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   "agnivo",
		Subsystem:   "db",
		Name:        "query_errors_total",
		Help:        "Total database query errors by classified code.",
		ConstLabels: labels,
	}, []string{"op", "code"})

	txTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   "agnivo",
		Subsystem:   "db",
		Name:        "transactions_total",
		Help:        "Total transactions by outcome.",
		ConstLabels: labels,
	}, []string{"outcome"})

	pc := &poolCollector{labels: labels}
	m := &Metrics{
		queryDuration: queryDuration,
		queryErrors:   queryErrors,
		txTotal:       txTotal,
		poolCollector: pc,
	}
	m.collectors = []prometheus.Collector{queryDuration, queryErrors, txTotal, pc}
	return m
}

// Collectors returns every collector for registration with a metrics registry.
func (m *Metrics) Collectors() []prometheus.Collector { return m.collectors }

// bindPool wires the live pool into the lazy pool collector.
func (m *Metrics) bindPool(pool *pgxpool.Pool) {
	if m == nil {
		return
	}
	m.poolCollector.set(pool)
}

// ObserveQuery records a query's latency and, on failure, its classified code.
func (m *Metrics) ObserveQuery(op string, seconds float64, err error) {
	if m == nil {
		return
	}
	m.queryDuration.WithLabelValues(op).Observe(seconds)
	if err != nil {
		m.queryErrors.WithLabelValues(op, string(classify(err))).Inc()
	}
}

// ObserveTx records a transaction outcome ("commit" or "rollback").
func (m *Metrics) ObserveTx(outcome string) {
	if m == nil {
		return
	}
	m.txTotal.WithLabelValues(outcome).Inc()
}

// poolCollector lazily samples pgxpool.Stat at scrape time.
type poolCollector struct {
	labels prometheus.Labels
	pool   *pgxpool.Pool

	acquired    *prometheus.Desc
	idle        *prometheus.Desc
	total       *prometheus.Desc
	max         *prometheus.Desc
	constructed *prometheus.Desc
}

func (c *poolCollector) set(pool *pgxpool.Pool) {
	c.pool = pool
	mk := func(name, help string) *prometheus.Desc {
		return prometheus.NewDesc("agnivo_db_pool_"+name, help, nil, c.labels)
	}
	c.acquired = mk("acquired_conns", "Connections currently acquired from the pool.")
	c.idle = mk("idle_conns", "Idle connections in the pool.")
	c.total = mk("total_conns", "Total connections in the pool.")
	c.max = mk("max_conns", "Maximum connections allowed.")
	c.constructed = mk("constructed_conns_total", "Connections constructed over the pool's lifetime.")
}

// Describe implements prometheus.Collector.
func (c *poolCollector) Describe(ch chan<- *prometheus.Desc) {
	if c.pool == nil {
		return
	}
	ch <- c.acquired
	ch <- c.idle
	ch <- c.total
	ch <- c.max
	ch <- c.constructed
}

// Collect implements prometheus.Collector, sampling pool stats on demand.
func (c *poolCollector) Collect(ch chan<- prometheus.Metric) {
	if c.pool == nil {
		return
	}
	s := c.pool.Stat()
	ch <- prometheus.MustNewConstMetric(c.acquired, prometheus.GaugeValue, float64(s.AcquiredConns()))
	ch <- prometheus.MustNewConstMetric(c.idle, prometheus.GaugeValue, float64(s.IdleConns()))
	ch <- prometheus.MustNewConstMetric(c.total, prometheus.GaugeValue, float64(s.TotalConns()))
	ch <- prometheus.MustNewConstMetric(c.max, prometheus.GaugeValue, float64(s.MaxConns()))
	ch <- prometheus.MustNewConstMetric(c.constructed, prometheus.CounterValue, float64(s.NewConnsCount()))
}

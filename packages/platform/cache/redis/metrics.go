package redis

import (
	goredis "github.com/redis/go-redis/v9"
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics exposes Redis pool statistics and command instrumentation. Pool
// gauges are sampled lazily at scrape time so they impose no steady overhead.
type Metrics struct {
	cmdDuration *prometheus.HistogramVec
	cmdErrors   *prometheus.CounterVec

	poolCollector *redisPoolCollector
	collectors    []prometheus.Collector
}

// NewMetrics builds Redis metrics namespaced by service.
func NewMetrics(service string) *Metrics {
	labels := prometheus.Labels{"service": service}

	cmdDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:   "agnivo",
		Subsystem:   "redis",
		Name:        "command_duration_seconds",
		Help:        "Redis command latency in seconds.",
		Buckets:     []float64{.0005, .001, .0025, .005, .01, .025, .05, .1, .25, .5, 1},
		ConstLabels: labels,
	}, []string{"command"})

	cmdErrors := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   "agnivo",
		Subsystem:   "redis",
		Name:        "command_errors_total",
		Help:        "Total Redis command errors.",
		ConstLabels: labels,
	}, []string{"command"})

	pc := &redisPoolCollector{labels: labels}
	m := &Metrics{cmdDuration: cmdDuration, cmdErrors: cmdErrors, poolCollector: pc}
	m.collectors = []prometheus.Collector{cmdDuration, cmdErrors, pc}
	return m
}

// Collectors returns every collector for registration with a metrics registry.
func (m *Metrics) Collectors() []prometheus.Collector { return m.collectors }

func (m *Metrics) bindClient(client *goredis.Client) {
	if m == nil {
		return
	}
	m.poolCollector.set(client)
}

// ObserveCommand records a command's latency and increments the error counter
// on failure.
func (m *Metrics) ObserveCommand(command string, seconds float64, err error) {
	if m == nil {
		return
	}
	m.cmdDuration.WithLabelValues(command).Observe(seconds)
	if err != nil {
		m.cmdErrors.WithLabelValues(command).Inc()
	}
}

type redisPoolCollector struct {
	labels prometheus.Labels
	client *goredis.Client

	hits     *prometheus.Desc
	misses   *prometheus.Desc
	timeouts *prometheus.Desc
	total    *prometheus.Desc
	idle     *prometheus.Desc
	stale    *prometheus.Desc
}

func (c *redisPoolCollector) set(client *goredis.Client) {
	c.client = client
	mk := func(name, help string) *prometheus.Desc {
		return prometheus.NewDesc("agnivo_redis_pool_"+name, help, nil, c.labels)
	}
	c.hits = mk("hits_total", "Pool hits where a free connection was found.")
	c.misses = mk("misses_total", "Pool misses requiring a new connection.")
	c.timeouts = mk("timeouts_total", "Pool acquisition timeouts.")
	c.total = mk("total_conns", "Total connections in the pool.")
	c.idle = mk("idle_conns", "Idle connections in the pool.")
	c.stale = mk("stale_conns_total", "Stale connections removed by the pool.")
}

// Describe implements prometheus.Collector.
func (c *redisPoolCollector) Describe(ch chan<- *prometheus.Desc) {
	if c.client == nil {
		return
	}
	ch <- c.hits
	ch <- c.misses
	ch <- c.timeouts
	ch <- c.total
	ch <- c.idle
	ch <- c.stale
}

// Collect implements prometheus.Collector, sampling pool stats on demand.
func (c *redisPoolCollector) Collect(ch chan<- prometheus.Metric) {
	if c.client == nil {
		return
	}
	s := c.client.PoolStats()
	ch <- prometheus.MustNewConstMetric(c.hits, prometheus.CounterValue, float64(s.Hits))
	ch <- prometheus.MustNewConstMetric(c.misses, prometheus.CounterValue, float64(s.Misses))
	ch <- prometheus.MustNewConstMetric(c.timeouts, prometheus.CounterValue, float64(s.Timeouts))
	ch <- prometheus.MustNewConstMetric(c.total, prometheus.GaugeValue, float64(s.TotalConns))
	ch <- prometheus.MustNewConstMetric(c.idle, prometheus.GaugeValue, float64(s.IdleConns))
	ch <- prometheus.MustNewConstMetric(c.stale, prometheus.CounterValue, float64(s.StaleConns))
}

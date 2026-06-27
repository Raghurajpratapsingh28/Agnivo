// Package metrics provides a shared Prometheus registry preloaded with Go
// runtime and process collectors, plus reusable HTTP server metrics.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// Registry wraps a Prometheus registry with the platform's default collectors
// and standard HTTP instruments.
type Registry struct {
	prom *prometheus.Registry

	httpRequests *prometheus.CounterVec
	httpDuration *prometheus.HistogramVec
	httpInflight *prometheus.GaugeVec
}

// New creates a registry seeded with Go runtime and process metrics and the
// standard HTTP instruments, all namespaced by the service name.
func New(service string) *Registry {
	prom := prometheus.NewRegistry()
	prom.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	labels := prometheus.Labels{"service": service}

	httpRequests := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   "agnivo",
		Subsystem:   "http",
		Name:        "requests_total",
		Help:        "Total HTTP requests handled.",
		ConstLabels: labels,
	}, []string{"method", "route", "status"})

	httpDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:   "agnivo",
		Subsystem:   "http",
		Name:        "request_duration_seconds",
		Help:        "HTTP request latency in seconds.",
		Buckets:     prometheus.DefBuckets,
		ConstLabels: labels,
	}, []string{"method", "route", "status"})

	httpInflight := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace:   "agnivo",
		Subsystem:   "http",
		Name:        "requests_in_flight",
		Help:        "In-flight HTTP requests.",
		ConstLabels: labels,
	}, []string{"method", "route"})

	prom.MustRegister(httpRequests, httpDuration, httpInflight)

	return &Registry{
		prom:         prom,
		httpRequests: httpRequests,
		httpDuration: httpDuration,
		httpInflight: httpInflight,
	}
}

// Prometheus returns the underlying registry (for the /metrics handler).
func (r *Registry) Prometheus() *prometheus.Registry { return r.prom }

// MustRegister registers additional collectors owned by feature packages.
func (r *Registry) MustRegister(cs ...prometheus.Collector) { r.prom.MustRegister(cs...) }

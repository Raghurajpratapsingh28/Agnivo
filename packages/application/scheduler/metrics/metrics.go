package metrics

import "github.com/prometheus/client_golang/prometheus"

// Metrics instruments scheduler operations.
type Metrics struct {
	placementLatency  prometheus.Histogram
	placementSuccess  prometheus.Counter
	placementFailures prometheus.Counter
	activeServers     prometheus.Gauge
	activeReservations prometheus.Gauge
	collectors        []prometheus.Collector
}

// New builds scheduler metrics.
func New(service string) *Metrics {
	labels := prometheus.Labels{"service": service}
	placementLatency := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "agnivo", Subsystem: "scheduler", Name: "placement_latency_seconds",
		Buckets: prometheus.DefBuckets, ConstLabels: labels,
	})
	placementSuccess := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "agnivo", Subsystem: "scheduler", Name: "placement_success_total", ConstLabels: labels,
	})
	placementFailures := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "agnivo", Subsystem: "scheduler", Name: "placement_failures_total", ConstLabels: labels,
	})
	activeServers := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "agnivo", Subsystem: "scheduler", Name: "active_servers", ConstLabels: labels,
	})
	activeReservations := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "agnivo", Subsystem: "scheduler", Name: "active_reservations", ConstLabels: labels,
	})
	return &Metrics{
		placementLatency: placementLatency, placementSuccess: placementSuccess,
		placementFailures: placementFailures, activeServers: activeServers,
		activeReservations: activeReservations,
		collectors: []prometheus.Collector{
			placementLatency, placementSuccess, placementFailures, activeServers, activeReservations,
		},
	}
}

// Collectors returns Prometheus collectors.
func (m *Metrics) Collectors() []prometheus.Collector {
	if m == nil {
		return nil
	}
	return m.collectors
}

func (m *Metrics) ObservePlacement(seconds float64) {
	if m != nil {
		m.placementLatency.Observe(seconds)
	}
}

func (m *Metrics) IncPlacementSuccess() {
	if m != nil {
		m.placementSuccess.Inc()
	}
}

func (m *Metrics) IncPlacementFailure() {
	if m != nil {
		m.placementFailures.Inc()
	}
}

func (m *Metrics) SetActiveServers(n float64) {
	if m != nil {
		m.activeServers.Set(n)
	}
}

func (m *Metrics) SetActiveReservations(n float64) {
	if m != nil {
		m.activeReservations.Set(n)
	}
}

package metrics

import "github.com/prometheus/client_golang/prometheus"

// Metrics instruments runtime agent operations.
type Metrics struct {
	pullDuration    prometheus.Histogram
	createDuration  prometheus.Histogram
	startDuration   prometheus.Histogram
	activeContainers prometheus.Gauge
	cpuUsage        prometheus.Gauge
	memoryUsage     prometheus.Gauge
	collectors      []prometheus.Collector
}

// New builds runtime metrics.
func New(service string) *Metrics {
	labels := prometheus.Labels{"service": service}
	pullDuration := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "agnivo", Subsystem: "runtime", Name: "image_pull_duration_seconds",
		Buckets: prometheus.DefBuckets, ConstLabels: labels,
	})
	createDuration := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "agnivo", Subsystem: "runtime", Name: "container_create_duration_seconds",
		Buckets: prometheus.DefBuckets, ConstLabels: labels,
	})
	startDuration := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "agnivo", Subsystem: "runtime", Name: "container_start_duration_seconds",
		Buckets: prometheus.DefBuckets, ConstLabels: labels,
	})
	activeContainers := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "agnivo", Subsystem: "runtime", Name: "active_containers", ConstLabels: labels,
	})
	cpuUsage := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "agnivo", Subsystem: "runtime", Name: "cpu_usage_percent", ConstLabels: labels,
	})
	memoryUsage := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "agnivo", Subsystem: "runtime", Name: "memory_usage_mb", ConstLabels: labels,
	})
	return &Metrics{
		pullDuration: pullDuration, createDuration: createDuration, startDuration: startDuration,
		activeContainers: activeContainers, cpuUsage: cpuUsage, memoryUsage: memoryUsage,
		collectors: []prometheus.Collector{
			pullDuration, createDuration, startDuration, activeContainers, cpuUsage, memoryUsage,
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

func (m *Metrics) ObservePull(seconds float64) {
	if m != nil {
		m.pullDuration.Observe(seconds)
	}
}

func (m *Metrics) ObserveCreate(seconds float64) {
	if m != nil {
		m.createDuration.Observe(seconds)
	}
}

func (m *Metrics) ObserveStart(seconds float64) {
	if m != nil {
		m.startDuration.Observe(seconds)
	}
}

func (m *Metrics) SetActiveContainers(n float64) {
	if m != nil {
		m.activeContainers.Set(n)
	}
}

func (m *Metrics) SetCPU(n float64) {
	if m != nil {
		m.cpuUsage.Set(n)
	}
}

func (m *Metrics) SetMemory(n float64) {
	if m != nil {
		m.memoryUsage.Set(n)
	}
}

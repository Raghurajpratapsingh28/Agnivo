package jobs

import "github.com/prometheus/client_golang/prometheus"

// Metrics instruments the job engine. All methods are nil-safe so the engine
// runs without metrics in tests.
type Metrics struct {
	enqueued   *prometheus.CounterVec
	dequeued   *prometheus.CounterVec
	completed  prometheus.Counter
	retried    prometheus.Counter
	dead       prometheus.Counter
	duration   *prometheus.HistogramVec
	collectors []prometheus.Collector
}

// NewMetrics builds job metrics namespaced by service.
func NewMetrics(service string) *Metrics {
	labels := prometheus.Labels{"service": service}

	enqueued := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "agnivo", Subsystem: "jobs", Name: "enqueued_total",
		Help: "Jobs enqueued.", ConstLabels: labels,
	}, []string{"queue"})
	dequeued := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "agnivo", Subsystem: "jobs", Name: "dequeued_total",
		Help: "Jobs dequeued.", ConstLabels: labels,
	}, []string{"queue"})
	completed := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "agnivo", Subsystem: "jobs", Name: "completed_total",
		Help: "Jobs completed successfully.", ConstLabels: labels,
	})
	retried := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "agnivo", Subsystem: "jobs", Name: "retried_total",
		Help: "Jobs rescheduled after failure.", ConstLabels: labels,
	})
	dead := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "agnivo", Subsystem: "jobs", Name: "dead_total",
		Help: "Jobs dead-lettered.", ConstLabels: labels,
	})
	duration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "agnivo", Subsystem: "jobs", Name: "process_duration_seconds",
		Help: "Job processing latency in seconds.", Buckets: prometheus.DefBuckets, ConstLabels: labels,
	}, []string{"type", "outcome"})

	return &Metrics{
		enqueued: enqueued, dequeued: dequeued, completed: completed,
		retried: retried, dead: dead, duration: duration,
		collectors: []prometheus.Collector{enqueued, dequeued, completed, retried, dead, duration},
	}
}

// Collectors returns every collector for registration.
func (m *Metrics) Collectors() []prometheus.Collector {
	if m == nil {
		return nil
	}
	return m.collectors
}

func (m *Metrics) incEnqueued(queue string) {
	if m != nil {
		m.enqueued.WithLabelValues(queue).Inc()
	}
}
func (m *Metrics) addDequeued(queue string, n int) {
	if m != nil && n > 0 {
		m.dequeued.WithLabelValues(queue).Add(float64(n))
	}
}
func (m *Metrics) incCompleted() {
	if m != nil {
		m.completed.Inc()
	}
}
func (m *Metrics) incRetried() {
	if m != nil {
		m.retried.Inc()
	}
}
func (m *Metrics) incDead() {
	if m != nil {
		m.dead.Inc()
	}
}
func (m *Metrics) observe(jobType, outcome string, seconds float64) {
	if m != nil {
		m.duration.WithLabelValues(jobType, outcome).Observe(seconds)
	}
}

package metrics

import "github.com/prometheus/client_golang/prometheus"

// Metrics instruments builder operations.
type Metrics struct {
	buildDuration    *prometheus.HistogramVec
	queueWait        prometheus.Histogram
	buildFailures    prometheus.Counter
	buildSuccesses   prometheus.Counter
	cacheHitRatio    prometheus.Histogram
	cloneDuration    prometheus.Histogram
	dockerDuration   prometheus.Histogram
	pushDuration     prometheus.Histogram
	activeBuilds     prometheus.Gauge
	concurrentBuilds prometheus.Gauge
	collectors       []prometheus.Collector
}

// New builds builder metrics namespaced by service.
func New(service string) *Metrics {
	labels := prometheus.Labels{"service": service}
	buildDurationHist := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "agnivo", Subsystem: "builder", Name: "build_duration_hist_seconds",
		Help: "End-to-end build duration histogram.", Buckets: prometheus.ExponentialBuckets(1, 2, 12),
		ConstLabels: labels,
	}, []string{"framework", "outcome"})
	queueWait := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "agnivo", Subsystem: "builder", Name: "queue_wait_seconds",
		Help: "Time from job enqueue to build start.", Buckets: prometheus.DefBuckets, ConstLabels: labels,
	})
	buildFailures := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "agnivo", Subsystem: "builder", Name: "failures_total",
		Help: "Failed builds.", ConstLabels: labels,
	})
	buildSuccesses := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "agnivo", Subsystem: "builder", Name: "successes_total",
		Help: "Successful builds.", ConstLabels: labels,
	})
	cacheHitRatio := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "agnivo", Subsystem: "builder", Name: "cache_hit_ratio",
		Help: "Layer cache hit ratio per build.", Buckets: []float64{0, 0.25, 0.5, 0.75, 0.9, 1.0}, ConstLabels: labels,
	})
	cloneDuration := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "agnivo", Subsystem: "builder", Name: "clone_duration_seconds",
		Help: "Git clone duration.", Buckets: prometheus.DefBuckets, ConstLabels: labels,
	})
	dockerDuration := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "agnivo", Subsystem: "builder", Name: "docker_build_duration_seconds",
		Help: "Docker/BuildKit build duration.", Buckets: prometheus.ExponentialBuckets(5, 2, 10), ConstLabels: labels,
	})
	pushDuration := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "agnivo", Subsystem: "builder", Name: "push_duration_seconds",
		Help: "Registry push duration.", Buckets: prometheus.DefBuckets, ConstLabels: labels,
	})
	activeBuilds := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "agnivo", Subsystem: "builder", Name: "active_builds",
		Help: "Currently running builds.", ConstLabels: labels,
	})
	concurrentBuilds := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "agnivo", Subsystem: "builder", Name: "concurrent_builds",
		Help: "Concurrent build goroutines.", ConstLabels: labels,
	})
	return &Metrics{
		buildDuration: buildDurationHist, queueWait: queueWait,
		buildFailures: buildFailures, buildSuccesses: buildSuccesses,
		cacheHitRatio: cacheHitRatio, cloneDuration: cloneDuration,
		dockerDuration: dockerDuration, pushDuration: pushDuration,
		activeBuilds: activeBuilds, concurrentBuilds: concurrentBuilds,
		collectors: []prometheus.Collector{
			buildDurationHist, queueWait, buildFailures, buildSuccesses,
			cacheHitRatio, cloneDuration, dockerDuration, pushDuration,
			activeBuilds, concurrentBuilds,
		},
	}
}

// Collectors returns Prometheus collectors for registration.
func (m *Metrics) Collectors() []prometheus.Collector {
	if m == nil {
		return nil
	}
	return m.collectors
}

func (m *Metrics) ObserveBuild(framework, outcome string, seconds float64) {
	if m != nil {
		m.buildDuration.WithLabelValues(framework, outcome).Observe(seconds)
	}
}

func (m *Metrics) IncSuccess() {
	if m != nil {
		m.buildSuccesses.Inc()
	}
}

func (m *Metrics) IncFailure() {
	if m != nil {
		m.buildFailures.Inc()
	}
}

func (m *Metrics) ObserveQueueWait(seconds float64) {
	if m != nil {
		m.queueWait.Observe(seconds)
	}
}

func (m *Metrics) ObserveCacheHit(ratio float64) {
	if m != nil {
		m.cacheHitRatio.Observe(ratio)
	}
}

func (m *Metrics) ObserveClone(seconds float64) {
	if m != nil {
		m.cloneDuration.Observe(seconds)
	}
}

func (m *Metrics) ObserveDocker(seconds float64) {
	if m != nil {
		m.dockerDuration.Observe(seconds)
	}
}

func (m *Metrics) ObservePush(seconds float64) {
	if m != nil {
		m.pushDuration.Observe(seconds)
	}
}

func (m *Metrics) IncActive() {
	if m != nil {
		m.activeBuilds.Inc()
		m.concurrentBuilds.Inc()
	}
}

func (m *Metrics) DecActive() {
	if m != nil {
		m.activeBuilds.Dec()
		m.concurrentBuilds.Dec()
	}
}

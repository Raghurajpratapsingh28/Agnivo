package metrics

import "github.com/prometheus/client_golang/prometheus"

// Metrics instruments deployer operations.
type Metrics struct {
	deployDuration     *prometheus.HistogramVec
	queueWait          prometheus.Histogram
	deploySuccesses    prometheus.Counter
	deployFailures     prometheus.Counter
	rollbackTotal      prometheus.Counter
	healthDuration     prometheus.Histogram
	pullDuration       prometheus.Histogram
	startupDuration    prometheus.Histogram
	activeDeployments  prometheus.Gauge
	concurrentDeploys  prometheus.Gauge
	reservationLatency prometheus.Histogram
	collectors         []prometheus.Collector
}

// New builds deployer metrics.
func New(service string) *Metrics {
	labels := prometheus.Labels{"service": service}
	deployDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "agnivo", Subsystem: "deployer", Name: "deploy_duration_seconds",
		Buckets: prometheus.ExponentialBuckets(1, 2, 12), ConstLabels: labels,
	}, []string{"strategy", "outcome"})
	queueWait := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "agnivo", Subsystem: "deployer", Name: "queue_wait_seconds",
		Buckets: prometheus.DefBuckets, ConstLabels: labels,
	})
	deploySuccesses := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "agnivo", Subsystem: "deployer", Name: "successes_total", ConstLabels: labels,
	})
	deployFailures := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "agnivo", Subsystem: "deployer", Name: "failures_total", ConstLabels: labels,
	})
	rollbackTotal := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "agnivo", Subsystem: "deployer", Name: "rollbacks_total", ConstLabels: labels,
	})
	healthDuration := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "agnivo", Subsystem: "deployer", Name: "health_check_duration_seconds",
		Buckets: prometheus.DefBuckets, ConstLabels: labels,
	})
	pullDuration := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "agnivo", Subsystem: "deployer", Name: "image_pull_duration_seconds",
		Buckets: prometheus.DefBuckets, ConstLabels: labels,
	})
	startupDuration := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "agnivo", Subsystem: "deployer", Name: "container_startup_duration_seconds",
		Buckets: prometheus.DefBuckets, ConstLabels: labels,
	})
	activeDeployments := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "agnivo", Subsystem: "deployer", Name: "active_deployments", ConstLabels: labels,
	})
	concurrentDeploys := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "agnivo", Subsystem: "deployer", Name: "concurrent_deployments", ConstLabels: labels,
	})
	reservationLatency := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "agnivo", Subsystem: "deployer", Name: "reservation_latency_seconds",
		Buckets: prometheus.DefBuckets, ConstLabels: labels,
	})
	return &Metrics{
		deployDuration: deployDuration, queueWait: queueWait,
		deploySuccesses: deploySuccesses, deployFailures: deployFailures,
		rollbackTotal: rollbackTotal, healthDuration: healthDuration,
		pullDuration: pullDuration, startupDuration: startupDuration,
		activeDeployments: activeDeployments, concurrentDeploys: concurrentDeploys,
		reservationLatency: reservationLatency,
		collectors: []prometheus.Collector{
			deployDuration, queueWait, deploySuccesses, deployFailures, rollbackTotal,
			healthDuration, pullDuration, startupDuration, activeDeployments,
			concurrentDeploys, reservationLatency,
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

func (m *Metrics) ObserveDeploy(strategy, outcome string, seconds float64) {
	if m != nil {
		m.deployDuration.WithLabelValues(strategy, outcome).Observe(seconds)
	}
}

func (m *Metrics) IncSuccess() {
	if m != nil {
		m.deploySuccesses.Inc()
	}
}

func (m *Metrics) IncFailure() {
	if m != nil {
		m.deployFailures.Inc()
	}
}

func (m *Metrics) IncRollback() {
	if m != nil {
		m.rollbackTotal.Inc()
	}
}

func (m *Metrics) ObserveQueueWait(seconds float64) {
	if m != nil {
		m.queueWait.Observe(seconds)
	}
}

func (m *Metrics) ObserveHealth(seconds float64) {
	if m != nil {
		m.healthDuration.Observe(seconds)
	}
}

func (m *Metrics) ObservePull(seconds float64) {
	if m != nil {
		m.pullDuration.Observe(seconds)
	}
}

func (m *Metrics) ObserveStartup(seconds float64) {
	if m != nil {
		m.startupDuration.Observe(seconds)
	}
}

func (m *Metrics) ObserveReservation(seconds float64) {
	if m != nil {
		m.reservationLatency.Observe(seconds)
	}
}

func (m *Metrics) IncActive() {
	if m != nil {
		m.activeDeployments.Inc()
		m.concurrentDeploys.Inc()
	}
}

func (m *Metrics) DecActive() {
	if m != nil {
		m.activeDeployments.Dec()
		m.concurrentDeploys.Dec()
	}
}

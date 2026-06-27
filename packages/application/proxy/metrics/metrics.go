// Package metrics defines Prometheus collectors for the proxy-manager service.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics holds all proxy-manager Prometheus instruments.
type Metrics struct {
	// Routes
	RouteTotal      *prometheus.GaugeVec
	RouteUpdates    *prometheus.CounterVec
	RouteUpdateMs   *prometheus.HistogramVec
	RouteErrors     *prometheus.CounterVec

	// Certificates
	CertTotal       *prometheus.GaugeVec
	CertRenewals    prometheus.Counter
	CertFailures    prometheus.Counter
	CertAgeSeconds  *prometheus.HistogramVec

	// DNS Verification
	VerifyAttempts  *prometheus.CounterVec
	VerifyLatencyMs *prometheus.HistogramVec

	// Traffic switching
	TrafficSwitches *prometheus.CounterVec
	SwitchLatencyMs *prometheus.HistogramVec

	// Preview environments
	PreviewTotal    prometheus.Gauge
	PreviewCreated  prometheus.Counter
	PreviewExpired  prometheus.Counter

	// Streaming
	SSEConnections    prometheus.Gauge
	SSEMessages       prometheus.Counter
	StreamingErrors   prometheus.Counter

	// Caddy Admin API
	CaddyRequests   *prometheus.CounterVec
	CaddyLatencyMs  *prometheus.HistogramVec
	CaddyErrors     *prometheus.CounterVec

	// Reconciliation
	ReconcileRuns   prometheus.Counter
	ReconcileErrors prometheus.Counter
	ReconcileMs     prometheus.Histogram
}

// New constructs a Metrics instance namespaced to the service name.
func New(service string) *Metrics {
	labels := prometheus.Labels{"service": service}
	ns := "agnivo"
	sub := "proxy"

	m := &Metrics{
		RouteTotal: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns, Subsystem: sub, Name: "routes_total",
			Help: "Current number of proxy routes by status.", ConstLabels: labels,
		}, []string{"status", "kind"}),

		RouteUpdates: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub, Name: "route_updates_total",
			Help: "Total route create/update/delete operations.", ConstLabels: labels,
		}, []string{"operation"}),

		RouteUpdateMs: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: ns, Subsystem: sub, Name: "route_update_duration_ms",
			Help: "Route update latency in milliseconds.", Buckets: []float64{5, 25, 100, 500, 1000, 5000},
			ConstLabels: labels,
		}, []string{"operation"}),

		RouteErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub, Name: "route_errors_total",
			Help: "Total route operation errors.", ConstLabels: labels,
		}, []string{"operation"}),

		CertTotal: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns, Subsystem: sub, Name: "certs_total",
			Help: "Current number of TLS certificates by status.", ConstLabels: labels,
		}, []string{"status"}),

		CertRenewals: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub, Name: "cert_renewals_total",
			Help: "Total certificate renewals triggered.", ConstLabels: labels,
		}),

		CertFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub, Name: "cert_failures_total",
			Help: "Total certificate issuance/renewal failures.", ConstLabels: labels,
		}),

		CertAgeSeconds: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: ns, Subsystem: sub, Name: "cert_age_seconds",
			Help:    "Certificate age at time of renewal in seconds.",
			Buckets: prometheus.ExponentialBuckets(3600, 2, 12),
			ConstLabels: labels,
		}, []string{"issuer"}),

		VerifyAttempts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub, Name: "verify_attempts_total",
			Help: "Total DNS verification attempts.", ConstLabels: labels,
		}, []string{"method", "result"}),

		VerifyLatencyMs: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: ns, Subsystem: sub, Name: "verify_duration_ms",
			Help: "DNS verification latency in milliseconds.", Buckets: []float64{100, 500, 1000, 5000, 15000},
			ConstLabels: labels,
		}, []string{"method"}),

		TrafficSwitches: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub, Name: "traffic_switches_total",
			Help: "Total traffic switch operations.", ConstLabels: labels,
		}, []string{"mode"}),

		SwitchLatencyMs: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: ns, Subsystem: sub, Name: "switch_duration_ms",
			Help: "Traffic switch latency in milliseconds.", Buckets: []float64{5, 25, 100, 500, 2000},
			ConstLabels: labels,
		}, []string{"mode"}),

		PreviewTotal: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: ns, Subsystem: sub, Name: "previews_active",
			Help: "Current number of active preview environments.", ConstLabels: labels,
		}),

		PreviewCreated: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub, Name: "previews_created_total",
			Help: "Total preview environments created.", ConstLabels: labels,
		}),

		PreviewExpired: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub, Name: "previews_expired_total",
			Help: "Total preview environments that expired.", ConstLabels: labels,
		}),

		SSEConnections: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: ns, Subsystem: sub, Name: "sse_connections",
			Help: "Current number of active SSE connections.", ConstLabels: labels,
		}),

		SSEMessages: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub, Name: "sse_messages_total",
			Help: "Total SSE messages published.", ConstLabels: labels,
		}),

		StreamingErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub, Name: "streaming_errors_total",
			Help: "Total streaming delivery errors.", ConstLabels: labels,
		}),

		CaddyRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub, Name: "caddy_requests_total",
			Help: "Total Caddy Admin API requests.", ConstLabels: labels,
		}, []string{"method", "path", "status"}),

		CaddyLatencyMs: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: ns, Subsystem: sub, Name: "caddy_request_duration_ms",
			Help: "Caddy Admin API request latency.", Buckets: []float64{5, 25, 100, 500, 1000},
			ConstLabels: labels,
		}, []string{"method", "path"}),

		CaddyErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub, Name: "caddy_errors_total",
			Help: "Total Caddy Admin API errors.", ConstLabels: labels,
		}, []string{"operation"}),

		ReconcileRuns: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub, Name: "reconcile_runs_total",
			Help: "Total reconciliation loop executions.", ConstLabels: labels,
		}),

		ReconcileErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub, Name: "reconcile_errors_total",
			Help: "Total reconciliation loop errors.", ConstLabels: labels,
		}),

		ReconcileMs: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: ns, Subsystem: sub, Name: "reconcile_duration_ms",
			Help: "Reconciliation loop duration in milliseconds.", Buckets: []float64{50, 200, 500, 2000, 10000},
			ConstLabels: labels,
		}),
	}
	return m
}

// Collectors returns all registered collectors for MustRegister.
func (m *Metrics) Collectors() []prometheus.Collector {
	return []prometheus.Collector{
		m.RouteTotal, m.RouteUpdates, m.RouteUpdateMs, m.RouteErrors,
		m.CertTotal, m.CertRenewals, m.CertFailures, m.CertAgeSeconds,
		m.VerifyAttempts, m.VerifyLatencyMs,
		m.TrafficSwitches, m.SwitchLatencyMs,
		m.PreviewTotal, m.PreviewCreated, m.PreviewExpired,
		m.SSEConnections, m.SSEMessages, m.StreamingErrors,
		m.CaddyRequests, m.CaddyLatencyMs, m.CaddyErrors,
		m.ReconcileRuns, m.ReconcileErrors, m.ReconcileMs,
	}
}

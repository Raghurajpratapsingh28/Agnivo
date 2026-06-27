// Package metrics defines Prometheus collectors for the Platform Operations Layer.
package metrics

import "github.com/prometheus/client_golang/prometheus"

// Metrics holds all ops Prometheus instruments.
type Metrics struct {
	// Worker throughput
	JobsProcessed   *prometheus.CounterVec
	JobsFailed      *prometheus.CounterVec
	JobDurationMs   *prometheus.HistogramVec
	QueueDepth      *prometheus.GaugeVec
	RetryRate       *prometheus.CounterVec

	// Billing
	InvoicesGenerated prometheus.Counter
	SubscriptionChanges *prometheus.CounterVec
	CreditGranted    prometheus.Counter
	BillingErrors    prometheus.Counter

	// Metering
	UsageRecorded   *prometheus.CounterVec
	RollupDurationMs prometheus.Histogram

	// Quota
	QuotaChecks     *prometheus.CounterVec
	QuotaViolations *prometheus.CounterVec
	QuotaEvalMs     prometheus.Histogram

	// Notifications
	NotifDelivered  *prometheus.CounterVec
	NotifFailed     *prometheus.CounterVec
	NotifLatencyMs  *prometheus.HistogramVec

	// Backups
	BackupRuns      *prometheus.CounterVec
	BackupDurationMs prometheus.Histogram
	BackupSizeBytes  prometheus.Histogram

	// Cleanup
	CleanupRuns     prometheus.Counter
	RowsPurged      *prometheus.CounterVec

	// Analytics
	AnalyticsRuns   prometheus.Counter
	AnalyticsDurationMs prometheus.Histogram

	// Auto-sleep
	SleepEvents     prometheus.Counter
	WakeEvents      prometheus.Counter

	// Cron
	CronFired       *prometheus.CounterVec
	CronErrors      *prometheus.CounterVec
	CronDurationMs  *prometheus.HistogramVec
}

// New constructs a Metrics instance.
func New(service string) *Metrics {
	labels := prometheus.Labels{"service": service}
	ns, sub := "agnivo", "ops"

	m := &Metrics{
		JobsProcessed: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub, Name: "jobs_processed_total",
			Help: "Total jobs processed.", ConstLabels: labels,
		}, []string{"queue", "type", "result"}),

		JobsFailed: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub, Name: "jobs_failed_total",
			Help: "Total job failures.", ConstLabels: labels,
		}, []string{"queue", "type"}),

		JobDurationMs: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: ns, Subsystem: sub, Name: "job_duration_ms",
			Help: "Job processing duration.", Buckets: []float64{5, 50, 200, 1000, 5000, 30000},
			ConstLabels: labels,
		}, []string{"queue", "type"}),

		QueueDepth: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: ns, Subsystem: sub, Name: "queue_depth",
			Help: "Estimated queue depth per queue.", ConstLabels: labels,
		}, []string{"queue"}),

		RetryRate: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub, Name: "job_retries_total",
			Help: "Total job retries.", ConstLabels: labels,
		}, []string{"queue", "type"}),

		InvoicesGenerated: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub, Name: "invoices_generated_total",
			Help: "Total invoices generated.", ConstLabels: labels,
		}),

		SubscriptionChanges: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub, Name: "subscription_changes_total",
			Help: "Total subscription state changes.", ConstLabels: labels,
		}, []string{"action", "plan"}),

		CreditGranted: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub, Name: "credits_granted_total",
			Help: "Total credits granted.", ConstLabels: labels,
		}),

		BillingErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub, Name: "billing_errors_total",
			Help: "Total billing processing errors.", ConstLabels: labels,
		}),

		UsageRecorded: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub, Name: "usage_recorded_total",
			Help: "Total usage records ingested.", ConstLabels: labels,
		}, []string{"dimension"}),

		RollupDurationMs: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: ns, Subsystem: sub, Name: "rollup_duration_ms",
			Help: "Usage rollup duration.", Buckets: []float64{100, 500, 2000, 10000},
			ConstLabels: labels,
		}),

		QuotaChecks: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub, Name: "quota_checks_total",
			Help: "Total quota evaluations.", ConstLabels: labels,
		}, []string{"dimension", "result"}),

		QuotaViolations: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub, Name: "quota_violations_total",
			Help: "Total quota violations.", ConstLabels: labels,
		}, []string{"dimension", "severity"}),

		QuotaEvalMs: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: ns, Subsystem: sub, Name: "quota_eval_ms",
			Help: "Quota evaluation latency.", Buckets: []float64{1, 5, 25, 100},
			ConstLabels: labels,
		}),

		NotifDelivered: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub, Name: "notifications_delivered_total",
			Help: "Total notifications delivered.", ConstLabels: labels,
		}, []string{"channel"}),

		NotifFailed: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub, Name: "notifications_failed_total",
			Help: "Total notification delivery failures.", ConstLabels: labels,
		}, []string{"channel"}),

		NotifLatencyMs: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: ns, Subsystem: sub, Name: "notification_latency_ms",
			Help: "Notification delivery latency.", Buckets: []float64{100, 500, 2000, 10000},
			ConstLabels: labels,
		}, []string{"channel"}),

		BackupRuns: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub, Name: "backup_runs_total",
			Help: "Total backup runs.", ConstLabels: labels,
		}, []string{"kind", "result"}),

		BackupDurationMs: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: ns, Subsystem: sub, Name: "backup_duration_ms",
			Help:    "Backup duration.", Buckets: prometheus.ExponentialBuckets(1000, 2, 10),
			ConstLabels: labels,
		}),

		BackupSizeBytes: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: ns, Subsystem: sub, Name: "backup_size_bytes",
			Help:    "Backup artifact size.", Buckets: prometheus.ExponentialBuckets(1024*1024, 2, 16),
			ConstLabels: labels,
		}),

		CleanupRuns: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub, Name: "cleanup_runs_total",
			Help: "Total GC cleanup runs.", ConstLabels: labels,
		}),

		RowsPurged: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub, Name: "rows_purged_total",
			Help: "Total rows purged by GC.", ConstLabels: labels,
		}, []string{"target"}),

		AnalyticsRuns: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub, Name: "analytics_runs_total",
			Help: "Total analytics aggregation runs.", ConstLabels: labels,
		}),

		AnalyticsDurationMs: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: ns, Subsystem: sub, Name: "analytics_duration_ms",
			Help: "Analytics aggregation duration.", Buckets: []float64{500, 2000, 10000, 60000},
			ConstLabels: labels,
		}),

		SleepEvents: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub, Name: "sleep_events_total",
			Help: "Total auto-sleep events.", ConstLabels: labels,
		}),

		WakeEvents: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub, Name: "wake_events_total",
			Help: "Total wake events.", ConstLabels: labels,
		}),

		CronFired: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub, Name: "cron_fired_total",
			Help: "Total cron jobs fired.", ConstLabels: labels,
		}, []string{"name"}),

		CronErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Subsystem: sub, Name: "cron_errors_total",
			Help: "Total cron fire errors.", ConstLabels: labels,
		}, []string{"name"}),

		CronDurationMs: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: ns, Subsystem: sub, Name: "cron_duration_ms",
			Help: "Cron job execution duration.", Buckets: []float64{10, 100, 500, 5000},
			ConstLabels: labels,
		}, []string{"name"}),
	}
	return m
}

// Collectors returns all registered collectors for MustRegister.
func (m *Metrics) Collectors() []prometheus.Collector {
	return []prometheus.Collector{
		m.JobsProcessed, m.JobsFailed, m.JobDurationMs, m.QueueDepth, m.RetryRate,
		m.InvoicesGenerated, m.SubscriptionChanges, m.CreditGranted, m.BillingErrors,
		m.UsageRecorded, m.RollupDurationMs,
		m.QuotaChecks, m.QuotaViolations, m.QuotaEvalMs,
		m.NotifDelivered, m.NotifFailed, m.NotifLatencyMs,
		m.BackupRuns, m.BackupDurationMs, m.BackupSizeBytes,
		m.CleanupRuns, m.RowsPurged,
		m.AnalyticsRuns, m.AnalyticsDurationMs,
		m.SleepEvents, m.WakeEvents,
		m.CronFired, m.CronErrors, m.CronDurationMs,
	}
}

package prober

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// Latency measures probing duration in seconds per target endpoint.
	LatencyHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "kube_prober_latency_seconds",
			Help: "The time taken to probe the target in seconds (Latency).",
		},
		[]string{"target"},
	)

	// Traffic tracks the total cumulative count of health-check probes sent per target.
	TrafficCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kube_prober_traffic_total",
			Help: "Total number of probes sent to the target (Traffic).",
		},
		[]string{"target"},
	)

	// Errors tracks failed probes categorized by target address and SRE error category.
	ErrorCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kube_prober_errors_total",
			Help: "Total number of failed probes (Errors).",
		},
		[]string{"target", "category"},
	)

	// Saturation gauges current capacity by counting active concurrent worker goroutines.
	SaturationGauge = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "kube_prober_saturation_active_workers",
			Help: "Number of active concurrent probing workers (Saturation).",
		},
	)

	// MaxWorkersGauge represents the maximum configured worker goroutine pool limit.
	MaxWorkersGauge = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "kube_prober_saturation_max_workers",
			Help: "Maximum capacity of worker goroutines configured for the prober.",
		},
	)

	// CategoryHintInfo exposes static troubleshooting hints per category as an info metric.
	CategoryHintInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "kube_prober_error_category_hint_info",
			Help: "Static mapping of error categories to actionable troubleshooting hints.",
		},
		[]string{"category", "hint"},
	)

	// TLSCertExpiryGauge tracks the remaining days until the target TLS certificate expires.
	TLSCertExpiryGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "kube_prober_tls_cert_expiry_days",
			Help: "Number of days until the TLS certificate expires.",
		},
		[]string{"target"},
	)
)

// allCategories defines the complete list of registered SRE error categories.
var allCategories = []ErrorCategory{
	CategoryDNS,
	CategoryConnectionRefused,
	CategoryTLS,
	CategoryTimeout,
	CategoryHTTP,
	CategoryGRPCNotServing,
	CategoryGRPCError,
	CategoryUnhealthy,
	CategoryAuth,
	CategoryUnknown,
}

// RegisterMetrics registers all prober metrics with the provided Prometheus registry.
func RegisterMetrics(reg prometheus.Registerer) {
	reg.MustRegister(
		LatencyHistogram,
		TrafficCounter,
		ErrorCounter,
		SaturationGauge,
		MaxWorkersGauge,
		CategoryHintInfo,
		TLSCertExpiryGauge,
	)

	// Pre-populate static troubleshooting hints for Prometheus info metrics
	for _, cat := range allCategories {
		CategoryHintInfo.WithLabelValues(string(cat), cat.Hint()).Set(1)
	}
}

// DeleteTargetMetrics cleans up all time-series vectors associated with a deleted target
// to avoid Prometheus TSDB cardinality bloat and memory leaks.
func DeleteTargetMetrics(target string) {
	// 1. Delete single-label target metrics
	LatencyHistogram.DeleteLabelValues(target)
	TrafficCounter.DeleteLabelValues(target)
	TLSCertExpiryGauge.DeleteLabelValues(target)

	// 2. Delete all error category variations for this specific target
	for _, cat := range allCategories {
		ErrorCounter.DeleteLabelValues(target, string(cat))
	}
}

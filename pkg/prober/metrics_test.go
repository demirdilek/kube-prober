package prober

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestRegisterMetricsAndHints(t *testing.T) {
	reg := prometheus.NewRegistry()
	RegisterMetrics(reg)

	// Ensure all 10 categories are registered with hints
	expectedCategories := []ErrorCategory{
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

	if len(allCategories) != len(expectedCategories) {
		t.Fatalf("expected %d categories, got %d", len(expectedCategories), len(allCategories))
	}

	// Verify hint exists and is not empty for each category
	for _, cat := range expectedCategories {
		hint := cat.Hint()
		if hint == "" {
			t.Errorf("expected non-empty hint for category %s", cat)
		}
	}
}

func TestDeleteTargetMetrics(t *testing.T) {
	target := "test-target.default.svc.cluster.local:80"

	// Populate metrics
	LatencyHistogram.WithLabelValues(target).Observe(0.123)
	TrafficCounter.WithLabelValues(target).Inc()
	TLSCertExpiryGauge.WithLabelValues(target).Set(30)
	for _, cat := range allCategories {
		ErrorCounter.WithLabelValues(target, string(cat)).Inc()
	}

	// Delete metrics for target
	DeleteTargetMetrics(target)

	// Verify that ErrorCounter labels for this target are cleaned up
	for _, cat := range allCategories {
		var metric dto.Metric
		err := ErrorCounter.WithLabelValues(target, string(cat)).Write(&metric)
		// When deleted, a new call initializes it to 0
		if err == nil && metric.Counter != nil && metric.Counter.GetValue() != 0 {
			t.Errorf("expected metric for %s/%s to be reset/deleted", target, cat)
		}
	}
}

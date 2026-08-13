package prober

import (
	"context"
	"testing"
	"time"
)

func TestDNSProber_ProbeDNSTarget(t *testing.T) {
	prober := NewDNSProber()

	tests := []struct {
		name     string
		target   string
		timeout  time.Duration
		expected ErrorCategory
	}{
		{
			name:     "Invalid URL format",
			target:   "%%%invalid-dns-target",
			timeout:  2 * time.Second,
			expected: CategoryUnknown,
		},
		{
			name: "Missing domain in path",
			// No path provided, so there is no domain to resolve
			target:   "dns://127.0.0.1:53",
			timeout:  2 * time.Second,
			expected: CategoryDNS,
		},
		{
			name: "Connection timeout",
			// Use a non-routable IP (TEST-NET-1) to force a timeout, combined with a tiny context deadline
			target:   "dns://198.51.100.1:53/example.com?type=A",
			timeout:  1 * time.Millisecond,
			expected: CategoryTimeout,
		},
		{
			name: "Custom record type execution timeout",
			// Test that query parameters like type=TXT are processed and correctly passed into the timeout context
			target:   "dns://198.51.100.1:53/example.com?type=TXT",
			timeout:  1 * time.Millisecond,
			expected: CategoryTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			got := prober.ProbeDNSTarget(ctx, tt.target)
			if got != tt.expected {
				t.Errorf("ProbeDNSTarget() = %v, want %v", got, tt.expected)
			}
		})
	}
}

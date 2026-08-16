package prober

import (
	"context"
	"testing"
	"time"
)

func TestDNSProber_ProbeDNSTarget(t *testing.T) {
	prober := NewDNSProber()

	tests := []struct {
		name      string
		target    Target
		timeout   time.Duration
		expectErr bool
	}{
		{
			name: "Successful DNS resolution",
			target: Target{
				Name:    "localhost-check",
				Address: "dns://localhost",
				Scheme:  "dns",
			},
			timeout:   2 * time.Second,
			expectErr: false,
		},
		{
			name: "Failed DNS resolution",
			target: Target{
				Name:    "invalid-host",
				Address: "dns://this-domain-does-not-exist.local",
				Scheme:  "dns",
			},
			timeout:   2 * time.Second,
			expectErr: true,
		},
		{
			name: "Timeout DNS resolution",
			// A context with zero timeout will force an immediate timeout error
			target: Target{
				Name:    "timeout-host",
				Address: "google.com",
				Scheme:  "dns",
			},
			timeout:   0 * time.Millisecond,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			got := prober.ProbeDNSTarget(ctx, tt.target)

			if tt.expectErr && got == "" {
				t.Errorf("ProbeDNSTarget() expected an error category, got success")
			}
			if !tt.expectErr && got != "" {
				t.Errorf("ProbeDNSTarget() expected success, got error category: %v", got)
			}
		})
	}
}

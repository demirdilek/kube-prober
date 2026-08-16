package prober

import (
	"context"
	"testing"
)

func TestDispatcher_Execute(t *testing.T) {
	d := NewDispatcher()

	// Mock probe functions to simulate different outcomes
	mockHTTPProber := func(ctx context.Context, target Target) ErrorCategory {
		return "" // Simulate success
	}

	mockTCPProber := func(ctx context.Context, target Target) ErrorCategory {
		return CategoryConnectionRefused // Simulate a specific error
	}

	// Register the mock functions
	d.Register("http", mockHTTPProber)
	d.Register("tcp", mockTCPProber)

	tests := []struct {
		name     string
		target   Target
		expected ErrorCategory
	}{
		{
			name: "Execute registered HTTP scheme successfully",
			target: Target{
				Name:    "test-http",
				Address: "http://example.com",
				Scheme:  "http",
			},
			expected: "",
		},
		{
			name: "Execute registered TCP scheme with expected error return",
			target: Target{
				Name:    "test-tcp",
				Address: "tcp://127.0.0.1:5432",
				Scheme:  "tcp",
			},
			expected: CategoryConnectionRefused,
		},
		{
			name: "Execute unknown scheme falls back to CategoryUnknown",
			target: Target{
				Name:    "test-unknown",
				Address: "ftp://example.com",
				Scheme:  "ftp",
			},
			expected: CategoryUnknown,
		},
		{
			name: "Execute empty scheme falls back to CategoryUnknown",
			target: Target{
				Name:    "test-empty",
				Address: "example.com",
				Scheme:  "",
			},
			expected: CategoryUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := d.Execute(context.Background(), tt.target)
			if got != tt.expected {
				t.Errorf("Execute() = %v, want %v", got, tt.expected)
			}
		})
	}
}

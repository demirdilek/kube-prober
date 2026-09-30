package prober

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestDispatcher_Execute(t *testing.T) {
	d := NewDispatcher(10)

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

func TestDispatcher_SemaphoreContextCancellation(t *testing.T) {
	// Semaphore with capacity 1
	d := NewDispatcher(1)

	blocker := make(chan struct{})

	d.Register("block", func(ctx context.Context, target Target) ErrorCategory {
		<-blocker
		return ""
	})

	target := Target{Address: "block://test", Scheme: "block"}

	var wg sync.WaitGroup
	wg.Add(1)

	// First probe acquires the only slot and blocks
	go func() {
		defer wg.Done()
		_ = d.Execute(context.Background(), target)
	}()

	time.Sleep(20 * time.Millisecond)

	// Second probe must time out when context expires while waiting for a slot
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	errCat := d.Execute(ctx, target)
	if errCat != CategoryTimeout {
		t.Fatalf("expected %v, got %v", CategoryTimeout, errCat)
	}

	close(blocker)
	wg.Wait()
}

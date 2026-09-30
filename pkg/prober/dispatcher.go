package prober

import (
	"context"
	"net/url"
	"sync"
)

// ProbeFunc defines the function pointer signature for any protocol prober.
type ProbeFunc func(ctx context.Context, target Target) ErrorCategory

// Dispatcher manages protocol handlers via function pointers
type Dispatcher struct {
	mu      sync.RWMutex
	probers map[string]ProbeFunc
	sem     chan struct{} // Counting semaphore Token bucket
}

// NewDispatcher initializes an empty dispatcher map
func NewDispatcher(maxConcurrency int) *Dispatcher {
	var sem chan struct{}
	if maxConcurrency > 0 {
		sem = make(chan struct{}, maxConcurrency)
	}

	return &Dispatcher{
		probers: make(map[string]ProbeFunc),
		sem:     sem,
	}
}

// Register assigns a ProbeFunc function pointer to a specific scheme (e.g., "http", "https")
func (d *Dispatcher) Register(scheme string, fn ProbeFunc) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.probers[scheme] = fn
}

// Execute resolves the scheme from the target struct and runs the corresponding ProbeFunc
func (d *Dispatcher) Execute(ctx context.Context, target Target) ErrorCategory {
	// Concurrency protection via non-blocking TryAcquire pattern
	if d.sem != nil {
		select {
		case d.sem <- struct{}{}:
			// Token acquire. Ensure release upon completion
			defer func() { <-d.sem }()
		case <-ctx.Done():
			return CategoryTimeout
		}
	}

	d.mu.RLock()
	fn, exists := d.probers[target.Scheme]
	d.mu.RUnlock()

	if !exists {
		// Fallback: If the scheme wasn't set, try to parse it from the address
		parsedURL, err := url.Parse(target.Address)
		if err == nil && parsedURL.Scheme != "" {
			d.mu.RLock()
			fn, exists = d.probers[parsedURL.Scheme]
			d.mu.RUnlock()
		}

		if !exists {
			return CategoryUnknown
		}
	}

	// Direct execution via function pointer
	return fn(ctx, target)
}

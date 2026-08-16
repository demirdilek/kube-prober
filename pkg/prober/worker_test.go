package prober

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestTargetScheduler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	jobs := make(chan Job, 5)
	var wg sync.WaitGroup

	target := Target{
		Name:    "scheduler-test",
		Address: "http://localhost:8080",
		Scheme:  "http",
	}

	wg.Add(1)
	// Run scheduler with a very short interval for fast testing
	go TargetScheduler(ctx, target, jobs, 10*time.Millisecond, &wg)

	// Wait for the initial immediate job
	select {
	case job := <-jobs:
		if job.Target.Address != target.Address {
			t.Errorf("Expected job target address %s, got %s", target.Address, job.Target.Address)
		}
		if job.Target.Scheme != target.Scheme {
			t.Errorf("Expected job target scheme %s, got %s", target.Scheme, job.Target.Scheme)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timed out waiting for initial job from scheduler")
	}

	// Wait for at least one ticker-based job
	select {
	case <-jobs:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("Timed out waiting for ticker job from scheduler")
	}

	// Cleanup
	cancel()
	wg.Wait()
}

func TestWorkerPool(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	d := NewDispatcher()

	// Use a channel to track if our mock prober was executed by the worker
	called := make(chan Target, 1)
	mockProber := func(ctx context.Context, target Target) ErrorCategory {
		called <- target
		return ""
	}
	d.Register("http", mockProber)

	jobs := make(chan Job, 1)
	var wg sync.WaitGroup

	target := Target{
		Name:    "worker-test",
		Address: "http://worker-pool-test",
		Scheme:  "http",
	}

	// Queue the job
	jobs <- Job{Target: target}

	wg.Add(1)
	go WorkerPool(ctx, jobs, d, &wg)

	// Verify the mock prober was executed with the correct target data
	select {
	case executedTarget := <-called:
		if executedTarget.Address != target.Address {
			t.Errorf("Expected target %s to be processed, got %s", target.Address, executedTarget.Address)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("WorkerPool failed to process job in time")
	}

	// Cleanup
	cancel()
	wg.Wait()
}

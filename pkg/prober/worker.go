package prober

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type Job struct {
	Target Target
}

// ProbeTarget executes a probe against a target and records 4 Golden Signals metrics.
func ProbeTarget(ctx context.Context, target Target, dispatcher *Dispatcher) {
	SaturationGauge.Inc()
	defer SaturationGauge.Dec()

	TrafficCounter.WithLabelValues(target.Address).Inc()

	// Create a new context with a timeout for the probe execution
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	startTime := time.Now()
	errCat := dispatcher.Execute(probeCtx, target)
	duration := time.Since(startTime).Seconds()

	LatencyHistogram.WithLabelValues(target.Address).Observe(duration)

	// Record the error category if there was an error, otherwise log success
	if errCat != "" {
		ErrorCounter.WithLabelValues(target.Address, string(errCat)).Inc()
		slog.Warn(
			"Target probing failed",
			"target", target.Address,
			"scheme", target.Scheme,
			"error_category", errCat,
			"hint", errCat.Hint(),
		)
	} else {
		slog.Debug("Target probed successfully", "target", target.Address, "duration_seconds", duration)
	}
}

// WorkerPool processes incoming probe jobs.
func WorkerPool(ctx context.Context, jobs <-chan Job, dispatcher *Dispatcher, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-jobs:
			if !ok {
				return
			}
			ProbeTarget(ctx, job.Target, dispatcher)
		}
	}
}

// TargetScheduler pushes probe jobs into the jobs channel periodically.
func TargetScheduler(ctx context.Context, target Target, jobs chan<- Job, interval time.Duration, wg *sync.WaitGroup) {
	defer wg.Done()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Initial immediate probe
	select {
	case jobs <- Job{Target: target}:
	case <-ctx.Done():
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			select {
			case jobs <- Job{Target: target}:
			case <-ctx.Done():
				return
			}
		}
	}
}

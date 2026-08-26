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

// ProbeTarget executes a probe against a target with configurable timeout.
func ProbeTarget(ctx context.Context, target Target, dispatcher *Dispatcher, timeout time.Duration) {
	SaturationGauge.Inc()
	defer SaturationGauge.Dec()

	TrafficCounter.WithLabelValues(target.Address).Inc()

	// Timeout aus Konfiguration statt festen 5s nutzen
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	startTime := time.Now()
	errCat := dispatcher.Execute(probeCtx, target)
	duration := time.Since(startTime).Seconds()

	LatencyHistogram.WithLabelValues(target.Address).Observe(duration)

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

// WorkerPool reicht den timeout weiter
func WorkerPool(ctx context.Context, jobs <-chan Job, dispatcher *Dispatcher, timeout time.Duration, wg *sync.WaitGroup) {
	defer wg.Done()

	for job := range jobs {
		ProbeTarget(ctx, job.Target, dispatcher, timeout)
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
	default:
		// Queue full: skip immediate probe and wait for next interval tick
		slog.Warn("Job queue full, probe dropped", "target", target.Address)
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
			default:
				// Queue full: skip immediate probe and wait for next interval tick
				slog.Warn("Job queue full, probe dropped", "target", target.Address)
			}
		}
	}
}

package server

import (
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// MetricsCleaner handles post-scrape metric cleanup to ensure Prometheus
// collects final metric states before removal.
type MetricsCleaner struct {
	mu             sync.Mutex
	pendingDeletes map[string]int // Target address -> remaining scrapes before deletion
	deleteFn       func(string)
}

// NewMetricsCleaner initializes a cleaner with the provided delete callback.
func NewMetricsCleaner(deleteFn func(string)) *MetricsCleaner {
	return &MetricsCleaner{
		pendingDeletes: make(map[string]int),
		deleteFn:       deleteFn,
	}
}

// MarkForDeletion queues a target address for cleanup after 1 final scrape cycle.
func (c *MetricsCleaner) MarkForDeletion(target string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pendingDeletes[target] = 1
}

// AbortDeletion cancels a pending deletion if the target becomes active again.
func (c *MetricsCleaner) AbortDeletion(target string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.pendingDeletes, target)
}

// Handler returns an http.Handler that wraps the standard Prometheus endpoint
// and executes deferred cleanups immediately after serving metrics.
func (c *MetricsCleaner) Handler() http.Handler {
	promHandler := promhttp.Handler()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Serve current metrics to Prometheus scraper
		promHandler.ServeHTTP(w, r)

		// 2. Collect expired targets under lock
		var toDelete []string
		c.mu.Lock()
		for target, remaining := range c.pendingDeletes {
			if remaining <= 1 {
				toDelete = append(toDelete, target)
				delete(c.pendingDeletes, target)
			} else {
				c.pendingDeletes[target]--
			}
		}
		c.mu.Unlock()

		// 3. Execute delete callback outside the mutex
		for _, target := range toDelete {
			c.deleteFn(target)
		}
	})
}

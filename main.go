package main

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/demirdilek/kube-prober/pkg/env"
	"github.com/demirdilek/kube-prober/pkg/kube"
	"github.com/demirdilek/kube-prober/pkg/prober"
	"github.com/demirdilek/kube-prober/pkg/server"
)

// init registers all core SRE metrics with the Prometheus default registry.
func init() {
	prober.RegisterMetrics(prometheus.DefaultRegisterer)
}

func main() {
	// Set Log	with enviroment variable
	logLevel := slog.LevelInfo
	if os.Getenv("LOG_LEVEL") == "DEBUG" {
		logLevel = slog.LevelDebug
	}

	opts := &slog.HandlerOptions{
		Level: logLevel,
	}

	// Setup structured JSON logging to stdout for cloud-native log ingestion
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, opts)))

	// Listen for OS interrupt and SIGTERM signals to initiate a graceful shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Load runtime parameters from environment variables with sensible production defaults
	numWorkers := env.GetInt("WORKERS", 50)
	prober.MaxWorkersGauge.Set(float64(numWorkers))
	jobQueueSize := env.GetInt("QUEUE_SIZE", 10000)
	probeInterval := time.Duration(env.GetInt("PROBE_INTERVAL_SECONDS", 2)) * time.Second
	httpTimeout := time.Duration(env.GetInt("HTTP_TIMEOUT_SECONDS", 5)) * time.Second

	// Pre-configure HTTP transport with aggressive connection pooling to prevent socket exhaustion
	httpClient := &http.Client{
		Timeout: httpTimeout,
		Transport: &http.Transport{
			MaxIdleConns:        env.GetInt("MAX_IDLE_CONNS", 1000),
			MaxIdleConnsPerHost: env.GetInt("MAX_IDLE_CONNS_PER_HOST", 100),
			IdleConnTimeout:     90 * time.Second,
		},
	}

	// Initialize protocol dispatcher and register respective health-check handlers
	dispatcher := prober.NewDispatcher()

	// Register HTTP/HTTPS handlers
	httpProber := prober.NewHTTPProber(httpClient)
	dispatcher.Register("http", httpProber.ProbeHTTPTarget)
	dispatcher.Register("https", httpProber.ProbeHTTPTarget)

	// Register TCP Layer 4 handler
	tcpProber := prober.NewTCPProber()
	dispatcher.Register("tcp", tcpProber.ProbeTCPTarget)

	// Register TLS handshake and certificate expiry validation handler
	var tlsCfg *tls.Config
	tlsProber := prober.NewTLSProber(tlsCfg)
	dispatcher.Register("tls", tlsProber.ProbeTLSTarget)

	// Register gRPC Health Checking protocol handler
	grpcProber := prober.NewGRPCProber()
	dispatcher.Register("grpc", grpcProber.ProbeGRPCTarget)

	// Register DNS hostname resolution handler
	dnsProber := prober.NewDNSProber()
	dispatcher.Register("dns", dnsProber.ProbeDNSTarget)

	// Separate WaitGroups for producers (schedulers) and consumers (worker pool)
	// to prevent deadlocks during coordinated graceful termination
	var workerWG sync.WaitGroup
	var schedulerWG sync.WaitGroup
	jobs := make(chan prober.Job, jobQueueSize)

	// Spawn the worker pool goroutines to process incoming probe jobs concurrently
	for i := 0; i < numWorkers; i++ {
		workerWG.Add(1)
		go prober.WorkerPool(ctx, jobs, dispatcher, &workerWG)
	}

	// Initialize standard Kubernetes Clientset for service discovery and peer tracking
	clientset, err := kube.InitClient()
	if err != nil {
		slog.Error("Initialization failed for clientset", "error", err)
		os.Exit(1)
	}

	// Initialize dynamic client for custom resource definitions (StaticTarget CRDs)
	dynClient, err := kube.InitDynamicClient()
	if err != nil {
		slog.Error("Initialization failed for dynamic client", "error", err)
		os.Exit(1)
	}

	// Initialize the sharded target registry using the local pod IP (Downward API)
	selfIP := os.Getenv("POD_IP")
	registry := prober.NewRegistry(selfIP)

	// Start watching StaticTarget CRDs in the background
	go func() {
		if err := prober.WatchStaticTargets(ctx, dynClient, registry); err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("StaticTargets informer stopped", "error", err)
		}
	}()

	// Initialize the unified KubeWatcher for EndpointSlices and peer topology
	watcher := prober.NewKubeWatcher(clientset, registry)

	// 1. Continuously watch prober peer replicas to rebalance targets upon HPA scaling events
	go watcher.WatchPeers(ctx)

	// 2. Start the EndpointSlice informer to dynamically discover annotated service endpoints
	go func() {
		if err := watcher.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("Informer watcher stopped", "error", err)
		}
	}()

	// Track active per-target scheduler cancellation functions
	activeSchedulers := make(map[string]context.CancelFunc)
	var schedMu sync.Mutex
	
	// 1. Initialize cleaner with the metric purge callback
	cleaner := server.NewMetricsCleaner(prober.DeleteTargetMetrics)
	// Start the internal HTTP server to expose Prometheus metrics and health probes (:8080)
	srv := server.New(":8080", cleaner)
	go srv.Start()

	// Event loop: handle target additions, rebalancing decisions, and removals from the registry
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-registry.Events:
				if !ok {
					return
				}

				func() {
					schedMu.Lock()
					defer schedMu.Unlock()

					if evt.IsAdded {
						// Cancel any pending deferred deletion if target is re-added
						cleaner.AbortDeletion(evt.Target.Address)

						if _, exists := activeSchedulers[evt.Target.Address]; !exists {
							slog.Info("New target discovered", "target", evt.Target.Address, "scheme", evt.Target.Scheme)
							schedCtx, schedCancel := context.WithCancel(ctx)
							activeSchedulers[evt.Target.Address] = schedCancel

							schedulerWG.Add(1)
							go prober.TargetScheduler(schedCtx, evt.Target, jobs, probeInterval, &schedulerWG)
						}
					} else {
						if cancelFunc, exists := activeSchedulers[evt.Target.Address]; exists {
							slog.Info("Target removed", "target", evt.Target.Address)
							cancelFunc()
							delete(activeSchedulers, evt.Target.Address)

							// Queue post-scrape metric cleanup without blocking goroutines
							cleaner.MarkForDeletion(evt.Target.Address)
						}
					}
				}()
			}
		}
	}()

	// Block main routine until an OS termination signal is received
	<-ctx.Done()
	slog.Info("Shutting down cleanly...")

	// Phase 1: Stop all active target schedulers to prevent new job generation
	schedMu.Lock()
	for _, cancelFn := range activeSchedulers {
		cancelFn()
	}
	schedMu.Unlock()

	// Phase 2: Wait until all scheduler loops have exited completely
	schedulerWG.Wait()

	// Phase 3: Close the job channel; workers will finish remaining buffered jobs and exit
	close(jobs)

	// Phase 4: Gracefully stop the HTTP server allowing in-flight scrapes to complete
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)

	// Phase 5: Wait for all worker goroutines to drain the channel and finish
	workerWG.Wait()
	slog.Info("Goodbye.")
}

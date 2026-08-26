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
	probeTimeout := time.Duration(env.GetInt("PROBE_TIMEOUT_SECONDS", 5)) * time.Second

	// Pre-configure HTTP transport with aggressive connection pooling to prevent socket exhaustion
	baseTransport := &http.Transport{
		MaxIdleConns:        env.GetInt("MAX_IDLE_CONNS", 1000),
		MaxIdleConnsPerHost: env.GetInt("MAX_IDLE_CONNS_PER_HOST", 100),
		IdleConnTimeout:     90 * time.Second,
	}

	// Initialize protocol dispatcher and register respective health-check handlers
	dispatcher := prober.NewDispatcher()

	// Register HTTP/HTTPS handlers
	httpProber := prober.NewHTTPProber(baseTransport)
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
		go prober.WorkerPool(ctx, jobs, dispatcher, probeTimeout, &workerWG)
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

	// Initialize the unified KubeWatcher for EndpointSlices and peer topology
	watcher := prober.NewKubeWatcher(clientset, registry)

	var informerWG sync.WaitGroup

	// 1. Prober Peer Discovery Informer
	informerWG.Add(1)
	go func() {
		defer informerWG.Done()
		watcher.WatchPeers(ctx)
	}()

	// StaticTarget CRDs Informer
	informerWG.Add(1)
	go func() {
		defer informerWG.Done()
		if err := prober.WatchStaticTargets(ctx, dynClient, registry); err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("StaticTargets informer stopped", "error", err)
		}
	}()

	// EndpointSlice Informer
	informerWG.Add(1)
	go func() {
		defer informerWG.Done()
		if err := watcher.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("Informer watcher stopped", "error", err)
		}
	}()

	// Close registry.Events strictly after all informers have terminated
	go func() {
		<-ctx.Done()
		informerWG.Wait()
		registry.Close()
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
	var eventLoopWG sync.WaitGroup
	eventLoopWG.Add(1)

	go func() {
		defer eventLoopWG.Done()
		// Liest automatisch alle Events ab und beendet sich, sobald registry.Close() aufgerufen wird
		for evt := range registry.Events {
			if evt.IsAdded {
				cleaner.AbortDeletion(evt.Target.Address)

				var shouldStart bool
				var schedCtx context.Context

				schedMu.Lock()
				if _, exists := activeSchedulers[evt.Target.Address]; !exists {
					var schedCancel context.CancelFunc
					schedCtx, schedCancel = context.WithCancel(ctx)
					activeSchedulers[evt.Target.Address] = schedCancel
					shouldStart = true
				}
				schedMu.Unlock()

				if shouldStart {
					slog.Info("New target discovered", "target", evt.Target.Address, "scheme", evt.Target.Scheme)
					schedulerWG.Add(1)
					go prober.TargetScheduler(schedCtx, evt.Target, jobs, probeInterval, &schedulerWG)
				}
			} else {
				schedMu.Lock()
				cancelFunc, exists := activeSchedulers[evt.Target.Address]
				if exists {
					delete(activeSchedulers, evt.Target.Address)
				}
				schedMu.Unlock()

				if exists {
					slog.Info("Target removed", "target", evt.Target.Address)
					cancelFunc()
					cleaner.MarkForDeletion(evt.Target.Address)
				}
			}
		}
	}()

	// Block main routine until an OS termination signal is received
	<-ctx.Done()
	slog.Info("Shutting down cleanly...")

	// Wait till all Informers terminated
	eventLoopWG.Wait()

	// Stop all active target schedulers (stop producing new jobs)
	schedMu.Lock()
	for _, cancelFn := range activeSchedulers {
		cancelFn()
	}
	schedMu.Unlock()

	// Wait until all scheduler loops exit
	schedulerWG.Wait()

	// Close the channel to let workers drain the remaining queue
	close(jobs)

	// Wait for all workers to finish remaining probe executions
	workerWG.Wait()

	// Shutdown HTTP server after all metrics updates are complete
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)

	slog.Info("Goodbye.")
}

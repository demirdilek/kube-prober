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
	"golang.org/x/sync/errgroup"

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
	for range numWorkers {
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

	// Erstellt eine Group, die an den Parent-Context gekoppelt ist
	informerGroup, informerCtx := errgroup.WithContext(ctx)

	// 1. Prober Peer Discovery Informer
	informerGroup.Go(func() error {
		watcher.WatchPeers(informerCtx)
		return nil
	})

	// 2. StaticTarget CRDs Informer
	informerGroup.Go(func() error {
		if err := prober.WatchStaticTargets(informerCtx, dynClient, registry); err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
		return nil
	})

	// 3. EndpointSlice Informer
	informerGroup.Go(func() error {
		if err := watcher.Start(informerCtx); err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
		return nil
	})

	// Track active per-target scheduler cancellation functions
	activeSchedulers := make(map[string]context.CancelFunc)
	var schedMu sync.Mutex

	// 1. Initialize cleaner with the metric purge callback
	cleaner := server.NewMetricsCleaner(prober.DeleteTargetMetrics)
	// Start the internal HTTP server to expose Prometheus metrics and health probes (:8080)
	srv := server.New(":8080", cleaner)
	go srv.Start()

	// Event loop: handle target additions, rebalancing decisions, and removals from the registry
	eventLoopDone := make(chan struct{})

	go func() {
		defer close(eventLoopDone)

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

	// 1. Cancel target schedulers immediately to stop generating new probe jobs
	schedMu.Lock()
	for _, cancelFn := range activeSchedulers {
		cancelFn()
	}

	schedMu.Unlock()
	schedulerWG.Wait()

	// 2. Wait for informers to fully stop, then close registry
	if err := informerGroup.Wait(); err != nil {
		slog.Error("Informer error during shutdown", "error", err)
	}
	registry.Close()

	// 3. Wait for the event loop to drain and terminate
	<-eventLoopDone

	// 4. Drain and close worker jobs
	close(jobs)
	workerWG.Wait()
	// Shutdown HTTP server after all metrics updates are complete
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)

	slog.Info("Goodbye.")
}

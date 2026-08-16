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

func init() {
	prober.RegisterMetrics(prometheus.DefaultRegisterer)
}

func main() {
	// Setup graceful shutdown context listening for SIGINT and SIGTERM OS signals
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	// Configure worker pool capacity and HTTP client options for heavy concurrent probing
	numWorkers := env.GetInt("WORKERS", 50)
	prober.MaxWorkersGauge.Set(float64(numWorkers))
	jobQueueSize := env.GetInt("QUEUE_SIZE", 10000)
	probeInterval := time.Duration(env.GetInt("PROBE_INTERVAL_SECONDS", 2)) * time.Second
	httpTimeout := time.Duration(env.GetInt("HTTP_TIMEOUT_SECONDS", 5)) * time.Second

	// Pre-configure HTTP transport with aggressive connection pooling for high-throughput reuse
	httpClient := &http.Client{
		Timeout: httpTimeout,
		Transport: &http.Transport{
			MaxIdleConns:        env.GetInt("MAX_IDLE_CONNS", 1000),
			MaxIdleConnsPerHost: env.GetInt("MAX_IDLE_CONNS_PER_HOST", 100),
			IdleConnTimeout:     90 * time.Second,
		},
	}

	// Initialize the Dispatcher
	dispatcher := prober.NewDispatcher()

	// Register HTTP handlers
	// Note: Ensure your `NewHTTPProber` function signature in http.go accepts `*http.Client`
	// so it can use this highly-tuned client!
	httpProber := prober.NewHTTPProber(httpClient)
	dispatcher.Register("http", httpProber.ProbeHTTPTarget)
	dispatcher.Register("https", httpProber.ProbeHTTPTarget)

	// Register TCP handlers
	tcpProber := prober.NewTCPProber()
	dispatcher.Register("tcp", tcpProber.ProbeTCPTarget)

	// Configure TLS verification mode via environment variable
	var tlsCfg *tls.Config
	if os.Getenv("TLS_INSECURE_SKIP_VERIFY") == "true" {
		slog.Warn("TLS verification disabled (InsecureSkipVerify=true) - run only in dev/test environments")
		tlsCfg = &tls.Config{InsecureSkipVerify: true}
	}

	// Register TLS handlers
	tlsProber := prober.NewTLSProber(tlsCfg)
	dispatcher.Register("tls", tlsProber.ProbeTLSTarget)

	// Register gRPC handlers
	// (Using the modernized NewGRPCProber we built that defaults to insecure credentials)
	grpcProber := prober.NewGRPCProber()
	dispatcher.Register("grpc", grpcProber.ProbeGRPCTarget)

	// Register DNS handlers
	dnsProber := prober.NewDNSProber()
	dispatcher.Register("dns", dnsProber.ProbeDNSTarget)

	var wg sync.WaitGroup
	jobs := make(chan prober.Job, jobQueueSize)

	// Spawn worker pool goroutines to process incoming probe jobs concurrently
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go prober.WorkerPool(ctx, jobs, dispatcher, &wg)
	}

	clientset, err := kube.InitClient()
	if err != nil {
		slog.Error("Initialization failed for clientset", "error", err)
		os.Exit(1)
	}

	dynClient, err := kube.InitDynamicClient()
	if err != nil {
		slog.Error("Initialization failed for dynamic client", "error", err)
		os.Exit(1)
	}

	selfIP := os.Getenv("POD_IP")
	registry := prober.NewRegistry(selfIP)

	// Start CRD Watcher Informer in background
	go func() {
		if err := prober.WatchStaticTargets(ctx, dynClient, registry); err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("StaticTargets informer stopped", "error", err)
		}
	}()

	// Initialize the unified KubeWatcher for both peer topology and target discovery
	watcher := prober.NewKubeWatcher(clientset, registry)

	// 1. Watch peer replicas dynamically to rebalance targets upon HPA scaling events
	go watcher.WatchPeers(ctx)

	// 2. Start the EndpointSlice informer in a background goroutine to stream target updates asynchronously
	go func() {
		if err := watcher.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("Informer watcher stopped", "error", err)
		}
	}()

	activeSchedulers := make(map[string]context.CancelFunc)
	var schedMu sync.Mutex

	// Event loop: process target assignments emitted by the sharding registry
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
						// Target assigned to this replica: start local periodic probe scheduler
						if _, exists := activeSchedulers[evt.Target.Address]; !exists {
							slog.Info("New target discovered", "target", evt.Target.Address, "scheme", evt.Target.Scheme)
							schedCtx, schedCancel := context.WithCancel(ctx)
							activeSchedulers[evt.Target.Address] = schedCancel

							wg.Add(1)
							go prober.TargetScheduler(schedCtx, evt.Target, jobs, probeInterval, &wg)
						}
					} else {
						// Target revoked or deleted: cancel local scheduler and purge metrics
						if cancelFunc, exists := activeSchedulers[evt.Target.Address]; exists {
							slog.Info("Target removed", "target", evt.Target.Address)
							cancelFunc()
							delete(activeSchedulers, evt.Target.Address)

							// Clean up all metrics associated with the removed target safely
							go func(targetAddr string) {
								// Wait for Prometheus to scrape the final state
								time.Sleep(6 * time.Second)

								// Lock the scheduler mutex to safely check the map
								schedMu.Lock()
								_, reAdded := activeSchedulers[targetAddr]
								schedMu.Unlock()

								// Only delete metrics if the target hasn't been re-added in the meantime
								if !reAdded {
									prober.DeleteTargetMetrics(targetAddr)
									slog.Debug("Metrics purged for removed target", "target", targetAddr)
								}
							}(evt.Target.Address)
						}
					}
				}()
			}
		}
	}()

	// Start the HTTP server to expose metrics and health endpoints
	srv := server.New(":8080")
	go srv.Start()

	// Wait for shutdown signal
	<-ctx.Done()
	slog.Info("Shutting down cleanly...")

	// 1. Cancel all active schedulers to stop periodic probing
	schedMu.Lock()
	for _, cancelFn := range activeSchedulers {
		cancelFn()
	}
	schedMu.Unlock()

	// 2. Close the job queue to signal workers to exit
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	_ = srv.Shutdown(shutdownCtx)
	wg.Wait()
	slog.Info("Goodbye.")
}

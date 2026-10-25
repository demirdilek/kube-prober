package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"time"
)

// Server encapsulates the HTTP telemetry and health check endpoints.
type Server struct {
	srv     *http.Server
	Cleaner *MetricsCleaner
}

// New constructs an HTTP server with metrics, health probes, and pprof profiling.
func New(addr string, cleaner *MetricsCleaner) *Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", cleaner.Handler())

	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("READY"))
	})

	return &Server{
		srv: &http.Server{
			Addr:              addr,
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		},
	}
}

// Start initiates the HTTP server listener routine.
func (s *Server) Start() {
	slog.Info("Metric server starting", "addr", s.srv.Addr)
	if err := s.srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		slog.Error("Metric server failed to run", "error", err)
	}
}

// Shutdown gracefully stops the server within the provided context deadline.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}

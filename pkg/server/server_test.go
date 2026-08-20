package server

import (
	"context"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestServer_EndpointsAndLifecycle(t *testing.T) {
	cleaner := NewMetricsCleaner(func(target string) {})

	addr := "127.0.0.1:18080"
	srv := New(addr, cleaner)

	go srv.Start()
	time.Sleep(50 * time.Millisecond)

	baseURL := "http://" + addr

	tests := []struct {
		name           string
		path           string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "Healthz endpoint",
			path:           "/healthz",
			expectedStatus: http.StatusOK,
			expectedBody:   "OK",
		},
		{
			name:           "Readyz endpoint",
			path:           "/readyz",
			expectedStatus: http.StatusOK,
			expectedBody:   "READY",
		},
		{
			name:           "Metrics endpoint",
			path:           "/metrics",
			expectedStatus: http.StatusOK,
			expectedBody:   "",
		},
		{
			name:           "Pprof debug index",
			path:           "/debug/pprof/",
			expectedStatus: http.StatusOK,
			expectedBody:   "",
		},
	}

	client := &http.Client{Timeout: 2 * time.Second}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := client.Get(baseURL + tt.path)
			if err != nil {
				t.Fatalf("failed to perform GET request to %s: %v", tt.path, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("expected status %d for %s, got %d", tt.expectedStatus, tt.path, resp.StatusCode)
			}

			if tt.expectedBody != "" {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Fatalf("failed to read body: %v", err)
				}
				if string(body) != tt.expectedBody {
					t.Errorf("expected body %q, got %q", tt.expectedBody, string(body))
				}
			}
		})
	}

	t.Run("MetricsCleaner_PostScrapeExecution", func(t *testing.T) {
		var mu sync.Mutex
		deletedTargets := make(map[string]bool)

		testCleaner := NewMetricsCleaner(func(target string) {
			mu.Lock()
			defer mu.Unlock()
			deletedTargets[target] = true
		})

		testAddr := "127.0.0.1:18081"
		subSrv := New(testAddr, testCleaner)
		go subSrv.Start()
		time.Sleep(30 * time.Millisecond)

		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()
			_ = subSrv.Shutdown(ctx)
		}()

		dummyTarget := "http://10.0.0.99:80/healthz"
		testCleaner.MarkForDeletion(dummyTarget)

		resp, err := client.Get("http://" + testAddr + "/metrics")
		if err != nil {
			t.Fatalf("failed to scrape /metrics: %v", err)
		}
		_ = resp.Body.Close()

		var cleaned bool
		for i := 0; i < 20; i++ {
			mu.Lock()
			cleaned = deletedTargets[dummyTarget]
			mu.Unlock()
			if cleaned {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}

		if !cleaned {
			t.Errorf("expected target %s to be cleaned up after scrape, but callback was not invoked", dummyTarget)
		}
	})

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		t.Errorf("expected clean server shutdown, got error: %v", err)
	}
}

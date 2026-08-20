package server

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestMetricsCleaner_Lifecycle(t *testing.T) {
	deleted := make(map[string]int)
	var mu sync.Mutex

	cleaner := NewMetricsCleaner(func(target string) {
		mu.Lock()
		defer mu.Unlock()
		deleted[target]++
	})

	handler := cleaner.Handler()
	targetA := "http://10.0.0.1:80/healthz"
	targetB := "http://10.0.0.2:80/healthz"

	// 1. Mark targetA for deletion
	cleaner.MarkForDeletion(targetA)

	// 2. First scrape: should serve metrics and trigger deletion for targetA
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200 on scrape, got %d", rr.Code)
	}

	mu.Lock()
	if deleted[targetA] != 1 {
		t.Errorf("expected targetA to be deleted once after scrape, got %d", deleted[targetA])
	}
	mu.Unlock()

	// 3. Second scrape: targetA was already removed, delete callback should not run again
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	mu.Lock()
	if deleted[targetA] != 1 {
		t.Errorf("expected targetA deletion count to remain 1, got %d", deleted[targetA])
	}
	mu.Unlock()

	// 4. Test AbortDeletion (Target re-added before scrape happens)
	cleaner.MarkForDeletion(targetB)
	cleaner.AbortDeletion(targetB)

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	mu.Lock()
	if deleted[targetB] != 0 {
		t.Errorf("expected targetB not to be deleted after abort, got %d", deleted[targetB])
	}
	mu.Unlock()
}

func TestMetricsCleaner_ConcurrentAccess(t *testing.T) {
	cleaner := NewMetricsCleaner(func(target string) {})
	handler := cleaner.Handler()

	var wg sync.WaitGroup
	workers := 25

	for i := 0; i < workers; i++ {
		wg.Add(3)

		// Concurrent marks
		go func(idx int) {
			defer wg.Done()
			cleaner.MarkForDeletion("http://target-test:80")
		}(i)

		// Concurrent aborts
		go func(idx int) {
			defer wg.Done()
			cleaner.AbortDeletion("http://target-test:80")
		}(i)

		// Concurrent scrapes
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
		}()
	}

	wg.Wait()
}

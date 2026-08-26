package prober

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPProber_ProbeHTTPTarget(t *testing.T) {
	// Start a local HTTP server with custom route handlers to simulate SRE scenarios
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/error" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if r.URL.Path == "/timeout" {
			time.Sleep(50 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	baseTransport := &http.Transport{}

	prober := NewHTTPProber(baseTransport)

	tests := []struct {
		name      string
		target    Target
		timeout   time.Duration
		expectErr bool
	}{
		{
			name: "Successful HTTP request",
			target: Target{
				Name:    "success-endpoint",
				Address: ts.URL,
				Scheme:  "http",
			},
			timeout:   2 * time.Second,
			expectErr: false,
		},
		{
			name: "Server returns HTTP 500 error",
			target: Target{
				Name:    "error-endpoint",
				Address: ts.URL + "/error",
				Scheme:  "http",
			},
			timeout:   2 * time.Second,
			expectErr: true,
		},
		{
			name: "Context timeout during request",
			target: Target{
				Name:    "timeout-endpoint",
				Address: ts.URL + "/timeout",
				Scheme:  "http",
			},
			timeout:   10 * time.Millisecond,
			expectErr: true,
		},
		{
			name: "Connection refused on invalid port",
			target: Target{
				Name:    "refused-endpoint",
				Address: "http://127.0.0.1:59844",
				Scheme:  "http",
			},
			timeout:   2 * time.Second,
			expectErr: true,
		},
		{
			name: "Invalid URL structure",
			target: Target{
				Name:    "invalid-url",
				Address: "%%%invalid-url",
				Scheme:  "http",
			},
			timeout:   2 * time.Second,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			got := prober.ProbeHTTPTarget(ctx, tt.target)

			if tt.expectErr && got == "" {
				t.Errorf("ProbeHTTPTarget() expected an error category, got success")
			}
			if !tt.expectErr && got != "" {
				t.Errorf("ProbeHTTPTarget() expected success, got error category: %v", got)
			}
		})
	}
}

func TestHTTPProber_InsecureSkipVerify(t *testing.T) {
	// Start local TLS server (generates a self-signed certificate)
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	baseTransport := &http.Transport{}
	prober := NewHTTPProber(baseTransport)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 1. Without InsecureSkipVerify: should fail with TLS error
	strictTarget := Target{
		Name:               "strict-tls-target",
		Address:            ts.URL,
		Scheme:             "https",
		InsecureSkipVerify: false,
	}
	if errCat := prober.ProbeHTTPTarget(ctx, strictTarget); errCat != CategoryTLS {
		t.Errorf("expected CategoryTLS for self-signed cert without skip-verify, got %q", errCat)
	}

	// 2. With InsecureSkipVerify: should succeed
	insecureTarget := Target{
		Name:               "insecure-tls-target",
		Address:            ts.URL,
		Scheme:             "https",
		InsecureSkipVerify: true,
	}
	if errCat := prober.ProbeHTTPTarget(ctx, insecureTarget); errCat != "" {
		t.Errorf("expected success for target with InsecureSkipVerify=true, got %q", errCat)
	}
}

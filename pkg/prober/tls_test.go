package prober

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTLSProber_ProbeTLSTarget(t *testing.T) {
	// Start a local TLS server
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()

	// Use InsecureSkipVerify to bypass local certificate SAN/IP mismatches during the test
	testConfig := &tls.Config{
		InsecureSkipVerify: true,
	}
	prober := NewTLSProber(testConfig)
	validTarget := Target{
		Name:    "valid-tls-server",
		Address: "tls://" + ts.Listener.Addr().String(),
		Scheme:  "tls",
	}

	refusedTarget := Target{
		Name:    "refused-tls-target",
		Address: "tls://127.0.0.1:59845",
		Scheme:  "tls",
	}

	tests := []struct {
		name      string
		target    Target
		timeout   time.Duration
		expectErr bool
	}{
		{
			name:      "Successful TLS handshake",
			target:    validTarget,
			timeout:   2 * time.Second,
			expectErr: false,
		},
		{
			name:      "Connection refused",
			target:    refusedTarget,
			timeout:   2 * time.Second,
			expectErr: true,
		},
		{
			name: "Timeout during connection or handshake",
			// Use a non-routable IP (TEST-NET-1) to force a timeout
			target: Target{
				Name:    "timeout-tls",
				Address: "tls://198.51.100.1:443",
				Scheme:  "tls",
			},
			timeout:   1 * time.Millisecond,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			got := prober.ProbeTLSTarget(ctx, tt.target)

			if tt.expectErr && got == "" {
				t.Errorf("ProbeTLSTarget() expected an error category, got success")
			}
			if !tt.expectErr && got != "" {
				t.Errorf("ProbeTLSTarget() expected success, got error category: %v", got)
			}
		})
	}
}

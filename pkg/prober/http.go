package prober

import (
	"context"
	"io"
	"net/http"
	"strings"
)

// HTTPProber executes HTTP and HTTPS connection checks.
type HTTPProber struct {
	client *http.Client
}

// NewHTTPProber creates a new HTTP prober with the provided http.Client.
func NewHTTPProber(client *http.Client) *HTTPProber {
	return &HTTPProber{
		client: client,
	}
}

// ProbeHTTPTarget attempts to perform an HTTP GET request against the target.
func (p *HTTPProber) ProbeHTTPTarget(ctx context.Context, target Target) ErrorCategory {
	address := target.Address
	if !strings.HasPrefix(address, "http://") && !strings.HasPrefix(address, "https://") {
		address = target.Scheme + "://" + address
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return CategoryUnknown
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return MapToCategory(err, 0)
	}

	// Drain and close the response body to reuse the TCP connection in the pool
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	// Classify HTTP 4xx and 5xx responses as errors for SRE Golden Signals
	if resp.StatusCode >= 400 {
		return MapToCategory(nil, resp.StatusCode)
	}

	return ""
}

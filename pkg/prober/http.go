package prober

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"strings"
)

// HTTPProber executes HTTP and HTTPS connection checks.
type HTTPProber struct {
	defaultClient  *http.Client
	insecureClient *http.Client
}

// NewHTTPProber creates a new HTTP prober.
// It sets up both a standard verifying client and an insecure client for self-signed targets.
func NewHTTPProber(baseTransport *http.Transport) *HTTPProber {
	// Standard Transport
	stdTransport := baseTransport.Clone()

	// Insecure Transport with TLS verification disabled
	insecureTransport := baseTransport.Clone()
	if insecureTransport.TLSClientConfig == nil {
		insecureTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // #nosec G402 -- Configurable for test targets
	} else {
		insecureTransport.TLSClientConfig = insecureTransport.TLSClientConfig.Clone()
		insecureTransport.TLSClientConfig.InsecureSkipVerify = true
	}

	return &HTTPProber{
		defaultClient:  &http.Client{Transport: stdTransport},
		insecureClient: &http.Client{Transport: insecureTransport},
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

	// Choose appropriate client based on target configuration
	client := p.defaultClient
	if target.InsecureSkipVerify {
		client = p.insecureClient
	}

	resp, err := client.Do(req)
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

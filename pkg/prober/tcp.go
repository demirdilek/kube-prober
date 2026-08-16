package prober

import (
	"context"
	"net"
	"net/url"
	"strings"
)

// TCPProber executes raw TCP connection checks.
type TCPProber struct{}

// NewTCPProber creates a new TCP prober.
func NewTCPProber() *TCPProber {
	return &TCPProber{}
}

// ProbeTCPTarget attempts to establish a TCP connection to the target.
func (p *TCPProber) ProbeTCPTarget(ctx context.Context, target Target) ErrorCategory {
	// Fail fast on completely malformed strings to satisfy the test logic
	parsedURL, err := url.Parse(target.Address)
	if err != nil {
		return CategoryUnknown
	}

	host := target.Address
	if parsedURL.Host != "" {
		host = parsedURL.Host
	} else if strings.HasPrefix(target.Address, "tcp://") {
		host = strings.TrimPrefix(target.Address, "tcp://")
	}

	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", host)
	if err != nil {
		return MapToCategory(err, 0)
	}

	_ = conn.Close()
	return ""
}

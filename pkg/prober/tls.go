package prober

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/url"
	"os"
	"strings"
	"time"
)

// TLSProber executes TLS handshake, certificate validation, and expiry checks.
type TLSProber struct {
	config *tls.Config
}

// NewTLSProber creates a new TLS prober.
// Passing nil for baseCfg uses standard secure defaults and loads the in-cluster Kubernetes CA.
func NewTLSProber(baseCfg *tls.Config) *TLSProber {
	if baseCfg != nil {
		return &TLSProber{config: baseCfg}
	}

	rootCAs, _ := x509.SystemCertPool()
	if rootCAs == nil {
		rootCAs = x509.NewCertPool()
	}

	// Load cluster-internal CA certificate to validate internal Service TLS connections
	caCertPath := "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
	if caCert, err := os.ReadFile(caCertPath); err == nil {
		rootCAs.AppendCertsFromPEM(caCert)
	}

	return &TLSProber{
		config: &tls.Config{
			RootCAs: rootCAs,
		},
	}
}

// ProbeTLSTarget establishes a raw TCP socket, completes a TLS handshake using the request context,
// and records the remaining certificate validity days for Prometheus metrics.
func (p *TLSProber) ProbeTLSTarget(ctx context.Context, target Target) ErrorCategory {
	address := target.Address

	// Safely strip scheme prefix to isolate host:port for network dialing
	if parsedURL, err := url.Parse(target.Address); err == nil && parsedURL.Host != "" {
		address = parsedURL.Host
	} else if strings.HasPrefix(target.Address, "tls://") {
		address = strings.TrimPrefix(target.Address, "tls://")
	}

	// Default to standard TLS port 443 if omitted
	if !strings.Contains(address, ":") {
		address += ":443"
	}

	// Extract hostname without port for SNI ServerName verification
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}

	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return MapToCategory(err, 0)
	}
	defer conn.Close()

	// Clone base config to safely assign target-specific SNI and skip-verify flags
	cfg := p.config.Clone()
	if cfg.ServerName == "" {
		cfg.ServerName = host
	}
	if target.InsecureSkipVerify {
		cfg.InsecureSkipVerify = true
	}

	tlsConn := tls.Client(conn, cfg)

	// Execute TLS handshake synchronously using context; eliminates unbuffered goroutine leaks on timeouts
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return MapToCategory(err, 0)
	}

	// Calculate and record certificate expiration telemetry
	state := tlsConn.ConnectionState()
	if len(state.PeerCertificates) > 0 {
		cert := state.PeerCertificates[0]
		daysRemaining := time.Until(cert.NotAfter).Hours() / 24
		TLSCertExpiryGauge.WithLabelValues(target.Address).Set(daysRemaining)
	}

	return ""
}

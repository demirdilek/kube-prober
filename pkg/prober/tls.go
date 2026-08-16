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

// TLSProber executes TLS handshake and certificate validation checks.
type TLSProber struct {
	config *tls.Config
}

// NewTLSProber creates a new TLS prober.
// Passing nil for the config will use standard secure defaults including the cluster CA.
func NewTLSProber(baseCfg *tls.Config) *TLSProber {
	if baseCfg != nil {
		return &TLSProber{config: baseCfg}
	}

	rootCAs, _ := x509.SystemCertPool()
	if rootCAs == nil {
		rootCAs = x509.NewCertPool()
	}

	// Cluster internal CA path
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

func (p *TLSProber) ProbeTLSTarget(ctx context.Context, target Target) ErrorCategory {
	address := target.Address

	// Safely strip the scheme so the dialer gets a clean host:port
	if parsedURL, err := url.Parse(target.Address); err == nil && parsedURL.Host != "" {
		address = parsedURL.Host
	} else if strings.HasPrefix(target.Address, "tls://") {
		address = strings.TrimPrefix(target.Address, "tls://")
	}

	// Ensure a port is present for the raw TCP dial
	if !strings.Contains(address, ":") {
		address += ":443"
	}

	// Extract hostname without port for SNI verification
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

	// Clone config to set target-specific ServerName safely
	cfg := p.config.Clone()
	if cfg.ServerName == "" {
		cfg.ServerName = host
	}

	tlsConn := tls.Client(conn, cfg)

	// Perform the handshake in a goroutine to respect the context timeout
	errc := make(chan error, 1)
	go func() {
		errc <- tlsConn.HandshakeContext(ctx)
	}()

	select {
	case <-ctx.Done():
		return CategoryTimeout
	case err := <-errc:
		if err != nil {
			return MapToCategory(err, 0)
		}
	}

	// Record certificate expiry days for telemetry and alerts
	state := tlsConn.ConnectionState()
	if len(state.PeerCertificates) > 0 {
		cert := state.PeerCertificates[0]
		daysRemaining := time.Until(cert.NotAfter).Hours() / 24
		TLSCertExpiryGauge.WithLabelValues(target.Address).Set(daysRemaining)
	}

	return ""
}

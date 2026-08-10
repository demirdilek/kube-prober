package prober

import (
	"context"
	"crypto/tls"
	"net/url"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
)

// GRPCProber
type GRPCProber struct {
	tlsConfig *tls.Config
}

func NewGRPCProber(cfg *tls.Config) *GRPCProber {
	return &GRPCProber{tlsConfig: cfg}
}

func (p *GRPCProber) ProbeGRPCTarget(ctx context.Context, target string) ErrorCategory {
	// 1. We need to parse the URL and the service name from the target

	// If the target does not have a scheme, we assume it's gRPC and prepend "grpc://"
	if !strings.Contains(target, "://") {
		target = "grpc://" + target
	}

	parsedURL, err := url.Parse(target)
	if err != nil {
		return CategoryUnknown
	}
	serviceName := strings.TrimPrefix(parsedURL.Path, "/")
	var opts []grpc.DialOption

	// 2. Decision about TLS secure or not for dial options
	if p.tlsConfig != nil {
		cfg := p.tlsConfig.Clone()
		cfg.ServerName = parsedURL.Hostname()
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(cfg)))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	// 3. establish gRPC Connection
	conn, err := grpc.NewClient(parsedURL.Host, opts...)
	if err != nil {
		return MapToCategory(err, 0)
	}
	defer conn.Close()

	healthClient := healthv1.NewHealthClient(conn)
	// 4. Perform health check
	healthResp, err := healthClient.Check(ctx, &healthv1.HealthCheckRequest{Service: serviceName})
	if err != nil {
		return MapToCategory(err, 0)
	}

	if healthResp.Status != healthv1.HealthCheckResponse_SERVING {
		return CategoryGRPCNotServing
	}

	return ""
}

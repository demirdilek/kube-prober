package prober

import (
	"context"
	"net/url"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

// GRPCProber executes gRPC connection checks.
type GRPCProber struct{}

// NewGRPCProber creates a new gRPC prober.
func NewGRPCProber() *GRPCProber {
	return &GRPCProber{}
}

// ProbeGRPCTarget attempts to establish a gRPC connection and verify health status.
func (p *GRPCProber) ProbeGRPCTarget(ctx context.Context, target Target) ErrorCategory {
	rawAddress := target.Address
	if !strings.Contains(rawAddress, "://") {
		rawAddress = "grpc://" + rawAddress
	}

	address := target.Address
	serviceName := ""

	if parsedURL, err := url.Parse(rawAddress); err == nil {
		if parsedURL.Host != "" {
			address = parsedURL.Host
		}
		// Extract optional service path, e.g. "/UnregisteredService" -> "UnregisteredService"
		serviceName = strings.TrimPrefix(parsedURL.Path, "/")
	}

	// Initialize gRPC client
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return MapToCategory(err, 0)
	}
	defer conn.Close()

	// Perform gRPC Health Check directly with the request context
	healthClient := healthpb.NewHealthClient(conn)

	resp, err := healthClient.Check(ctx, &healthpb.HealthCheckRequest{
		Service: serviceName,
	})
	if err != nil {
		if st, ok := status.FromError(err); ok {
			switch st.Code() {
			case codes.NotFound:
				return CategoryGRPCNotServing
			case codes.Unavailable:
				return CategoryConnectionRefused
			case codes.DeadlineExceeded:
				return CategoryTimeout
			case codes.Unimplemented:
				// Server is reachable on gRPC transport, but does not implement grpc.health.v1
				return ""
			}
		}
		return CategoryGRPCError
	}

	if resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		return CategoryGRPCNotServing
	}

	return "" // Success
}

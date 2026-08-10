package prober

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
)

func TestGRPCProber_ProbeGRPCTarget(t *testing.T) {
	// 1. Set up a local gRPC server with health checking
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	healthServer := health.NewServer()

	// Set the health status for the overall server and a specific service
	healthServer.SetServingStatus("", healthv1.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus("unhealthy-service", healthv1.HealthCheckResponse_NOT_SERVING)

	healthv1.RegisterHealthServer(grpcServer, healthServer)

	go func() {
		_ = grpcServer.Serve(lis)
	}()
	defer grpcServer.Stop()

	prober := NewGRPCProber(nil)
	targetBase := "grpc://" + lis.Addr().String()

	// 2. Define test cases
	tests := []struct {
		name     string
		target   string
		expected ErrorCategory
	}{
		{
			name:     "Overall Health Check SERVING",
			target:   targetBase,
			expected: "",
		},
		{
			name:     "Specific Service NOT_SERVING",
			target:   targetBase + "/unhealthy-service",
			expected: CategoryGRPCNotServing,
		},
		{
			name:     "Connection Refused (Dead Port)",
			target:   "grpc://127.0.0.1:59844",
			expected: CategoryConnectionRefused,
		},
	}

	// 3. Run test cases
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := prober.ProbeGRPCTarget(context.Background(), tt.target)
			if got != tt.expected {
				t.Errorf("ProbeGRPCTarget() = %v, want %v", got, tt.expected)
			}
		})
	}
}

package prober

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func TestGRPCProber_ProbeGRPCTarget(t *testing.T) {
	// Start a local gRPC server to test successful connections
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start local gRPC listener: %v", err)
	}
	defer listener.Close()

	grpcServer := grpc.NewServer()
	go func() {
		_ = grpcServer.Serve(listener)
	}()
	defer grpcServer.Stop()

	// Register standard gRPC health service and set status to SERVING
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus("MyService", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, healthServer)

	validTarget := Target{
		Name:    "local-grpc-server",
		Address: "grpc://" + listener.Addr().String(),
		Scheme:  "grpc",
	}

	// Use an arbitrary unassigned high port to simulate connection refused
	refusedTarget := Target{
		Name:    "refused-target",
		Address: "grpc://127.0.0.1:59843",
		Scheme:  "grpc",
	}

	prober := NewGRPCProber()

	tests := []struct {
		name      string
		target    Target
		timeout   time.Duration
		expectErr bool
	}{
		{
			name:      "Successful gRPC connection",
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
			name: "Connection timeout",
			// Use a non-routable IP (TEST-NET-1) to force a timeout
			target: Target{
				Name:    "timeout-target",
				Address: "grpc://198.51.100.1:80",
				Scheme:  "grpc",
			},
			timeout:   1 * time.Millisecond,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			got := prober.ProbeGRPCTarget(ctx, tt.target)

			// We check boolean expectation here because MapToCategory can sometimes
			// map gRPC block errors differently than raw TCP errors depending on the OS.
			if tt.expectErr && got == "" {
				t.Errorf("ProbeGRPCTarget() expected an error category, got success")
			}
			if !tt.expectErr && got != "" {
				t.Errorf("ProbeGRPCTarget() expected success, got error category: %v", got)
			}
		})
	}
}

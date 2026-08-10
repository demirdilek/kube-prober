package prober

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrorCategory represents the 6 SRE error types
type ErrorCategory string

const (
	CategoryDNS               ErrorCategory = "dns_error"
	CategoryTLS               ErrorCategory = "tls_error"
	CategoryTimeout           ErrorCategory = "timeout"
	CategoryHTTP              ErrorCategory = "http_error"
	CategoryGRPCNotServing    ErrorCategory = "grpc_not_serving"
	CategoryConnectionRefused ErrorCategory = "connection_refused"
	CategoryAuth              ErrorCategory = "authentication_error"
	CategoryUnhealthy         ErrorCategory = "unhealthy_service"
	CategoryUnknown           ErrorCategory = "unknown_error"
)

// Hint returns actionable troubleshooting context for the SRE operator
func (c ErrorCategory) Hint() string {
	switch c {
	case CategoryDNS:
		return "Verify CoreDNS resolution, cluster domain suffixes, or Service name spelling"
	case CategoryTLS:
		return "Check TLS certificate validity, SAN coverage, or CA trust store"
	case CategoryTimeout:
		return "Target is slow to respond; check upstream latency, network network policies, or resource limits"
	case CategoryHTTP:
		return "Application responded with 4xx/5xx; check application logs and payload specs"
	case CategoryGRPCNotServing:
		return "gRPC service is not serving; check service status and logs"
	case CategoryConnectionRefused:
		return "Connection refused; check if the service is running and listening on the specified port"
	case CategoryAuth:
		return "Authentication failed; check credentials, tokens, or permissions"
	case CategoryUnhealthy:
		return "Service is unhealthy; check health checks, service status, and logs"
	case CategoryUnknown:
		fallthrough
	default:
		return "Unclassified network or protocol error; inspect underlying OS/socket errors"
	}
}

// MapToCategory unwraps the error chain and categorizes network/HTTP issues
func MapToCategory(err error, statusCode int) ErrorCategory {
	if err == nil {
		if statusCode >= 400 {
			return CategoryHTTP
		}
		return ""
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return CategoryTimeout
	}

	// gRPC specific error handling
	if st, ok := status.FromError(err); ok {
		switch st.Code() {
		case codes.DeadlineExceeded:
			return CategoryTimeout
		case codes.Unavailable:
			return CategoryConnectionRefused
		case codes.Unauthenticated, codes.PermissionDenied:
			return CategoryAuth
		default:
			return CategoryGRPCNotServing
		}
	}

	// Enhanced TLS / Certificate error checking (unwraps custom CAs and SAN mismatches)
	var certErr *x509.CertificateInvalidError
	var unknownAuthErr x509.UnknownAuthorityError
	var hostnameErr x509.HostnameError
	var recErr tls.RecordHeaderError

	if errors.As(err, &certErr) ||
		errors.As(err, &unknownAuthErr) ||
		errors.As(err, &hostnameErr) ||
		errors.As(err, &recErr) {
		return CategoryTLS
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return CategoryDNS
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if opErr.Timeout() {
			return CategoryTimeout
		}
		return CategoryConnectionRefused
	}

	return CategoryUnknown
}

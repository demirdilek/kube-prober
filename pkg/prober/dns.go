package prober

import (
	"context"
	"net"
	"net/url"
	"strings"
)

// DNSProber executes DNS resolution checks.
type DNSProber struct{}

// NewDNSProber creates a new DNS prober.
func NewDNSProber() *DNSProber {
	return &DNSProber{}
}

// ProbeDNSTarget executes the DNS probe and maps the result to an SRE category.
func (p *DNSProber) ProbeDNSTarget(ctx context.Context, target string) ErrorCategory {
	// --- 1. URL Parsing ---
	// We receive a target string like "dns://10.96.0.10:53/example.com?type=TXT".
	// url.Parse breaks this string into structural components (Scheme, Host, Path, Query).
	parsedURL, err := url.Parse(target)
	if err != nil {
		return CategoryUnknown
	}

	// --- 2. Parameter Extraction ---
	// parsedURL.Path contains "/example.com".
	// We use strings.TrimPrefix to remove the leading slash, leaving just the domain "example.com".
	domainToResolve := strings.TrimPrefix(parsedURL.Path, "/")
	if domainToResolve == "" {
		return CategoryDNS // Fail early if no domain was provided
	}

	// parsedURL.Query() extracts the URL parameters (e.g., "?type=TXT").
	// We fetch the "type" value, default it to "A" if empty, and make it uppercase safely.
	recordType := strings.ToUpper(parsedURL.Query().Get("type"))
	if recordType == "" {
		recordType = "A"
	}

	// --- 3. Resolver Initialization ---
	// We create a custom DNS resolver instead of using the host OS's default DNS settings.
	resolver := &net.Resolver{
		// PreferGo forces the use of Go's built-in DNS resolver instead of the OS's C library (CGO).
		PreferGo: true,
		// The Dial function intercepts the underlying network connection process.
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			var d net.Dialer

			// parsedURL.Host contains our target DNS server (e.g., "10.96.0.10" or "10.96.0.10:53").
			// We split it to check if a port is defined.
			host, port, err := net.SplitHostPort(parsedURL.Host)
			if err != nil {
				// If SplitHostPort fails, it usually means the port was missing.
				host = parsedURL.Host
				port = "53" // 53 is the standard port for DNS
			}

			// Recombine the host and port into a clean address string.
			targetAddr := net.JoinHostPort(host, port)

			// DialContext establishes the actual connection to our specific DNS server.
			// It passes through the timeout context and the required network protocol ("udp" or "tcp").
			return d.DialContext(ctx, network, targetAddr)
		},
	}

	// --- 4. Lookup Execution ---
	var lookupErr error

	// We use a switch statement to call the correct resolver method based on the requested record type.
	// The 'ctx' ensures the operation cancels automatically if it exceeds the probe timeout.
	switch recordType {
	case "TXT":
		_, lookupErr = resolver.LookupTXT(ctx, domainToResolve)
	case "CNAME":
		_, lookupErr = resolver.LookupCNAME(ctx, domainToResolve)
	default:
		// LookupHost resolves A (IPv4) and AAAA (IPv6) records.
		_, lookupErr = resolver.LookupHost(ctx, domainToResolve)
	}

	// --- 5. Error Classification ---
	// If the lookup failed (e.g., NXDOMAIN, timeout, connection refused), we pass the raw Go error
	// to our central mapping function to translate it into a standard SRE ErrorCategory.
	if lookupErr != nil {
		return MapToCategory(lookupErr, 0)
	}

	// Return an empty string to indicate a successful, error-free probe.
	return ""
}

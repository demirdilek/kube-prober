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

// ProbeDNSTarget attempts to resolve the hostname of the target.
func (p *DNSProber) ProbeDNSTarget(ctx context.Context, target Target) ErrorCategory {
	host := target.Address

	// 1. URL parsen oder dns://-Präfix entfernen
	if parsedURL, err := url.Parse(target.Address); err == nil && parsedURL.Host != "" {
		host = parsedURL.Host
	} else if strings.HasPrefix(target.Address, "dns://") {
		host = strings.TrimPrefix(target.Address, "dns://")
	}

	// 2. Port abschneiden, falls vorhanden (z. B. "kube-dns:53" -> "kube-dns")
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	// 3. DNS-Lookup ausführen
	_, err := net.DefaultResolver.LookupHost(ctx, host)
	if err != nil {
		return MapToCategory(err, 0)
	}

	return ""
}

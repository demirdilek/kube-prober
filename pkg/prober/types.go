package prober

// Target represents a single endpoint to be monitored by the prober.
type Target struct {
	Name               string // The namespace/name of the service or the static target name
	Address            string // The full URL or IP:Port to probe
	Scheme             string // The protocol scheme (http, https, tcp, grpc, dns)
	Static             bool   // Indicates if the target comes from a static config instead of dynamic discovery
	InsecureSkipVerify bool   // Target-specific override to bypass TLS certificate verification
}

// TargetEvent is sent through the channel to notify schedulers of target lifecycle changes.
type TargetEvent struct {
	Target  Target
	IsAdded bool // true if the target should be started, false if it should be stopped
}

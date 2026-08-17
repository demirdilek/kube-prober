package prober

import (
	"fmt"
	"hash/fnv"
	"log/slog"
	"sync"
	"time"

	discoveryv1 "k8s.io/api/discovery/v1"
)

// Registry manages the in-memory state of all discovered targets (both dynamic and static).
// It implements Rendezvous Hashing (Highest Random Weight) to ensure deterministic target
// sharding across prober replicas without requiring inter-pod coordination.
type Registry struct {
	mu            sync.RWMutex
	targets       map[string]Target   // Canonical map of all monitored targets keyed by address
	sliceTargets  map[string][]string // Maps an EndpointSlice key (namespace/name) to its generated target addresses
	clusterPodIPs []string            // Current active list of prober pod IPs used for sharding evaluation
	selfPodIP     string              // Local pod IP used to claim target ownership
	active        map[string]bool     // Tracks whether this specific replica is currently probing a target
	Events        chan TargetEvent    // Buffered event stream consumed by the scheduler event loop in main.go
}

// NewRegistry instantiates a thread-safe Registry with pre-allocated tracking structures.
func NewRegistry(selfPodIP string) *Registry {
	return &Registry{
		targets:      make(map[string]Target),
		sliceTargets: make(map[string][]string),
		active:       make(map[string]bool),
		selfPodIP:    selfPodIP,
		Events:       make(chan TargetEvent, 1000), // Sized buffer to absorb bursty Kubernetes watch events
	}
}

// hashTargetAndPod computes a deterministic 64-bit FNV-1a hash for a given (target, podIP) pair.
// A null-byte delimiter is injected between strings to prevent hash collision vulnerabilities
// caused by boundary ambiguities (e.g., "ab" + "cd" vs "a" + "bcd").
func hashTargetAndPod(target, podIP string) uint64 {
	key := target + "\x00" + podIP
	h := fnv.New64a()
	h.Write([]byte(key))
	return h.Sum64()
}

// shouldProcessTargetLocked evaluates whether the local replica owns the given target.
// It iterates across all active peer IPs, finds the pod with the highest hash weight,
// and returns true if the winning pod matches selfPodIP.
//
// Precondition: Caller MUST acquire at least a read lock (r.mu.RLock) before invoking this.
func (r *Registry) shouldProcessTargetLocked(target string) bool {
	// If the pod IP is not configured or there are no peers, fall back to processing all targets locally.
	if r.selfPodIP == "" || len(r.clusterPodIPs) == 0 {
		return true
	}

	var highestHash uint64
	var selectedPod string

	for i, podIP := range r.clusterPodIPs {
		h := hashTargetAndPod(target, podIP)
		// Force assignment on the first iteration to guarantee a valid candidate even if hash is 0
		if i == 0 || h > highestHash {
			highestHash = h
			selectedPod = podIP
		}
	}

	isOwner := selectedPod == r.selfPodIP

	slog.Debug(
		"Sharding evaluation",
		"target", target,
		"assigned_pod", selectedPod,
		"my_pod", r.selfPodIP,
		"is_owner", isOwner,
		"peer_count", len(r.clusterPodIPs),
		"cluster_ips", r.clusterPodIPs,
	)

	return isOwner
}

// ShouldProcessTarget provides thread-safe external access to evaluate target ownership.
func (r *Registry) ShouldProcessTarget(targetAddress string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.shouldProcessTargetLocked(targetAddress)
}

// emitEvent safely pushes a TargetEvent to the internal Events channel.
// Critical removal/stop events (IsAdded: false) are guaranteed delivery via a bounded timeout
// to prevent scheduler leaks and zombie metrics. Transient add events use non-blocking dispatch
// to maintain controller responsiveness under event channel saturation.
func (r *Registry) emitEvent(evt TargetEvent) {
	if !evt.IsAdded {
		// Removal events must be delivered to cancel goroutines and purge metrics
		select {
		case r.Events <- evt:
		case <-time.After(2 * time.Second):
			slog.Error("Critical: Failed to deliver target removal event within timeout", "target", evt.Target.Address)
		}
		return
	}

	// Add events drop non-blockingly if channel is saturated; subsequent sync cycles will recover state
	select {
	case r.Events <- evt:
	default:
		slog.Warn("Registry events channel full, skipping transient add event", "target", evt.Target.Address)
	}
}

// UpdatePeers synchronizes the active prober replica topology and triggers target rebalancing.
// When prober pods scale up or down (e.g., via HPA), target ownership is recomputed
// and respective Add/Remove events are queued.
func (r *Registry) UpdatePeers(peers []string) {
	var eventsToSend []TargetEvent

	func() {
		r.mu.Lock()
		defer r.mu.Unlock()

		r.clusterPodIPs = peers

		// Re-evaluate Rendezvous Hashing ownership for every registered endpoint
		for address, target := range r.targets {
			shouldProcess := r.shouldProcessTargetLocked(address)
			isCurrentlyProcessing := r.active[address]

			if shouldProcess && !isCurrentlyProcessing {
				// Target gained: mark active and emit start event
				r.active[address] = true
				eventsToSend = append(eventsToSend, TargetEvent{
					Target:  target,
					IsAdded: true,
				})
			} else if !shouldProcess && isCurrentlyProcessing {
				// Target lost to another peer: mark inactive and emit stop event
				r.active[address] = false
				eventsToSend = append(eventsToSend, TargetEvent{
					Target:  target,
					IsAdded: false,
				})
			}
		}
	}()

	for _, evt := range eventsToSend {
		r.emitEvent(evt)
	}
}

// UpdateFromEndpointSlice reconciles endpoints discovered dynamically from Kubernetes EndpointSlices.
// It parses ready pod addresses, filters duplicates, respects CRD static target precedence,
// and emits lifecycle events for newly acquired or decommissioned endpoints.
func (r *Registry) UpdateFromEndpointSlice(slice *discoveryv1.EndpointSlice, scheme, path string) {
	var eventsToSend []TargetEvent

	func() {
		r.mu.Lock()
		defer r.mu.Unlock()

		sliceKey := slice.Namespace + "/" + slice.Name
		newTargetMap := make(map[string]bool)
		var newTargetList []string

		// Extract available service ports or fall back to default HTTP port 80
		var activePorts []int32
		if len(slice.Ports) == 0 {
			activePorts = append(activePorts, 80)
		} else {
			for _, port := range slice.Ports {
				if port.Port != nil {
					activePorts = append(activePorts, *port.Port)
				}
			}
		}

		// Construct formatted URLs for ready endpoints
		for _, ep := range slice.Endpoints {
			if ep.Conditions.Ready == nil || !*ep.Conditions.Ready {
				continue
			}
			for _, port := range activePorts {
				for _, ip := range ep.Addresses {
					address := fmt.Sprintf("%s://%s:%d%s", scheme, ip, port, path)
					newTargetMap[address] = true
					newTargetList = append(newTargetList, address)
				}
			}
		}

		// Phase 1: Clean up endpoints that disappeared from this specific EndpointSlice
		oldTargets := r.sliceTargets[sliceKey]
		for _, oldAddress := range oldTargets {
			if !newTargetMap[oldAddress] {
				// Only delete dynamic endpoints; never purge static targets defined via CRD
				if existing, exists := r.targets[oldAddress]; exists && !existing.Static {
					delete(r.targets, oldAddress)

					wasActive := r.active[oldAddress]
					delete(r.active, oldAddress)

					if wasActive {
						eventsToSend = append(eventsToSend, TargetEvent{
							Target:  Target{Address: oldAddress},
							IsAdded: false,
						})
					}
				}
			}
		}

		// Phase 2: Register new dynamic endpoints (skipping existing static CRD targets)
		for _, newAddress := range newTargetList {
			if existing, exists := r.targets[newAddress]; exists && existing.Static {
				// Static targets take precedence over dynamic discovery
				continue
			}

			if _, exists := r.targets[newAddress]; !exists {
				fullTarget := Target{
					Name:    sliceKey,
					Address: newAddress,
					Scheme:  scheme,
					Static:  false,
				}
				r.targets[newAddress] = fullTarget

				// If this replica owns the newly discovered target, schedule it
				if r.shouldProcessTargetLocked(newAddress) {
					r.active[newAddress] = true
					eventsToSend = append(eventsToSend, TargetEvent{
						Target:  fullTarget,
						IsAdded: true,
					})
				}
			}
		}

		r.sliceTargets[sliceKey] = newTargetList
	}()

	for _, evt := range eventsToSend {
		r.emitEvent(evt)
	}
}

// RemoveEndpointSlice unregisters all dynamic targets associated with a deleted EndpointSlice resource.
func (r *Registry) RemoveEndpointSlice(slice *discoveryv1.EndpointSlice, scheme, path string) {
	var eventsToSend []TargetEvent

	func() {
		r.mu.Lock()
		defer r.mu.Unlock()

		sliceKey := slice.Namespace + "/" + slice.Name
		oldTargets := r.sliceTargets[sliceKey]

		for _, oldAddress := range oldTargets {
			// Retain targets that are backed by a static CRD definition
			if existing, exists := r.targets[oldAddress]; exists && existing.Static {
				continue
			}

			delete(r.targets, oldAddress)
			wasActive := r.active[oldAddress]
			delete(r.active, oldAddress)

			if wasActive {
				eventsToSend = append(eventsToSend, TargetEvent{
					Target:  Target{Address: oldAddress},
					IsAdded: false,
				})
			}
		}

		delete(r.sliceTargets, sliceKey)
	}()

	for _, evt := range eventsToSend {
		r.emitEvent(evt)
	}
}

// Add registers a declarative static target from CRD definitions.
// Static targets are given top precedence and will overwrite existing dynamic targets.
func (r *Registry) Add(target Target) {
	var eventToSend *TargetEvent

	func() {
		r.mu.Lock()
		defer r.mu.Unlock()

		target.Static = true
		existing, exists := r.targets[target.Address]

		// Insert or upgrade dynamic target to static
		if !exists || !existing.Static {
			r.targets[target.Address] = target

			if r.shouldProcessTargetLocked(target.Address) {
				r.active[target.Address] = true
				eventToSend = &TargetEvent{
					Target:  target,
					IsAdded: true,
				}
			}
		}
	}()

	if eventToSend != nil {
		r.emitEvent(*eventToSend)
	}
}

// Remove unregisters a target by its address and emits a removal event if currently active.
func (r *Registry) Remove(address string) {
	var eventToSend *TargetEvent

	func() {
		r.mu.Lock()
		defer r.mu.Unlock()

		if target, exists := r.targets[address]; exists {
			delete(r.targets, address)
			wasActive := r.active[address]
			delete(r.active, address)

			if wasActive {
				eventToSend = &TargetEvent{
					Target:  target,
					IsAdded: false,
				}
			}
		}
	}()

	if eventToSend != nil {
		r.emitEvent(*eventToSend)
	}
}

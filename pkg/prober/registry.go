package prober

import (
	"fmt"
	"hash/fnv"
	"log/slog"
	"sync"

	discoveryv1 "k8s.io/api/discovery/v1"
)

// Registry holds the state of all discovered targets.
type Registry struct {
	mu            sync.RWMutex
	targets       map[string]Target
	sliceTargets  map[string][]string
	clusterPodIPs []string
	selfPodIP     string
	active        map[string]bool
	Events        chan TargetEvent
}

// NewRegistry initializes a new Registry.
func NewRegistry(selfPodIP string) *Registry {
	return &Registry{
		targets:      make(map[string]Target),
		sliceTargets: make(map[string][]string),
		active:       make(map[string]bool),
		selfPodIP:    selfPodIP,
		Events:       make(chan TargetEvent, 1000),
	}
}

// hashTargetAndPod creates a deterministic hash for Rendezvous Hashing.
func hashTargetAndPod(target, podIP string) uint64 {
	// Use a null-byte separator to prevent string concatenation collisions
	key := target + "\x00" + podIP
	h := fnv.New64a()
	h.Write([]byte(key))
	return h.Sum64()
}

// shouldProcessTargetLocked checks if this replica should process the target.
// The caller MUST hold r.mu (Lock or RLock) before calling this function.
// ShouldProcessTarget is the safe, exported version for external calls.
func (r *Registry) shouldProcessTargetLocked(target string) bool {
	if r.selfPodIP == "" || len(r.clusterPodIPs) == 0 {
		return true
	}

	var highestHash uint64
	var selectedPod string

	for i, podIP := range r.clusterPodIPs {
		h := hashTargetAndPod(target, podIP)
		// CRITICAL FIX: Force initialization on the first iteration
		if i == 0 || h > highestHash {
			highestHash = h
			selectedPod = podIP
		}
	}

	isOwner := selectedPod == r.selfPodIP

	// DEBUG: Log the sharding decision and the current state of cluster IPs
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

// ShouldProcessTarget checks if this replica should process the target.
// This is the safe, exported version for external calls.
func (r *Registry) ShouldProcessTarget(targetAddress string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.shouldProcessTargetLocked(targetAddress)
}

// UpdatePeers updates the list of active prober pods and rebalances targets.
func (r *Registry) UpdatePeers(peers []string) {
	var eventsToSend []TargetEvent

	func() {
		r.mu.Lock()
		defer r.mu.Unlock()

		r.clusterPodIPs = peers

		// Re-evaluate ownership for every known target
		for address, target := range r.targets {

			shouldProcess := r.shouldProcessTargetLocked(address)
			isCurrentlyProcessing := r.active[address]

			if shouldProcess && !isCurrentlyProcessing {
				r.active[address] = true
				eventsToSend = append(eventsToSend, TargetEvent{
					Target:  target,
					IsAdded: true,
				})
			} else if !shouldProcess && isCurrentlyProcessing {
				r.active[address] = false
				eventsToSend = append(eventsToSend, TargetEvent{
					Target:  target,
					IsAdded: false,
				})
			}
		}
	}()
	for _, evt := range eventsToSend {
		select {
		case r.Events <- evt:
		default:
			slog.Warn("Registry events channel full, dropping event to prevent deadlock", "target", evt.Target.Address)
		}
	}
}

// UpdateFromEndpointSlice synchronizes the targets for a given EndpointSlice.
func (r *Registry) UpdateFromEndpointSlice(slice *discoveryv1.EndpointSlice, scheme, path string) {
	var eventsToSend []TargetEvent

	func() {
		r.mu.Lock()
		defer r.mu.Unlock()

		sliceKey := slice.Namespace + "/" + slice.Name
		newTargetMap := make(map[string]bool)
		var newTargetList []string

		// Fallback to port 80 if the EndpointSlice defines no ports
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

		// Remove old targets
		oldTargets := r.sliceTargets[sliceKey]
		for _, oldAddress := range oldTargets {
			if !newTargetMap[oldAddress] {
				delete(r.targets, oldAddress)

				// CRITICAL FIX: Only send a stop event if we actually processed it
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

		// Add new targets
		for _, newAddress := range newTargetList {
			if _, exists := r.targets[newAddress]; !exists {
				fullTarget := Target{
					Name:    sliceKey,
					Address: newAddress,
					Scheme:  scheme,
					Static:  false,
				}
				r.targets[newAddress] = fullTarget

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
		select {
		case r.Events <- evt:
		default:
			slog.Warn("Registry events channel full, dropping event to prevent deadlock", "target", evt.Target.Address)
		}
	}
}

// RemoveEndpointSlice completely removes all targets associated with a given EndpointSlice.
func (r *Registry) RemoveEndpointSlice(slice *discoveryv1.EndpointSlice, scheme, path string) {
	var eventsToSend []TargetEvent

	func() {
		r.mu.Lock()
		defer r.mu.Unlock()

		sliceKey := slice.Namespace + "/" + slice.Name
		oldTargets := r.sliceTargets[sliceKey]

		for _, oldAddress := range oldTargets {
			delete(r.targets, oldAddress)
			delete(r.active, oldAddress)
			eventsToSend = append(eventsToSend, TargetEvent{
				Target:  Target{Address: oldAddress},
				IsAdded: false,
			})
		}

		delete(r.sliceTargets, sliceKey)
	}()

	for _, evt := range eventsToSend {
		select {
		case r.Events <- evt:
		default:
			slog.Warn("Registry events channel full, dropping event to prevent deadlock", "target", evt.Target.Address)
		}
	}
}

// Add manually registers a single static target.
func (r *Registry) Add(target Target) {
	var eventToSend *TargetEvent

	func() {
		r.mu.Lock()
		defer r.mu.Unlock()

		if _, exists := r.targets[target.Address]; !exists {
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
		select {
		case r.Events <- *eventToSend:
		default:
			slog.Warn("Registry events channel full, dropping or delayed event", "target", target.Address)
		}
	}
}

// Remove deletes a target by address and emits a removal event
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
		select {
		case r.Events <- *eventToSend:
		default:
			slog.Warn("Registry events channel full, dropping or delayed event", "target", address)
		}
	}
}

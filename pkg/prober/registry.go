package prober

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	discoveryv1 "k8s.io/api/discovery/v1"
)

// Registry manages the in-memory state of all discovered targets (both dynamic and static).
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
		Events:       make(chan TargetEvent, 1000),
	}
}

func (r *Registry) shouldProcessTargetLocked(target string) bool {
	return evalShardingOwner(target, r.selfPodIP, r.clusterPodIPs)
}

// ShouldProcessTarget provides thread-safe external access to evaluate target ownership.
func (r *Registry) ShouldProcessTarget(targetAddress string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.shouldProcessTargetLocked(targetAddress)
}

func (r *Registry) emitEvent(evt TargetEvent) {
	if !evt.IsAdded {
		select {
		case r.Events <- evt:
		case <-time.After(2 * time.Second):
			slog.Error("Critical: Failed to deliver target removal event within timeout", "target", evt.Target.Address)
		}
		return
	}

	select {
	case r.Events <- evt:
	case <-time.After(100 * time.Millisecond):
		slog.Warn("Registry events channel full, skipping transient add event", "target", evt.Target.Address)
	}
}

// UpdatePeers synchronizes the active prober replica topology and triggers target rebalancing.
func (r *Registry) UpdatePeers(peers []string) {
	var eventsToSend []TargetEvent

	func() {
		r.mu.Lock()
		defer r.mu.Unlock()

		r.clusterPodIPs = peers

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
		r.emitEvent(evt)
	}
}

// UpdateFromEndpointSlice reconciles endpoints discovered dynamically from Kubernetes EndpointSlices.
func (r *Registry) UpdateFromEndpointSlice(slice *discoveryv1.EndpointSlice, scheme, path string) {
	var eventsToSend []TargetEvent

	func() {
		r.mu.Lock()
		defer r.mu.Unlock()

		sliceKey := slice.Namespace + "/" + slice.Name
		newTargetMap := make(map[string]bool)
		var newTargetList []string

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

		// Clean up disappearing dynamic endpoints
		oldTargets := r.sliceTargets[sliceKey]
		for _, oldAddress := range oldTargets {
			if !newTargetMap[oldAddress] {
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

		// Register new endpoints
		for _, newAddress := range newTargetList {
			if existing, exists := r.targets[newAddress]; exists && existing.Static {
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
func (r *Registry) Add(target Target) {
	var eventToSend *TargetEvent

	func() {
		r.mu.Lock()
		defer r.mu.Unlock()

		target.Static = true
		existing, exists := r.targets[target.Address]

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

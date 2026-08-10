package prober

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"sync"

	discoveryv1 "k8s.io/api/discovery/v1"
)

// TargetEvent represents a change in the discovered targets.
type TargetEvent struct {
	Target  string
	IsAdded bool
}

type Registry struct {
	mu            sync.RWMutex
	targets       map[string]string
	sliceTargets  map[string][]string
	Events        chan TargetEvent
	selfPodIP     string
	clusterPodIPs []string
}

func NewRegistry() *Registry {
	return &Registry{
		targets:       make(map[string]string),
		sliceTargets:  make(map[string][]string),
		Events:        make(chan TargetEvent, 1000),
		clusterPodIPs: make([]string, 0),
	}
}

// SetSelfIP initializes the pod's own identity used for hash comparisons.
func (r *Registry) SetSelfIP(ip string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.selfPodIP = ip
}

// UpdatePeers updates the active replica topology and triggers a rebalance.
func (r *Registry) UpdatePeers(peers []string) {
	r.mu.Lock()

	// Sort IPs to guarantee an identical and deterministic hash topology across all replicas
	sortedPeers := make([]string, len(peers))
	copy(sortedPeers, peers)
	sort.Strings(sortedPeers)

	r.clusterPodIPs = sortedPeers

	// Collect events while holding the lock
	eventsToSend := r.rebalanceTargetsLocked()

	r.mu.Unlock() // 🔴 ALWAYS Unlock BEFORE writing to channels!

	// Emit events asynchronously to prevent deadlocks
	go r.emitEvents(eventsToSend)
}

// ShouldProcessTarget uses Rendezvous Hashing (Highest Random Weight)
// to determine if this specific pod is responsible for probing the target.
func (r *Registry) ShouldProcessTarget(target string) bool {
	if r.selfPodIP == "" || len(r.clusterPodIPs) == 0 {
		return true
	}

	var highestHash uint64
	var selectedPod string

	for _, podIP := range r.clusterPodIPs {
		h := hashTargetAndPod(target, podIP)
		if h > highestHash {
			highestHash = h
			selectedPod = podIP
		}
	}

	return selectedPod == r.selfPodIP
}

func hashTargetAndPod(target, podIP string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(target + ":" + podIP))
	return h.Sum64()
}

// rebalanceTargetsLocked re-evaluates all known targets against the new topology.
// It returns a list of events to be emitted AFTER the lock is released.
func (r *Registry) rebalanceTargetsLocked() []TargetEvent {
	var events []TargetEvent
	for targetURL := range r.targets {
		shouldProcess := r.ShouldProcessTarget(targetURL)
		events = append(events, TargetEvent{Target: targetURL, IsAdded: shouldProcess})
	}
	return events
}

// UpdateFromEndpointSlice adds newly discovered targets from Kubernetes EndpointSlices.
func (r *Registry) UpdateFromEndpointSlice(slice *discoveryv1.EndpointSlice, scheme, path string) {
	r.mu.Lock()

	sliceKey := slice.Namespace + "/" + slice.Name

	if scheme == "" {
		scheme = "http"
	}
	if path != "" && !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	portVal := int32(80)
	if len(slice.Ports) > 0 && slice.Ports[0].Port != nil {
		portVal = *slice.Ports[0].Port
	}

	newTargets := make(map[string]bool)
	var newTargetList []string

	for _, ep := range slice.Endpoints {
		for _, addr := range ep.Addresses {
			targetURL := fmt.Sprintf("%s://%s:%d%s", scheme, addr, portVal, path)
			newTargets[targetURL] = true
			newTargetList = append(newTargetList, targetURL)
		}
	}

	var eventsToSend []TargetEvent

	// 2. Remove old targets
	oldTargets := r.sliceTargets[sliceKey]
	for _, oldTarget := range oldTargets {
		if !newTargets[oldTarget] {
			delete(r.targets, oldTarget)
			eventsToSend = append(eventsToSend, TargetEvent{Target: oldTarget, IsAdded: false})
		}
	}

	// 3. Add new targets
	for _, newTarget := range newTargetList {
		if _, exists := r.targets[newTarget]; !exists {
			r.targets[newTarget] = slice.Namespace
			if r.ShouldProcessTarget(newTarget) {
				eventsToSend = append(eventsToSend, TargetEvent{Target: newTarget, IsAdded: true})
			}
		}
	}

	// 4. Update the mapping
	if len(newTargetList) > 0 {
		r.sliceTargets[sliceKey] = newTargetList
	} else {
		delete(r.sliceTargets, sliceKey)
	}

	r.mu.Unlock() // 🔴 Unlock BEFORE writing to channels!

	go r.emitEvents(eventsToSend)
}

// RemoveEndpointSlice removes deleted targets and stops their local schedulers.
func (r *Registry) RemoveEndpointSlice(slice *discoveryv1.EndpointSlice, scheme, path string) {
	r.mu.Lock()

	sliceKey := slice.Namespace + "/" + slice.Name
	oldTargets := r.sliceTargets[sliceKey]

	var eventsToSend []TargetEvent

	for _, targetURL := range oldTargets {
		if _, exists := r.targets[targetURL]; exists {
			delete(r.targets, targetURL)
			eventsToSend = append(eventsToSend, TargetEvent{Target: targetURL, IsAdded: false})
		}
	}

	delete(r.sliceTargets, sliceKey)

	r.mu.Unlock() // 🔴 Unlock BEFORE writing to channels!

	go r.emitEvents(eventsToSend)
}

// Helper to send events asynchronously
func (r *Registry) emitEvents(events []TargetEvent) {
	for _, evt := range events {
		r.Events <- evt
	}
}

// GetTargets returns all targets currently owned by this specific pod instance.
func (r *Registry) GetTargets() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	targets := make([]string, 0, len(r.targets))
	for t := range r.targets {
		if r.ShouldProcessTarget(t) {
			targets = append(targets, t)
		}
	}
	return targets
}

// GetPeers returns a copy of the current sorted peer IPs for testing or debugging
func (r *Registry) GetPeers() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	peersCopy := make([]string, len(r.clusterPodIPs))
	copy(peersCopy, r.clusterPodIPs)

	return peersCopy
}

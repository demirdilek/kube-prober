package prober

import (
	"fmt"
	"sync"
	"testing"
	"time"

	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Helper to safely drain expected events from the channel with a timeout
func drainEvents(ch <-chan TargetEvent, expectedCount int) []TargetEvent {
	var events []TargetEvent
	timeout := time.After(1 * time.Second)
	for i := 0; i < expectedCount; i++ {
		select {
		case evt := <-ch:
			events = append(events, evt)
		case <-timeout:
			return events
		}
	}
	return events
}

func TestRegistry_AddStaticTarget(t *testing.T) {
	r := NewRegistry("10.0.0.1")

	target := Target{
		Name:    "google-dns",
		Address: "8.8.8.8",
		Scheme:  "dns",
		Static:  true,
	}

	r.Add(target)

	events := drainEvents(r.Events, 1)
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}
	if !events[0].IsAdded || events[0].Target.Address != "8.8.8.8" {
		t.Errorf("Expected IsAdded=true for 8.8.8.8, got %v", events[0])
	}
}

func TestRegistry_UpdateFromEndpointSlice_And_GhostEvents(t *testing.T) {
	selfIP := "10.0.0.1"
	r := NewRegistry(selfIP)
	r.UpdatePeers([]string{selfIP})

	port := int32(8080)
	slice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-slice",
		},
		Ports: []discoveryv1.EndpointPort{
			{Port: &port},
		},
		Endpoints: []discoveryv1.Endpoint{
			{
				Addresses: []string{"192.168.1.100"},
				Conditions: discoveryv1.EndpointConditions{
					Ready: boolPtr(true),
				},
			},
		},
	}

	// 1. Add targets via EndpointSlice
	r.UpdateFromEndpointSlice(slice, "http", "/healthz")

	events := drainEvents(r.Events, 1)
	if len(events) != 1 {
		t.Fatalf("Expected 1 add event, got %d", len(events))
	}
	expectedAddress := "http://192.168.1.100:8080/healthz"
	if events[0].Target.Address != expectedAddress || !events[0].IsAdded {
		t.Errorf("Unexpected event: %+v", events[0])
	}

	// 2. Simulate Endpoint removal (scaling down the monitored app)
	slice.Endpoints = []discoveryv1.Endpoint{} // Empty endpoints
	r.UpdateFromEndpointSlice(slice, "http", "/healthz")

	removeEvents := drainEvents(r.Events, 1)
	if len(removeEvents) != 1 {
		t.Fatalf("Expected 1 remove event, got %d", len(removeEvents))
	}
	if removeEvents[0].IsAdded {
		t.Errorf("Expected IsAdded=false for target removal")
	}

	// 3. Test Ghost Events: Remove the slice completely now
	// Since the target is already inactive, this should NOT emit any ghost events.
	r.RemoveEndpointSlice(slice, "http", "/healthz")
	ghostEvents := drainEvents(r.Events, 1)
	if len(ghostEvents) != 0 {
		t.Errorf("Expected 0 ghost events, got %d", len(ghostEvents))
	}
}

func TestRegistry_ShardingRendezvousHashing(t *testing.T) {
	// Use explicit IPs to test hash distribution
	pod1 := "10.0.0.1"
	pod2 := "10.0.0.2"

	r1 := NewRegistry(pod1)
	r2 := NewRegistry(pod2)

	peers := []string{pod1, pod2}
	r1.UpdatePeers(peers)
	r2.UpdatePeers(peers)

	// Ensure the null-byte hash distribution is stable and doesn't orphan targets
	targetA := "http://app-1:80/healthz"
	targetB := "http://app-2:80/healthz"

	// Both registries should agree on who owns what without coordination
	r1OwnsA := r1.ShouldProcessTarget(targetA)
	r2OwnsA := r2.ShouldProcessTarget(targetA)

	if r1OwnsA == r2OwnsA {
		t.Errorf("Split brain detected! Both or neither pods claim ownership of targetA: r1=%v, r2=%v", r1OwnsA, r2OwnsA)
	}

	r1OwnsB := r1.ShouldProcessTarget(targetB)
	r2OwnsB := r2.ShouldProcessTarget(targetB)

	if r1OwnsB == r2OwnsB {
		t.Errorf("Split brain detected! Both or neither pods claim ownership of targetB: r1=%v, r2=%v", r1OwnsB, r2OwnsB)
	}
}

func TestRegistry_UpdatePeers_Rebalancing(t *testing.T) {
	selfIP := "10.0.0.1"
	r := NewRegistry(selfIP)

	// Start with only self
	r.UpdatePeers([]string{selfIP})

	// Add dynamic targets
	for i := 0; i < 10; i++ {
		r.Add(Target{
			Name:    fmt.Sprintf("dyn-%d", i),
			Address: fmt.Sprintf("http://192.168.1.%d", i),
			Static:  false,
		})
	}

	// Drain the initial 10 add events
	initialEvents := drainEvents(r.Events, 10)
	if len(initialEvents) != 10 {
		t.Fatalf("Expected 10 initial add events, got %d", len(initialEvents))
	}

	// Simulate HPA scaling up: Add 3 new peers
	newPeers := []string{selfIP, "10.0.0.2", "10.0.0.3", "10.0.0.4"}
	r.UpdatePeers(newPeers)

	// Draining events to see what ownership we lost
	// We expect to lose roughly ~75% of the targets (though random due to hashing)
	rebalanceEvents := drainEvents(r.Events, 10)

	if len(rebalanceEvents) == 0 {
		t.Errorf("Expected rebalancing events after peer update, got 0")
	}

	for _, evt := range rebalanceEvents {
		if evt.IsAdded {
			t.Errorf("Did not expect any IsAdded=true events during scale-up (we only lose targets), got: %+v", evt)
		}
	}
}

func TestRegistry_ConcurrencySafety(t *testing.T) {
	r := NewRegistry("10.0.0.1")
	r.UpdatePeers([]string{"10.0.0.1", "10.0.0.2"})

	var wg sync.WaitGroup
	workers := 50

	// Run multiple operations concurrently to trigger the race detector
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			// 1. Concurrent Reads
			_ = r.ShouldProcessTarget(fmt.Sprintf("http://test-%d", idx))

			// 2. Concurrent Add
			r.Add(Target{
				Name:    fmt.Sprintf("static-%d", idx),
				Address: fmt.Sprintf("192.168.1.%d", idx),
				Static:  true,
			})

			// 3. Concurrent Peer Updates
			r.UpdatePeers([]string{"10.0.0.1", "10.0.0.2", "10.0.0.3"})
		}(i)
	}

	// Drain the channel continuously so we don't block the workers
	go func() {
		for {
			select {
			case <-r.Events:
			case <-time.After(2 * time.Second):
				return
			}
		}
	}()

	wg.Wait()
	// If the test reaches here without panicking or triggering `-race`, the mutex handling is solid.
}

func TestRegistry_StaticTargetPrecedence(t *testing.T) {
	reg := NewRegistry("10.0.0.1")

	staticTarget := Target{
		Address: "tls://kubernetes.default.svc.cluster.local:443",
		Scheme:  "tls",
		Static:  true,
	}
	reg.Add(staticTarget)

	// Consume initial event
	<-reg.Events

	// Simulate discovery of the same endpoint via EndpointSlice
	port := int32(443)
	ready := true
	slice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{Name: "kubernetes", Namespace: "default"},
		Ports:      []discoveryv1.EndpointPort{{Port: &port}},
		Endpoints: []discoveryv1.Endpoint{
			{
				Addresses:  []string{"kubernetes.default.svc.cluster.local"},
				Conditions: discoveryv1.EndpointConditions{Ready: &ready},
			},
		},
	}

	reg.UpdateFromEndpointSlice(slice, "tls", "")

	// Assert no duplicate event was queued
	select {
	case evt := <-reg.Events:
		t.Fatalf("unexpected event for duplicate static target: %+v", evt)
	default:
		// Expected: duplicate ignored
	}
}

func TestRegistry_DynamicOverwrittenByStatic(t *testing.T) {
	reg := NewRegistry("10.0.0.1")

	// 1. Add dynamic target first
	dynTarget := Target{
		Address: "http://192.168.1.50:80",
		Scheme:  "http",
		Static:  false,
	}
	// Simulate adding via internal map or mock slice update
	reg.targets[dynTarget.Address] = dynTarget

	// 2. Now add static CRD with same address
	staticTarget := Target{
		Address: "http://192.168.1.50:80",
		Scheme:  "http",
		Static:  true,
	}
	reg.Add(staticTarget)

	// 3. Verify it is now marked as static in the registry
	reg.mu.RLock()
	stored, exists := reg.targets[staticTarget.Address]
	reg.mu.RUnlock()

	if !exists || !stored.Static {
		t.Errorf("Expected dynamic target to be overwritten and marked as static, got: %+v", stored)
	}
}

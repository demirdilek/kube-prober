package prober

import (
	"testing"
	"time"

	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestRegistry_StaticTargetPrecedence(t *testing.T) {
	selfIP := "10.244.0.1"
	peers := []string{selfIP}
	r := NewRegistry(selfIP)
	r.UpdatePeers(peers)

	// Drain peer-update event
	select {
	case <-r.Events:
	case <-time.After(100 * time.Millisecond):
	}

	targetAddr := "http://10.244.2.5:8080/healthz"

	// 1. Add static target via CRD
	r.Add(Target{
		Name:    "static-svc",
		Address: targetAddr,
		Scheme:  "http",
	})

	select {
	case evt := <-r.Events:
		if !evt.IsAdded || evt.Target.Address != targetAddr || !evt.Target.Static {
			t.Fatalf("unexpected event for static add: %+v", evt)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for static target event")
	}

	// 2. Discover same endpoint dynamically via EndpointSlice
	slice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dynamic-slice",
			Namespace: "default",
		},
		Ports: []discoveryv1.EndpointPort{
			{Port: ptr.To(int32(8080))},
		},
		Endpoints: []discoveryv1.Endpoint{
			{
				Addresses:  []string{"10.244.2.5"},
				Conditions: discoveryv1.EndpointConditions{Ready: ptr.To(true)},
			},
		},
	}

	r.UpdateFromEndpointSlice(slice, "http", "/healthz")

	// Verify static target remained untouched and no duplicate event was triggered
	r.mu.RLock()
	target, exists := r.targets[targetAddr]
	r.mu.RUnlock()

	if !exists || !target.Static {
		t.Fatalf("expected static target to retain precedence over EndpointSlice")
	}

	select {
	case evt := <-r.Events:
		t.Fatalf("unexpected duplicate event received: %+v", evt)
	case <-time.After(100 * time.Millisecond):
		// Expected: channel remains empty
	}
}

func TestRegistry_UpdatePeers_Rebalancing(t *testing.T) {
	selfIP := "10.244.0.1"
	peerIP := "10.244.0.2"

	r := NewRegistry(selfIP)
	r.UpdatePeers([]string{selfIP})

	targetAddr := "http://10.244.5.10:8080/metrics"
	r.Add(Target{
		Name:    "demo-target",
		Address: targetAddr,
		Scheme:  "http",
	})

	// Drain initial add event
	select {
	case evt := <-r.Events:
		if !evt.IsAdded {
			t.Fatalf("expected add event, got delete")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for add event")
	}

	// Scale prober cluster by adding a second peer
	r.UpdatePeers([]string{selfIP, peerIP})

	// If ownership transferred away from selfIP, verify deletion event is queued
	r.mu.RLock()
	isActive := r.active[targetAddr]
	isOwner := r.shouldProcessTargetLocked(targetAddr)
	r.mu.RUnlock()

	if !isOwner {
		if isActive {
			t.Fatalf("expected active map to reflect loss of ownership")
		}
		select {
		case evt := <-r.Events:
			if evt.IsAdded || evt.Target.Address != targetAddr {
				t.Fatalf("expected removal event on rebalance, got %+v", evt)
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatal("timed out waiting for rebalance delete event")
		}
	}
}

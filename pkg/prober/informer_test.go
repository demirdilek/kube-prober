package prober

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// boolPtr is a helper to get a pointer to a boolean
func boolPtr(b bool) *bool {
	return &b
}

// int32Ptr is a helper to get a pointer to an int32
func int32Ptr(i int32) *int32 {
	return &i
}

// TestKubeWatcher_InformerEvents_DynamicPath tests if the informer correctly
// reads custom schemes and paths from the Service annotations.
func TestKubeWatcher_InformerEvents_DynamicPath(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. Create a fake clientset and pre-load the Service so it's in the cache
	testService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "default",
			Annotations: map[string]string{
				"probe/scheme": "tcp",
				"probe/path":   "", // TCP doesn't need a path
			},
		},
	}
	clientset := fake.NewSimpleClientset(testService)

	// 2. Initialize Registry and Watcher
	registry := NewRegistry("10.0.0.1")
	registry.UpdatePeers([]string{"10.0.0.1"}) // Make this pod the owner
	watcher := NewKubeWatcher(clientset, registry)

	// 3. Start the watcher and wait for caches to sync
	err := watcher.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}

	// 4. Create the EndpointSlice dynamically
	slice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-slice",
			Namespace: "default",
			Labels: map[string]string{
				"kubernetes.io/service-name": testService.Name,
				"probe":                      "true",
			},
		},
		Ports: []discoveryv1.EndpointPort{
			{Port: int32Ptr(6379)},
		},
		Endpoints: []discoveryv1.Endpoint{
			{
				Addresses: []string{"10.244.0.15"},
				Conditions: discoveryv1.EndpointConditions{
					Ready: boolPtr(true),
				},
			},
		},
	}

	_, err = clientset.DiscoveryV1().EndpointSlices("default").Create(ctx, slice, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create endpoint slice: %v", err)
	}

	// 5. Verify the target was added with the correct dynamic scheme and path
	select {
	case evt := <-registry.Events:
		if !evt.IsAdded {
			t.Errorf("Expected IsAdded=true, got false")
		}
		expectedAddress := "tcp://10.244.0.15:6379"
		if evt.Target.Address != expectedAddress {
			t.Errorf("Expected address %q, got %q", expectedAddress, evt.Target.Address)
		}
		if evt.Target.Scheme != "tcp" {
			t.Errorf("Expected scheme 'tcp', got %q", evt.Target.Scheme)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for target to be added to registry")
	}

	// 6. Test removal by stripping the "probe" label
	slice.Labels["probe"] = "false"
	_, err = clientset.DiscoveryV1().EndpointSlices("default").Update(ctx, slice, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("failed to update endpoint slice: %v", err)
	}

	select {
	case evt := <-registry.Events:
		if evt.IsAdded {
			t.Errorf("Expected IsAdded=false for target removal, got true")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for target to be removed from registry")
	}
}

// TestKubeWatcher_WatchPeers tests if the watcher successfully discovers
// and registers the IP addresses of the prober pods themselves.
func TestKubeWatcher_WatchPeers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clientset := fake.NewSimpleClientset()
	registry := NewRegistry("10.0.0.1")
	watcher := NewKubeWatcher(clientset, registry)

	// Pre-add a mock dynamic target to see if peer updates trigger a rebalance event
	registry.targets["http://app:80"] = Target{Address: "http://app:80", Static: false}
	registry.active["http://app:80"] = true

	// Create an EndpointSlice representing the kube-prober pods
	proberSlice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kube-prober-slice",
			Namespace: "kube-system",
			Labels: map[string]string{
				"kubernetes.io/service-name": "kube-prober",
			},
		},
		Endpoints: []discoveryv1.Endpoint{
			{
				Addresses: []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"},
				Conditions: discoveryv1.EndpointConditions{
					Ready: boolPtr(true),
				},
			},
		},
	}

	_, err := clientset.DiscoveryV1().EndpointSlices("kube-system").Create(ctx, proberSlice, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create prober endpoint slice: %v", err)
	}

	// Start watching peers
	go watcher.WatchPeers(ctx)

	// Wait for the rebalance event triggered by the peer update
	// Since we added 3 peers and 1 target, ownership will shift. We expect an event.
	select {
	case evt := <-registry.Events:
		// We expect either an IsAdded=true or false depending on the hash,
		// but receiving ANY event proves the WatchPeers callback fired UpdatePeers.
		if evt.Target.Address != "http://app:80" {
			t.Errorf("Expected event for http://app:80, got %q", evt.Target.Address)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for WatchPeers to trigger an UpdatePeers rebalance event")
	}
}

package prober

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
)

// TestWatchStaticTargets verifies that the dynamic informer correctly parses,
// adds, and removes StaticTarget CRD objects from the registry.
func TestWatchStaticTargets(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. Define test CRD object
	targetObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kube-prober.io/v1alpha1",
			"kind":       "StaticTarget",
			"metadata": map[string]interface{}{
				"name":      "google-public-dns",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"address": "8.8.8.8",
				"scheme":  "dns",
			},
		},
	}

	scheme := runtime.NewScheme()

	// Map GVR to the expected List Kind
	gvrToListKind := map[schema.GroupVersionResource]string{
		{
			Group:    "kube-prober.io",
			Version:  "v1alpha1",
			Resource: "statictargets",
		}: "StaticTargetList",
	}

	// 2. Initialize fake dynamic client and registry
	fakeClient := fake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind)
	registry := NewRegistry("10.0.0.1")

	// 3. Start watching in the background
	go func() {
		_ = WatchStaticTargets(ctx, fakeClient, registry)
	}()

	// Wait briefly for informer factory startup
	time.Sleep(50 * time.Millisecond)

	// 4. Create the CRD object via the fake dynamic client
	_, err := fakeClient.Resource(StaticTargetGVR).Namespace("default").Create(ctx, targetObj, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create StaticTarget: %v", err)
	}

	// 5. Verify the target was added to the registry and emitted on Events channel
	select {
	case evt := <-registry.Events:
		if !evt.IsAdded {
			t.Errorf("Expected IsAdded=true, got false")
		}
		if evt.Target.Name != "google-public-dns" {
			t.Errorf("Expected name 'google-public-dns', got %q", evt.Target.Name)
		}
		if evt.Target.Address != "8.8.8.8" {
			t.Errorf("Expected address '8.8.8.8', got %q", evt.Target.Address)
		}
		if evt.Target.Scheme != "dns" {
			t.Errorf("Expected scheme 'dns', got %q", evt.Target.Scheme)
		}
		if !evt.Target.Static {
			t.Errorf("Expected target to have Static=true")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for StaticTarget Add event")
	}

	// 6. Delete the CRD object
	err = fakeClient.Resource(StaticTargetGVR).Namespace("default").Delete(ctx, "google-public-dns", metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("failed to delete StaticTarget: %v", err)
	}

	// 7. Verify the target removal event
	select {
	case evt := <-registry.Events:
		if evt.IsAdded {
			t.Errorf("Expected IsAdded=false on deletion, got true")
		}
		if evt.Target.Address != "8.8.8.8" {
			t.Errorf("Expected removed address '8.8.8.8', got %q", evt.Target.Address)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for StaticTarget Delete event")
	}
}

// TestParseStaticTarget verifies the parsing helper logic and error branches.
func TestParseStaticTarget(t *testing.T) {
	tests := []struct {
		name      string
		obj       *unstructured.Unstructured
		expectErr bool
	}{
		{
			name: "Valid CRD spec",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{"name": "valid-target"},
					"spec": map[string]interface{}{
						"address": "1.1.1.1",
						"scheme":  "dns",
					},
				},
			},
			expectErr: false,
		},
		{
			name: "Missing spec block",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{"name": "missing-spec"},
				},
			},
			expectErr: true,
		},
		{
			name: "Missing address field",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{"name": "no-address"},
					"spec": map[string]interface{}{
						"scheme": "http",
					},
				},
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, err := parseStaticTarget(tt.obj)
			if tt.expectErr && err == nil {
				t.Errorf("expected error parsing target, got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("expected successful parse, got: %v", err)
			}
			if !tt.expectErr && target.Name != tt.obj.GetName() {
				t.Errorf("expected target name %q, got %q", tt.obj.GetName(), target.Name)
			}
		})
	}
}

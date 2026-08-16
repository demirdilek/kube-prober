package prober

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
)

// StaticTargetGVR defines the GroupVersionResource for StaticTarget CRD
var StaticTargetGVR = schema.GroupVersionResource{
	Group:    "kube-prober.io",
	Version:  "v1alpha1",
	Resource: "statictargets",
}

// WatchStaticTargets starts an informer for StaticTarget CRDs across all namespaces
func WatchStaticTargets(ctx context.Context, dynClient dynamic.Interface, registry *Registry) error {
	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(
		dynClient,
		10*time.Minute,
		metav1.NamespaceAll,
		nil,
	)

	informer := factory.ForResource(StaticTargetGVR).Informer()

	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			u, ok := obj.(*unstructured.Unstructured)
			if !ok {
				return
			}
			target, err := parseStaticTarget(u)
			if err != nil {
				slog.Error("Failed to parse StaticTarget CRD", "error", err, "name", u.GetName())
				return
			}
			registry.Add(target)
			slog.Info("Loaded static target from CRD", "name", target.Name, "address", target.Address, "scheme", target.Scheme)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			u, ok := newObj.(*unstructured.Unstructured)
			if !ok {
				return
			}
			target, err := parseStaticTarget(u)
			if err != nil {
				slog.Error("Failed to parse updated StaticTarget CRD", "error", err, "name", u.GetName())
				return
			}
			registry.Add(target)
		},
		DeleteFunc: func(obj interface{}) {
			u, ok := obj.(*unstructured.Unstructured)
			if !ok {
				tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
				if !ok {
					return
				}
				u, ok = tombstone.Obj.(*unstructured.Unstructured)
				if !ok {
					return
				}
			}
			target, err := parseStaticTarget(u)
			if err != nil {
				return
			}
			// Trigger remove event for static target
			registry.Remove(target.Address)
			slog.Info("Removed static target from CRD", "name", target.Name, "address", target.Address)
		},
	})
	if err != nil {
		return fmt.Errorf("failed to register StaticTarget event handlers: %w", err)
	}

	factory.Start(ctx.Done())

	if !cache.WaitForCacheSync(ctx.Done(), informer.HasSynced) {
		return fmt.Errorf("failed to sync StaticTarget informer cache: %w", ctx.Err())
	}

	return nil
}

func parseStaticTarget(u *unstructured.Unstructured) (Target, error) {
	spec, ok := u.Object["spec"].(map[string]interface{})
	if !ok {
		return Target{}, fmt.Errorf("spec field missing or invalid")
	}

	address, _ := spec["address"].(string)
	scheme, _ := spec["scheme"].(string)

	if address == "" || scheme == "" {
		return Target{}, fmt.Errorf("address or scheme missing in spec")
	}

	// Ensure uniform scheme prefix across metrics and dashboard queries
	if !strings.Contains(address, "://") {
		address = fmt.Sprintf("%s://%s", scheme, address)
	}

	return Target{
		Name:    u.GetName(),
		Address: address,
		Scheme:  scheme,
		Static:  true,
	}, nil
}

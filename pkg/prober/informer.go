package prober

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

// KubeWatcher observes K8s resources (EndpointSlices) to keep the target
// registry in sync and maintain the active replica topology for sharding.
type KubeWatcher struct {
	clientset kubernetes.Interface
	registry  *Registry
}

func NewKubeWatcher(clientset kubernetes.Interface, reg *Registry) *KubeWatcher {
	return &KubeWatcher{
		clientset: clientset,
		registry:  reg,
	}
}

// getProbeSchemeAndPath dynamically extracts both the protocol scheme (e.g., http, tcp)
// and the HTTP path from the Kubernetes Service annotations using a local RAM cache.
func (w *KubeWatcher) getProbeSchemeAndPath(slice *discoveryv1.EndpointSlice, svcLister corev1listers.ServiceLister) (string, string) {
	svcName := slice.Labels["kubernetes.io/service-name"]

	// Default to HTTP and /healthz if no specific annotations are found.
	scheme := "http"
	path := "/healthz"

	if svcName == "" {
		slog.Warn("EndpointSlice missing service-name label, falling back to defaults", "slice", slice.Name)
		return scheme, path
	}

	// FAST: Read directly from the local RAM cache instead of making a live API call!
	svc, err := svcLister.Services(slice.Namespace).Get(svcName)
	if err != nil || svc.Annotations == nil {
		slog.Warn("Failed to fetch Service from cache, falling back to defaults", "service", svcName, "error", err)
		return scheme, path
	}

	// 1. Check for a custom protocol scheme (e.g., "probe/scheme: tcp").
	// This allows the Dispatcher to route the probe to the correct protocol handler.
	if s, exists := svc.Annotations["probe/scheme"]; exists && s != "" {
		slog.Debug("Found custom probe scheme", "service", svcName, "scheme", s)
		scheme = s
	}

	// 2. Check for a custom path.
	// Note: We check if it exists, not if it's empty, because for protocols
	// like raw TCP, we actively want an empty path ("").
	if p, exists := svc.Annotations["probe/path"]; exists {
		path = p
	}

	return scheme, path
}

func (w *KubeWatcher) Start(ctx context.Context) error {
	factory := informers.NewSharedInformerFactoryWithOptions(
		w.clientset,
		10*time.Minute,
	)

	endpointSliceInformer := factory.Discovery().V1().EndpointSlices().Informer()

	// Instantiate Service Informer and Lister to enable local RAM caching
	serviceInformer := factory.Core().V1().Services().Informer()
	serviceLister := factory.Core().V1().Services().Lister()

	_, err := endpointSliceInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if slice, ok := obj.(*discoveryv1.EndpointSlice); ok {
				if slice.Labels["probe"] == "true" {
					// Pass the serviceLister for fast annotation lookup
					scheme, path := w.getProbeSchemeAndPath(slice, serviceLister)
					w.registry.UpdateFromEndpointSlice(slice, scheme, path)
				}
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			if newSlice, ok := newObj.(*discoveryv1.EndpointSlice); ok {
				if newSlice.Labels["probe"] == "true" {
					// Pass the serviceLister for fast annotation lookup
					scheme, path := w.getProbeSchemeAndPath(newSlice, serviceLister)
					w.registry.UpdateFromEndpointSlice(newSlice, scheme, path)
				} else {
					// Target lost the "probe" label, safely remove it
					w.registry.RemoveEndpointSlice(newSlice, "", "")
				}
			}
		},
		DeleteFunc: func(obj interface{}) {
			// 1. Try to cast the object directly to an EndpointSlice
			slice, ok := obj.(*discoveryv1.EndpointSlice)

			// 2. If it's not a direct slice, check if it's a tombstone (missed deletion event)
			if !ok {
				tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
				if !ok {
					return // Not a slice and not a tombstone, safely ignore
				}

				// Extract the actual slice from the tombstone
				slice, ok = tombstone.Obj.(*discoveryv1.EndpointSlice)
				if !ok {
					return
				}
			}

			// 3. Now that we safely have the 'slice', we can read its labels and clean up
			if slice.Labels["probe"] == "true" {
				w.registry.RemoveEndpointSlice(slice, "", "")
			}
		},
	})
	if err != nil {
		return fmt.Errorf("failed to add event handler to EndpointSlice informer: %w", err)
	}

	factory.Start(ctx.Done())

	// Wait until BOTH caches (EndpointSlices AND Services) are fully synchronized
	if !cache.WaitForCacheSync(ctx.Done(), endpointSliceInformer.HasSynced, serviceInformer.HasSynced) {
		return fmt.Errorf("failed to sync informer caches: %w", ctx.Err())
	}

	return nil
}

// WatchPeers observes EndpointSlices of the prober deployment itself
// to keep the active replica topology synced for Rendezvous Hashing.
func (w *KubeWatcher) WatchPeers(ctx context.Context) {
	factory := informers.NewSharedInformerFactoryWithOptions(
		w.clientset,
		10*time.Minute,
	)

	informer := factory.Discovery().V1().EndpointSlices().Informer()

	updatePeers := func() {
		var peerIPs []string

		for _, obj := range informer.GetStore().List() {
			slice, ok := obj.(*discoveryv1.EndpointSlice)

			if ok && slice.Labels["kubernetes.io/service-name"] == "kube-prober" {
				for _, ep := range slice.Endpoints {
					if ep.Conditions.Ready != nil && *ep.Conditions.Ready {
						peerIPs = append(peerIPs, ep.Addresses...)
					}
				}
			}
		}

		if len(peerIPs) > 0 {
			w.registry.UpdatePeers(peerIPs)
		}
	}

	_, _ = informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { updatePeers() },
		UpdateFunc: func(oldObj, newObj interface{}) { updatePeers() },
		DeleteFunc: func(obj interface{}) { updatePeers() },
	})

	factory.Start(ctx.Done())
	cache.WaitForCacheSync(ctx.Done(), informer.HasSynced)

	updatePeers()
}

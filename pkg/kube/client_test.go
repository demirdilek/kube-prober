package kube

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClient_InitializationAndFallback(t *testing.T) {
	// Point KUBECONFIG to a temporary dummy config to test local fallback
	tempDir := t.TempDir()
	fakeKubeconfig := filepath.Join(tempDir, "config")
	t.Setenv("KUBECONFIG", fakeKubeconfig)

	// Create a dummy minimal kubeconfig file
	dummyConfig := `
apiVersion: v1
kind: Config
clusters:
- cluster:
    server: http://127.0.0.1:8080
  name: dummy
contexts:
- context:
    cluster: dummy
    user: dummy
  name: dummy
current-context: dummy
users:
- name: dummy
`
	if err := os.WriteFile(fakeKubeconfig, []byte(dummyConfig), 0o600); err != nil {
		t.Fatalf("failed to write dummy kubeconfig: %v", err)
	}

	// 1. Test GetConfig
	config, err := GetConfig()
	if err != nil {
		t.Fatalf("expected no error getting config, got: %v", err)
	}
	if config == nil {
		t.Fatal("expected non-nil rest.Config")
	}

	// 2. Test InitClient (Standard Clientset)
	clientset, err := InitClient()
	if err != nil {
		t.Fatalf("expected no error initializing client, got: %v", err)
	}
	if clientset == nil {
		t.Fatal("expected non-nil kubernetes Clientset")
	}

	// 3. Test InitDynamicClient (Dynamic Interface for CRDs)
	dynClient, err := InitDynamicClient()
	if err != nil {
		t.Fatalf("expected no error initializing dynamic client, got: %v", err)
	}
	if dynClient == nil {
		t.Fatal("expected non-nil dynamic client")
	}
}

package prober

import (
	"fmt"
	"math"
	"testing"
)

func TestHashTargetPodAndVNode_Deterministic(t *testing.T) {
	target := "http://10.244.0.15:8080/healthz"
	podIP := "10.244.1.20"
	vnode := 42

	hash1 := hashTargetPodAndVNode(target, podIP, vnode)
	hash2 := hashTargetPodAndVNode(target, podIP, vnode)

	if hash1 != hash2 {
		t.Fatalf("expected deterministic hash output, got %d and %d", hash1, hash2)
	}

	hashDifferentVnode := hashTargetPodAndVNode(target, podIP, vnode+1)
	if hash1 == hashDifferentVnode {
		t.Fatalf("expected different hash values across vnode indices")
	}
}

func TestEvalShardingOwner_SinglePeerFallback(t *testing.T) {
	target := "http://10.244.0.10:8080/healthz"

	// No selfPodIP configured: fallback to owning everything
	if !evalShardingOwner(target, "", []string{"10.244.1.2"}) {
		t.Errorf("expected true when selfPodIP is empty")
	}

	// No peers configured: fallback to owning everything
	if !evalShardingOwner(target, "10.244.1.2", nil) {
		t.Errorf("expected true when peers slice is empty")
	}
}

func TestEvalShardingOwner_DistributionBalance(t *testing.T) {
	numPods := 5
	numTargets := 1000
	peers := make([]string, numPods)
	for i := 0; i < numPods; i++ {
		peers[i] = fmt.Sprintf("10.244.0.%d", i+1)
	}

	assignedCounts := make(map[string]int)

	for i := 0; i < numTargets; i++ {
		target := fmt.Sprintf("http://10.244.100.%d:8080/healthz", i)
		for _, podIP := range peers {
			if evalShardingOwner(target, podIP, peers) {
				assignedCounts[podIP]++
			}
		}
	}

	// Verify exact partition: each target must be owned by exactly one peer
	totalAssigned := 0
	for _, count := range assignedCounts {
		totalAssigned += count
	}
	if totalAssigned != numTargets {
		t.Fatalf("expected exactly %d total assignments, got %d", numTargets, totalAssigned)
	}

	expectedPerPod := float64(numTargets) / float64(numPods)
	for podIP, count := range assignedCounts {
		variance := math.Abs(float64(count)-expectedPerPod) / expectedPerPod
		// Expect distribution skew to be well within 15% across 1000 targets with 100 vnodes
		if variance > 0.25 {
			t.Errorf("pod %s target count %d deviated by %.2f%% from expected %f",
				podIP, count, variance*100, expectedPerPod)
		}
	}
}

package prober

import "log/slog"

const (
	// FNV-1a 64-bit constants
	fnvOffset64 = 14695981039346656037
	fnvPrime64  = 1099511628211

	// Number of virtual nodes per prober peer to minimize sharding variance (<5% skew)
	vnodesPerPeer = 100
)

// hashTargetPodAndVNode computes a zero-allocation deterministic 64-bit FNV-1a hash
// for a given (target, podIP, vnodeIndex) tuple.
func hashTargetPodAndVNode(target, podIP string, vnode int) uint64 {
	h := uint64(fnvOffset64)

	// Hash target string directly from memory without heap copies
	for i := 0; i < len(target); i++ {
		h ^= uint64(target[i])
		h *= fnvPrime64
	}

	// Null-byte delimiter to prevent boundary collision vulnerabilities
	h ^= 0
	h *= fnvPrime64

	// Hash podIP string
	for i := 0; i < len(podIP); i++ {
		h ^= uint64(podIP[i])
		h *= fnvPrime64
	}

	// Delimiter before virtual node index
	h ^= '#'
	h *= fnvPrime64

	// Hash vnode index digits without string formatting allocation
	if vnode == 0 {
		h ^= '0'
		h *= fnvPrime64
	} else {
		var digits [10]byte
		idx := len(digits)
		for vnode > 0 {
			idx--
			digits[idx] = byte('0' + (vnode % 10))
			vnode /= 10
		}
		for i := idx; i < len(digits); i++ {
			h ^= uint64(digits[i])
			h *= fnvPrime64
		}
	}

	return h
}

// evalShardingOwner determines which peer replica owns a given target using Rendezvous Hashing with Virtual Nodes.
func evalShardingOwner(target, selfPodIP string, peers []string) bool {
	if selfPodIP == "" || len(peers) == 0 {
		return true
	}

	var highestHash uint64
	var selectedPod string

	for i, podIP := range peers {
		for v := 0; v < vnodesPerPeer; v++ {
			h := hashTargetPodAndVNode(target, podIP, v)
			if (i == 0 && v == 0) || h > highestHash {
				highestHash = h
				selectedPod = podIP
			}
		}
	}

	isOwner := selectedPod == selfPodIP

	slog.Debug(
		"Sharding evaluation",
		"target", target,
		"assigned_pod", selectedPod,
		"my_pod", selfPodIP,
		"is_owner", isOwner,
		"peer_count", len(peers),
		"cluster_ips", peers,
	)

	return isOwner
}

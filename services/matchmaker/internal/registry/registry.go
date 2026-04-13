// Package registry maintains an in-memory registry of active Game Nodes.
//
// Each Game Node registers via gRPC and sends periodic heartbeats. If a
// heartbeat is missed beyond the deadline, the node is marked unhealthy.
// The Matchmaker queries this registry to find the least-loaded healthy
// node for incoming match requests.
package registry

import (
	"log/slog"
	"sync"
	"time"
)

// NodeInfo holds the runtime state of a registered Game Node.
type NodeInfo struct {
	NodeID      string
	Address     string    // host:port for direct WebSocket connections
	Capacity    int32     // maximum concurrent players
	CurrentLoad int32     // current number of connected players
	Healthy     bool      // set to false when heartbeat times out
	Draining    bool      // if true, node is winding down — don't assign
	LastSeen    time.Time // timestamp of last heartbeat
}

// Registry is a concurrency-safe, in-memory store for Game Node metadata.
type Registry struct {
	mu               sync.RWMutex
	nodes            map[string]*NodeInfo
	heartbeatTimeout time.Duration
}

// New creates a Registry with the given heartbeat timeout. If a node has
// not sent a heartbeat within this duration, it will be marked unhealthy.
func New(heartbeatTimeout time.Duration) *Registry {
	return &Registry{
		nodes:            make(map[string]*NodeInfo),
		heartbeatTimeout: heartbeatTimeout,
	}
}

// Register adds or updates a Game Node in the registry.
// Returns true if this is a new registration, false if it's a re-registration.
func (r *Registry) Register(nodeID, address string, capacity int32) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, exists := r.nodes[nodeID]
	r.nodes[nodeID] = &NodeInfo{
		NodeID:      nodeID,
		Address:     address,
		Capacity:    capacity,
		CurrentLoad: 0,
		Healthy:     true,
		Draining:    false,
		LastSeen:    time.Now(),
	}

	if !exists {
		slog.Info("node registered", "node_id", nodeID, "address", address, "capacity", capacity)
	} else {
		slog.Info("node re-registered", "node_id", nodeID, "address", address)
	}
	return !exists
}

// Heartbeat updates the liveness timestamp and load for a node.
// Returns false if the node is not registered.
func (r *Registry) Heartbeat(nodeID string, currentLoad int32) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	node, ok := r.nodes[nodeID]
	if !ok {
		return false
	}
	node.LastSeen = time.Now()
	node.CurrentLoad = currentLoad
	node.Healthy = true
	return true
}

// BestNode returns the healthy, non-draining node with the most spare
// capacity (capacity - currentLoad). Returns nil if no suitable node exists.
func (r *Registry) BestNode() *NodeInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var best *NodeInfo
	var bestSpare int32 = -1

	for _, node := range r.nodes {
		if !node.Healthy || node.Draining {
			continue
		}
		spare := node.Capacity - node.CurrentLoad
		if spare <= 0 {
			continue
		}
		if spare > bestSpare {
			best = node
			bestSpare = spare
		}
	}

	// Return a copy to avoid data races after releasing the lock.
	if best != nil {
		cp := *best
		return &cp
	}
	return nil
}

// IncrementLoad atomically increments the load counter for a node.
// This is called after a successful player assignment.
func (r *Registry) IncrementLoad(nodeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if node, ok := r.nodes[nodeID]; ok {
		node.CurrentLoad++
	}
}

// SetDraining marks a node as draining (no new assignments).
func (r *Registry) SetDraining(nodeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if node, ok := r.nodes[nodeID]; ok {
		node.Draining = true
		slog.Info("node set to draining", "node_id", nodeID)
	}
}

// Remove fully removes a node from the registry.
func (r *Registry) Remove(nodeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.nodes, nodeID)
	slog.Info("node removed", "node_id", nodeID)
}

// HealthCheck iterates all nodes and marks any that have exceeded the
// heartbeat timeout as unhealthy. Should be called from a periodic goroutine.
func (r *Registry) HealthCheck() {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	for id, node := range r.nodes {
		if node.Healthy && now.Sub(node.LastSeen) > r.heartbeatTimeout {
			node.Healthy = false
			slog.Warn("node heartbeat timeout", "node_id", id, "last_seen", node.LastSeen)
		}
	}
}

// Snapshot returns a copy of all nodes (for observability / debugging).
func (r *Registry) Snapshot() []NodeInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]NodeInfo, 0, len(r.nodes))
	for _, n := range r.nodes {
		out = append(out, *n)
	}
	return out
}

// Count returns the total number of registered nodes (any health status).
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.nodes)
}

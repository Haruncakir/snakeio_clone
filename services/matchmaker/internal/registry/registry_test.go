package registry

import (
	"testing"
	"time"
)

func TestRegister(t *testing.T) {
	r := New(10 * time.Second)

	// First registration should return true (new).
	if !r.Register("n1", "localhost:8080", 50) {
		t.Fatal("expected new registration to return true")
	}
	if r.Count() != 1 {
		t.Fatalf("expected 1 node, got %d", r.Count())
	}

	// Re-registration should return false.
	if r.Register("n1", "localhost:8080", 50) {
		t.Fatal("expected re-registration to return false")
	}
	if r.Count() != 1 {
		t.Fatalf("expected 1 node after re-register, got %d", r.Count())
	}
}

func TestHeartbeat(t *testing.T) {
	r := New(10 * time.Second)

	// Heartbeat for unregistered node should fail.
	if r.Heartbeat("n1", 10) {
		t.Fatal("expected heartbeat for unknown node to return false")
	}

	r.Register("n1", "localhost:8080", 50)

	if !r.Heartbeat("n1", 10) {
		t.Fatal("expected heartbeat to succeed")
	}

	snap := r.Snapshot()
	if snap[0].CurrentLoad != 10 {
		t.Fatalf("expected load 10, got %d", snap[0].CurrentLoad)
	}
}

func TestBestNode_SelectsLeastLoaded(t *testing.T) {
	r := New(10 * time.Second)

	r.Register("n1", "host1:8080", 50)
	r.Heartbeat("n1", 40) // 10 spare

	r.Register("n2", "host2:8080", 50)
	r.Heartbeat("n2", 20) // 30 spare ← should be selected

	r.Register("n3", "host3:8080", 50)
	r.Heartbeat("n3", 49) // 1 spare

	best := r.BestNode()
	if best == nil {
		t.Fatal("expected a best node")
	}
	if best.NodeID != "n2" {
		t.Fatalf("expected n2 (most spare), got %s", best.NodeID)
	}
}

func TestBestNode_SkipsUnhealthyAndDraining(t *testing.T) {
	r := New(10 * time.Second)

	r.Register("n1", "host1:8080", 50) // healthy, 50 spare
	r.Register("n2", "host2:8080", 100) // will be unhealthy

	// Mark n2 as unhealthy by forcing staleness.
	r.mu.Lock()
	r.nodes["n2"].Healthy = false
	r.mu.Unlock()

	best := r.BestNode()
	if best == nil || best.NodeID != "n1" {
		t.Fatalf("expected n1, got %v", best)
	}

	// Mark n1 as draining.
	r.SetDraining("n1")
	best = r.BestNode()
	if best != nil {
		t.Fatalf("expected nil (all nodes unhealthy/draining), got %s", best.NodeID)
	}
}

func TestBestNode_NoCapacity(t *testing.T) {
	r := New(10 * time.Second)

	r.Register("n1", "host1:8080", 50)
	r.Heartbeat("n1", 50) // full

	best := r.BestNode()
	if best != nil {
		t.Fatalf("expected nil (no capacity), got %s", best.NodeID)
	}
}

func TestHealthCheck_MarksStaleNodes(t *testing.T) {
	r := New(100 * time.Millisecond) // very short timeout for testing

	r.Register("n1", "host1:8080", 50)

	// Immediately after registration, node should be healthy.
	r.HealthCheck()
	snap := r.Snapshot()
	if !snap[0].Healthy {
		t.Fatal("node should be healthy right after registration")
	}

	// Wait for the heartbeat timeout to expire.
	time.Sleep(150 * time.Millisecond)
	r.HealthCheck()

	snap = r.Snapshot()
	if snap[0].Healthy {
		t.Fatal("node should be unhealthy after heartbeat timeout")
	}
}

func TestIncrementLoad(t *testing.T) {
	r := New(10 * time.Second)
	r.Register("n1", "host1:8080", 50)

	r.IncrementLoad("n1")
	r.IncrementLoad("n1")
	r.IncrementLoad("n1")

	snap := r.Snapshot()
	if snap[0].CurrentLoad != 3 {
		t.Fatalf("expected load 3, got %d", snap[0].CurrentLoad)
	}
}

func TestRemove(t *testing.T) {
	r := New(10 * time.Second)
	r.Register("n1", "host1:8080", 50)
	r.Register("n2", "host2:8080", 50)

	r.Remove("n1")
	if r.Count() != 1 {
		t.Fatalf("expected 1 node after remove, got %d", r.Count())
	}
}

func TestBestNode_ReturnsCopy(t *testing.T) {
	r := New(10 * time.Second)
	r.Register("n1", "host1:8080", 50)

	best := r.BestNode()
	best.CurrentLoad = 999 // mutating the copy

	snap := r.Snapshot()
	if snap[0].CurrentLoad == 999 {
		t.Fatal("BestNode should return a defensive copy, not a pointer into the map")
	}
}

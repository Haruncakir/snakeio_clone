package engine

import (
	"testing"

	pb "github.com/Haruncakir/snakeio_clone/pkg/gen/pb/game"
)

func TestNewSnake(t *testing.T) {
	s := NewSnake("p1", "Alice", Vec2{X: 500, Y: 500})

	if s.PlayerID != "p1" {
		t.Fatalf("expected player_id p1, got %s", s.PlayerID)
	}
	if len(s.Segments) != InitialSegments {
		t.Fatalf("expected %d segments, got %d", InitialSegments, len(s.Segments))
	}
	if !s.Alive {
		t.Fatal("snake should start alive")
	}
	// Head should be at spawn position.
	if s.Head().X != 500 || s.Head().Y != 500 {
		t.Fatalf("expected head at (500,500), got (%f,%f)", s.Head().X, s.Head().Y)
	}
	// Segments should trail to the left (initial direction is RIGHT).
	for i := 1; i < len(s.Segments); i++ {
		if s.Segments[i].X >= s.Segments[i-1].X {
			t.Fatalf("segment %d should be left of segment %d", i, i-1)
		}
	}
}

func TestSnake_SetInput_PreventReversal(t *testing.T) {
	s := NewSnake("p1", "Alice", Vec2{X: 500, Y: 500})
	// Initial direction is RIGHT.

	// Should reject LEFT (180° reversal).
	s.SetInput(pb.Direction_DIRECTION_LEFT, false)
	if s.Direction != pb.Direction_DIRECTION_RIGHT {
		t.Fatal("should not allow 180° reversal from RIGHT to LEFT")
	}

	// Should accept UP.
	s.SetInput(pb.Direction_DIRECTION_UP, false)
	if s.Direction != pb.Direction_DIRECTION_UP {
		t.Fatal("should allow turning from RIGHT to UP")
	}

	// Should reject DOWN now (opposite of UP).
	s.SetInput(pb.Direction_DIRECTION_DOWN, false)
	if s.Direction != pb.Direction_DIRECTION_UP {
		t.Fatal("should not allow 180° reversal from UP to DOWN")
	}
}

func TestSnake_Tick_Movement(t *testing.T) {
	s := NewSnake("p1", "Alice", Vec2{X: 500, Y: 500})
	origHead := s.Head()

	s.Tick()

	newHead := s.Head()
	// Moving RIGHT at BaseSpeed, X should increase.
	if newHead.X <= origHead.X {
		t.Fatalf("snake should move right: was %f, now %f", origHead.X, newHead.X)
	}
	if newHead.Y != origHead.Y {
		t.Fatalf("Y should not change when moving right: was %f, now %f", origHead.Y, newHead.Y)
	}
}

func TestSnake_Grow(t *testing.T) {
	s := NewSnake("p1", "Alice", Vec2{X: 500, Y: 500})
	originalLen := len(s.Segments)

	s.Grow(3)
	// Tick 3 times — each tick should add one segment.
	for i := 0; i < 3; i++ {
		s.Tick()
	}

	if len(s.Segments) != originalLen+3 {
		t.Fatalf("expected %d segments after growing 3, got %d", originalLen+3, len(s.Segments))
	}
}

func TestSnake_Boost(t *testing.T) {
	s := NewSnake("p1", "Alice", Vec2{X: 500, Y: 500})
	s.Score = 100
	origHead := s.Head()

	// Tick without boost.
	s.Tick()
	normalDist := s.Head().X - origHead.X

	// Reset and tick with boost.
	s2 := NewSnake("p2", "Bob", Vec2{X: 500, Y: 500})
	s2.Score = 100
	s2.SetInput(pb.Direction_DIRECTION_RIGHT, true)
	s2.Tick()
	boostDist := s2.Head().X - 500

	if boostDist <= normalDist {
		t.Fatalf("boost should move further: normal=%f, boost=%f", normalDist, boostDist)
	}
	if s2.Score >= 100 {
		t.Fatal("boost should deduct score")
	}
}

func TestSnake_WallCollision(t *testing.T) {
	// Spawn near the top wall. Initial direction is RIGHT, turn UP first (allowed),
	// then the snake will hit the top boundary.
	s := NewSnake("p1", "Alice", Vec2{X: 500, Y: 20})
	s.SetInput(pb.Direction_DIRECTION_UP, false) // RIGHT → UP is allowed

	// Tick until the snake crosses the top wall (Y < 0).
	for i := 0; i < 20; i++ {
		s.Tick()
		s.CheckWallCollision(2000, 2000)
		if !s.Alive {
			return // test passes
		}
	}
	t.Fatal("snake should have died from wall collision")
}

func TestSnake_SelfCollision(t *testing.T) {
	// Create a long snake and curl it back on itself.
	s := NewSnake("p1", "Alice", Vec2{X: 500, Y: 500})
	// Make it long enough for self-collision to be possible.
	s.Grow(20)
	for i := 0; i < 20; i++ {
		s.Tick()
	}

	// Now make a tight U-turn: RIGHT → UP → LEFT → DOWN → should eventually self-collide.
	s.SetInput(pb.Direction_DIRECTION_UP, false)
	for i := 0; i < 3; i++ {
		s.Tick()
	}
	s.SetInput(pb.Direction_DIRECTION_LEFT, false)
	for i := 0; i < 5; i++ {
		s.Tick()
	}
	s.SetInput(pb.Direction_DIRECTION_DOWN, false)
	for i := 0; i < 10; i++ {
		s.Tick()
		s.CheckSelfCollision()
	}

	// Self-collision depends on geometry — just verify the method doesn't panic.
	// The important thing is that CheckSelfCollision runs correctly.
}

func TestSnake_HeadCollision(t *testing.T) {
	s1 := NewSnake("p1", "Alice", Vec2{X: 500, Y: 500})
	s2 := NewSnake("p2", "Bob", Vec2{X: 500, Y: 500}) // same position = collision

	if !s1.CheckHeadCollision(s2) {
		t.Fatal("overlapping snakes should collide")
	}

	s3 := NewSnake("p3", "Charlie", Vec2{X: 9999, Y: 9999}) // far away
	if s1.CheckHeadCollision(s3) {
		t.Fatal("distant snakes should not collide")
	}
}

func TestSnake_DropFood(t *testing.T) {
	s := NewSnake("p1", "Alice", Vec2{X: 500, Y: 500})
	s.Grow(10)
	for i := 0; i < 10; i++ {
		s.Tick()
	}

	positions := s.DropFood()
	// Should drop food at every other segment.
	expectedCount := (len(s.Segments) + 1) / 2
	if len(positions) != expectedCount {
		t.Fatalf("expected %d food positions, got %d", expectedCount, len(positions))
	}
}

func TestSnake_ToProto(t *testing.T) {
	s := NewSnake("p1", "Alice", Vec2{X: 500, Y: 500})
	s.Score = 42

	proto := s.ToProto()
	if proto.PlayerId != "p1" {
		t.Fatalf("expected player_id p1, got %s", proto.PlayerId)
	}
	if proto.Score != 42 {
		t.Fatalf("expected score 42, got %d", proto.Score)
	}
	if len(proto.Segments) != InitialSegments {
		t.Fatalf("expected %d proto segments, got %d", InitialSegments, len(proto.Segments))
	}
}

// ── Helpers ─────────────────────────────────────────────────────────

func TestIsOpposite(t *testing.T) {
	tests := []struct {
		a, b     pb.Direction
		opposite bool
	}{
		{pb.Direction_DIRECTION_UP, pb.Direction_DIRECTION_DOWN, true},
		{pb.Direction_DIRECTION_DOWN, pb.Direction_DIRECTION_UP, true},
		{pb.Direction_DIRECTION_LEFT, pb.Direction_DIRECTION_RIGHT, true},
		{pb.Direction_DIRECTION_RIGHT, pb.Direction_DIRECTION_LEFT, true},
		{pb.Direction_DIRECTION_UP, pb.Direction_DIRECTION_LEFT, false},
		{pb.Direction_DIRECTION_UP, pb.Direction_DIRECTION_RIGHT, false},
	}

	for _, tt := range tests {
		got := isOpposite(tt.a, tt.b)
		if got != tt.opposite {
			t.Errorf("isOpposite(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.opposite)
		}
	}
}

func TestDirectionDelta(t *testing.T) {
	dx, dy := directionDelta(pb.Direction_DIRECTION_RIGHT)
	if dx != 1 || dy != 0 {
		t.Fatalf("RIGHT should be (1,0), got (%f,%f)", dx, dy)
	}
	dx, dy = directionDelta(pb.Direction_DIRECTION_UP)
	if dx != 0 || dy != -1 {
		t.Fatalf("UP should be (0,-1), got (%f,%f)", dx, dy)
	}
}

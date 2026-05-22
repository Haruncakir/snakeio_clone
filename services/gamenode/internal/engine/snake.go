// Package engine implements the core game simulation.
//
// snake.go defines the Snake entity: movement physics, growth on food
// consumption, collision detection against walls and other snakes, and
// the boost mechanic that burns score for extra speed.
package engine

import (
	"math"

	pb "github.com/Haruncakir/snakeio_clone/pkg/gen/pb/game"
)

const (
	// BaseSpeed is the default movement speed in units per tick.
	BaseSpeed float32 = 5.0

	// BoostMultiplier is applied to speed when the player holds boost.
	BoostMultiplier float32 = 2.0

	// BoostScoreCost is the score deducted per tick while boosting.
	BoostScoreCost int32 = 1

	// InitialSegments is how many segments a new snake starts with.
	InitialSegments int = 3

	// SegmentSpacing is the distance between segments.
	SegmentSpacing float32 = 10.0

	// HeadRadius is used for collision detection.
	HeadRadius float32 = 8.0

	// SegmentRadius is used for head-vs-body collision.
	SegmentRadius float32 = 6.0
)

// Snake is the server-authoritative state for one player's snake.
type Snake struct {
	PlayerID  string
	Name      string
	Segments  []Vec2 // Segments[0] is the head
	Direction pb.Direction
	Speed     float32
	Score     int32
	Alive     bool
	Boosting  bool

	// growPending tracks how many segments to append on the next ticks.
	growPending int
}

// Vec2 is a 2D float32 vector.
type Vec2 struct {
	X, Y float32
}

// NewSnake spawns a snake at the given position, facing right.
func NewSnake(playerID, name string, pos Vec2) *Snake {
	segments := make([]Vec2, InitialSegments)
	for i := range segments {
		segments[i] = Vec2{
			X: pos.X - float32(i)*SegmentSpacing,
			Y: pos.Y,
		}
	}
	return &Snake{
		PlayerID:  playerID,
		Name:      name,
		Segments:  segments,
		Direction: pb.Direction_DIRECTION_RIGHT,
		Speed:     BaseSpeed,
		Score:     0,
		Alive:     true,
	}
}

// Head returns the position of the snake's head.
func (s *Snake) Head() Vec2 {
	return s.Segments[0]
}

// SetInput updates the snake's direction and boost state from a client input.
// Prevents 180° reversals (can't go directly backwards).
func (s *Snake) SetInput(dir pb.Direction, boost bool) {
	if dir != pb.Direction_DIRECTION_UNSPECIFIED && !isOpposite(s.Direction, dir) {
		s.Direction = dir
	}
	s.Boosting = boost
}

// Tick advances the snake by one simulation step.
func (s *Snake) Tick() {
	if !s.Alive {
		return
	}

	// Compute effective speed.
	speed := s.Speed
	if s.Boosting && s.Score > BoostScoreCost {
		speed *= BoostMultiplier
		s.Score -= BoostScoreCost
	}

	// Move the head in the current direction.
	dx, dy := directionDelta(s.Direction)
	newHead := Vec2{
		X: s.Segments[0].X + dx*speed,
		Y: s.Segments[0].Y + dy*speed,
	}

	// Shift all segments forward: each segment takes the position of the one ahead.
	// If we need to grow, skip removing the tail.
	if s.growPending > 0 {
		// Insert new head, keep all existing segments (tail stays = growth).
		s.Segments = append([]Vec2{newHead}, s.Segments...)
		s.growPending--
	} else {
		// Shift: copy each segment to the position of the one in front.
		for i := len(s.Segments) - 1; i > 0; i-- {
			s.Segments[i] = s.Segments[i-1]
		}
		s.Segments[0] = newHead
	}
}

// Grow queues segment growth (called when food is consumed).
func (s *Snake) Grow(amount int) {
	s.growPending += amount
}

// CheckWallCollision kills the snake if its head is outside the arena.
func (s *Snake) CheckWallCollision(arenaW, arenaH float32) {
	h := s.Head()
	if h.X < 0 || h.X > arenaW || h.Y < 0 || h.Y > arenaH {
		s.Alive = false
	}
}

// CheckSelfCollision kills the snake if its head overlaps its own body.
// Skips the first few segments to avoid false positives when turning.
func (s *Snake) CheckSelfCollision() {
	if len(s.Segments) < 5 {
		return
	}
	head := s.Head()
	for _, seg := range s.Segments[4:] {
		if dist(head, seg) < HeadRadius+SegmentRadius {
			s.Alive = false
			return
		}
	}
}

// CheckHeadCollision checks if this snake's head collides with another
// snake's body. Returns true if a collision is detected.
func (s *Snake) CheckHeadCollision(other *Snake) bool {
	if !s.Alive || !other.Alive {
		return false
	}
	head := s.Head()
	// Check against all segments of the other snake (including its head for head-on).
	for _, seg := range other.Segments {
		if dist(head, seg) < HeadRadius+SegmentRadius {
			return true
		}
	}
	return false
}

// ToProto converts the snake to its Protobuf representation.
func (s *Snake) ToProto() *pb.Snake {
	segments := make([]*pb.Position, len(s.Segments))
	for i, seg := range s.Segments {
		segments[i] = &pb.Position{X: seg.X, Y: seg.Y}
	}
	return &pb.Snake{
		PlayerId:  s.PlayerID,
		Name:      s.Name,
		Segments:  segments,
		Direction: s.Direction,
		Speed:     s.Speed,
		Score:     s.Score,
		Alive:     s.Alive,
	}
}

// SpawnFood converts a dead snake's segments into food items.
// Returns positions where food should be placed.
func (s *Snake) DropFood() []Vec2 {
	// Drop food at every other segment to avoid clustering.
	var positions []Vec2
	for i := 0; i < len(s.Segments); i += 2 {
		positions = append(positions, s.Segments[i])
	}
	return positions
}

// ── Helpers ─────────────────────────────────────────────────────────

func isOpposite(a, b pb.Direction) bool {
	switch a {
	case pb.Direction_DIRECTION_UP:
		return b == pb.Direction_DIRECTION_DOWN
	case pb.Direction_DIRECTION_DOWN:
		return b == pb.Direction_DIRECTION_UP
	case pb.Direction_DIRECTION_LEFT:
		return b == pb.Direction_DIRECTION_RIGHT
	case pb.Direction_DIRECTION_RIGHT:
		return b == pb.Direction_DIRECTION_LEFT
	}
	return false
}

func directionDelta(d pb.Direction) (float32, float32) {
	switch d {
	case pb.Direction_DIRECTION_UP:
		return 0, -1
	case pb.Direction_DIRECTION_DOWN:
		return 0, 1
	case pb.Direction_DIRECTION_LEFT:
		return -1, 0
	case pb.Direction_DIRECTION_RIGHT:
		return 1, 0
	}
	return 0, 0
}

func dist(a, b Vec2) float32 {
	dx := a.X - b.X
	dy := a.Y - b.Y
	return float32(math.Sqrt(float64(dx*dx + dy*dy)))
}

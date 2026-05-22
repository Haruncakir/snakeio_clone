package engine

import (
	"context"
	"sync"
	"testing"
	"time"

	pb "github.com/Haruncakir/snakeio_clone/pkg/gen/pb/game"
	"google.golang.org/protobuf/proto"
)

func TestGame_AddRemovePlayer(t *testing.T) {
	g := NewGame(2000, 2000, 50*time.Millisecond)

	g.AddPlayer("p1", "Alice")
	if g.PlayerCount() != 1 {
		t.Fatalf("expected 1 player, got %d", g.PlayerCount())
	}

	g.AddPlayer("p2", "Bob")
	if g.PlayerCount() != 2 {
		t.Fatalf("expected 2 players, got %d", g.PlayerCount())
	}

	g.RemovePlayer("p1")
	if g.PlayerCount() != 1 {
		t.Fatalf("expected 1 player after remove, got %d", g.PlayerCount())
	}
}

func TestGame_SubmitInput(t *testing.T) {
	g := NewGame(2000, 2000, 50*time.Millisecond)
	g.AddPlayer("p1", "Alice")

	// Submit an input — should not block.
	g.SubmitInput(PlayerInput{
		PlayerID:  "p1",
		Direction: pb.Direction_DIRECTION_UP,
		Boost:     false,
	})

	// Verify by draining: the snake's direction should change on next tick.
	g.mu.Lock()
	g.drainInputs()
	snake := g.snakes["p1"]
	g.mu.Unlock()

	if snake.Direction != pb.Direction_DIRECTION_UP {
		t.Fatalf("expected direction UP after input, got %v", snake.Direction)
	}
}

func TestGame_TickLoop_BroadcastsState(t *testing.T) {
	g := NewGame(2000, 2000, 20*time.Millisecond)
	g.AddPlayer("p1", "Alice")

	var mu sync.Mutex
	var received [][]byte

	g.SetBroadcastFunc(func(data []byte) {
		mu.Lock()
		defer mu.Unlock()
		cp := make([]byte, len(data))
		copy(cp, data)
		received = append(received, cp)
	})

	ctx, cancel := context.WithCancel(context.Background())
	go g.Run(ctx)

	// Let it run for ~100ms (should get ~5 ticks at 20ms).
	time.Sleep(120 * time.Millisecond)
	cancel()

	mu.Lock()
	defer mu.Unlock()

	if len(received) < 3 {
		t.Fatalf("expected at least 3 broadcasts, got %d", len(received))
	}

	// Verify the first broadcast is valid protobuf.
	var state pb.GameState
	if err := proto.Unmarshal(received[0], &state); err != nil {
		t.Fatalf("failed to unmarshal GameState: %v", err)
	}

	if state.Tick < 1 {
		t.Fatal("tick counter should be >= 1")
	}
	if len(state.Snakes) != 1 {
		t.Fatalf("expected 1 snake, got %d", len(state.Snakes))
	}
	if state.Snakes[0].PlayerId != "p1" {
		t.Fatalf("expected player p1, got %s", state.Snakes[0].PlayerId)
	}
	if state.ArenaW != 2000 || state.ArenaH != 2000 {
		t.Fatalf("arena dimensions wrong: %f x %f", state.ArenaW, state.ArenaH)
	}
	if len(state.Foods) < 1 {
		t.Fatal("expected some food in state")
	}
}

func TestGame_FoodConsumption(t *testing.T) {
	g := NewGame(2000, 2000, 50*time.Millisecond)

	// Place snake at a known position and put food right in front.
	g.mu.Lock()
	snake := NewSnake("p1", "Alice", Vec2{X: 500, Y: 500})
	g.snakes["p1"] = snake
	// Place food directly ahead (snake moves RIGHT).
	g.food.mu.Lock()
	g.food.items["target"] = &FoodItem{
		ID:       "target",
		Position: Vec2{X: 500 + BaseSpeed + 1, Y: 500},
		Value:    3,
	}
	g.food.mu.Unlock()
	g.mu.Unlock()

	// Run a few ticks to let the snake reach the food.
	g.SetBroadcastFunc(func(data []byte) {})
	ctx, cancel := context.WithCancel(context.Background())
	go g.Run(ctx)
	time.Sleep(150 * time.Millisecond)
	cancel()

	g.mu.RLock()
	s := g.snakes["p1"]
	score := s.Score
	g.mu.RUnlock()

	if score < 3 {
		t.Fatalf("expected score >= 3 after eating food, got %d", score)
	}
}

func TestGame_WallDeath(t *testing.T) {
	g := NewGame(200, 200, 10*time.Millisecond) // small arena

	g.mu.Lock()
	// Place snake near the right wall, moving right.
	snake := NewSnake("p1", "Alice", Vec2{X: 195, Y: 100})
	g.snakes["p1"] = snake
	g.mu.Unlock()

	var deathCalled bool
	g.SetPlayerDeathFunc(func(playerID string) {
		deathCalled = true
	})
	g.SetBroadcastFunc(func(data []byte) {})

	ctx, cancel := context.WithCancel(context.Background())
	go g.Run(ctx)
	time.Sleep(100 * time.Millisecond)
	cancel()

	g.mu.RLock()
	alive := g.snakes["p1"].Alive
	g.mu.RUnlock()

	if alive {
		t.Fatal("snake should be dead from wall collision")
	}
	if !deathCalled {
		t.Fatal("death callback should have been called")
	}
}

func TestGame_InterSnakeCollision(t *testing.T) {
	g := NewGame(2000, 2000, 10*time.Millisecond)

	g.mu.Lock()
	// Snake 1 moving RIGHT, snake 2 placed directly in its path.
	s1 := NewSnake("p1", "Alice", Vec2{X: 500, Y: 500})
	s2 := NewSnake("p2", "Bob", Vec2{X: 510, Y: 500})
	g.snakes["p1"] = s1
	g.snakes["p2"] = s2
	g.mu.Unlock()

	g.SetBroadcastFunc(func(data []byte) {})
	g.SetPlayerDeathFunc(func(playerID string) {})

	ctx, cancel := context.WithCancel(context.Background())
	go g.Run(ctx)
	time.Sleep(100 * time.Millisecond)
	cancel()

	g.mu.RLock()
	p1Alive := g.snakes["p1"].Alive
	p2Alive := g.snakes["p2"].Alive
	g.mu.RUnlock()

	// At least one should be dead from the collision.
	if p1Alive && p2Alive {
		t.Fatal("at least one snake should die from head collision")
	}
}

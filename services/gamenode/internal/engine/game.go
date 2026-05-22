// Package engine implements the server-authoritative game simulation.
//
// game.go defines the Game room: a single arena instance that runs a
// fixed-rate tick loop. Each tick:
//   1. Process all pending player inputs
//   2. Move all snakes
//   3. Check food collisions (eat → grow + score)
//   4. Check wall collisions
//   5. Check snake-vs-snake collisions (death → drop food)
//   6. Replenish food
//   7. Build and broadcast the GameState protobuf
package engine

import (
	"context"
	"log/slog"
	"math/rand"
	"sync"
	"time"

	pb "github.com/Haruncakir/snakeio_clone/pkg/gen/pb/game"
	"google.golang.org/protobuf/proto"
)

// PlayerInput is an input command received from a client.
type PlayerInput struct {
	PlayerID  string
	Direction pb.Direction
	Boost     bool
}

// BroadcastFunc is called each tick with the serialized GameState.
// The network layer provides this callback to send data to all clients.
type BroadcastFunc func(data []byte)

// PlayerDeathFunc is called when a player's snake dies.
type PlayerDeathFunc func(playerID string)

// Game is a single game room / arena instance.
type Game struct {
	mu      sync.RWMutex
	snakes  map[string]*Snake
	food    *FoodManager
	arenaW  float32
	arenaH  float32
	tick    int64
	running bool

	// inputCh buffers player inputs between ticks.
	inputCh chan PlayerInput

	// callbacks
	onBroadcast   BroadcastFunc
	onPlayerDeath PlayerDeathFunc

	// tick rate
	tickRate time.Duration
}

// NewGame creates a new Game room.
func NewGame(arenaW, arenaH float32, tickRate time.Duration) *Game {
	return &Game{
		snakes:   make(map[string]*Snake),
		food:     NewFoodManager(arenaW, arenaH, time.Now().UnixNano()),
		arenaW:   arenaW,
		arenaH:   arenaH,
		tickRate: tickRate,
		inputCh:  make(chan PlayerInput, 1024), // buffered to avoid blocking clients
	}
}

// SetBroadcastFunc sets the callback for broadcasting game state each tick.
func (g *Game) SetBroadcastFunc(fn BroadcastFunc) {
	g.onBroadcast = fn
}

// SetPlayerDeathFunc sets the callback invoked when a player dies.
func (g *Game) SetPlayerDeathFunc(fn PlayerDeathFunc) {
	g.onPlayerDeath = fn
}

// AddPlayer spawns a new snake at a random position in the arena.
func (g *Game) AddPlayer(playerID, name string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Random spawn position, padded from walls.
	x := 100 + rand.Float32()*(g.arenaW-200)
	y := 100 + rand.Float32()*(g.arenaH-200)

	snake := NewSnake(playerID, name, Vec2{X: x, Y: y})
	g.snakes[playerID] = snake

	slog.Info("player joined game",
		"player_id", playerID,
		"spawn", [2]float32{x, y},
		"total_players", len(g.snakes),
	)
}

// RemovePlayer removes a player from the game.
func (g *Game) RemovePlayer(playerID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.snakes, playerID)
	slog.Info("player removed from game", "player_id", playerID)
}

// SubmitInput queues a player input for the next tick.
func (g *Game) SubmitInput(input PlayerInput) {
	select {
	case g.inputCh <- input:
	default:
		// Drop input if buffer is full (client sending too fast).
		slog.Debug("input dropped (buffer full)", "player_id", input.PlayerID)
	}
}

// PlayerCount returns the current number of players.
func (g *Game) PlayerCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.snakes)
}

// Run starts the game loop. Blocks until ctx is cancelled.
func (g *Game) Run(ctx context.Context) {
	g.running = true
	ticker := time.NewTicker(g.tickRate)
	defer ticker.Stop()

	slog.Info("game loop started",
		"tick_rate", g.tickRate,
		"arena", [2]float32{g.arenaW, g.arenaH},
	)

	for {
		select {
		case <-ctx.Done():
			g.running = false
			slog.Info("game loop stopped", "final_tick", g.tick)
			return
		case <-ticker.C:
			g.processTick()
		}
	}
}

// processTick runs one full simulation step.
func (g *Game) processTick() {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.tick++

	// 1. Drain and apply all pending inputs.
	g.drainInputs()

	// 2. Move all snakes.
	for _, snake := range g.snakes {
		snake.Tick()
	}

	// 3. Check food collisions.
	for _, snake := range g.snakes {
		if !snake.Alive {
			continue
		}
		score := g.food.CheckCollisions(snake.Head())
		if score > 0 {
			snake.Score += score
			snake.Grow(int(score)) // 1 segment per point
		}
	}

	// 4. Check wall collisions.
	for _, snake := range g.snakes {
		if !snake.Alive {
			continue
		}
		snake.CheckWallCollision(g.arenaW, g.arenaH)
	}

	// 5. Check self-collisions.
	for _, snake := range g.snakes {
		if !snake.Alive {
			continue
		}
		snake.CheckSelfCollision()
	}

	// 6. Check snake-vs-snake collisions.
	g.checkInterSnakeCollisions()

	// 7. Handle deaths: drop food from dead snakes, notify callbacks.
	g.handleDeaths()

	// 8. Replenish natural food.
	g.food.Replenish()

	// 9. Build and broadcast state.
	g.broadcastState()
}

// drainInputs reads all pending inputs from the channel and applies them.
func (g *Game) drainInputs() {
	for {
		select {
		case input := <-g.inputCh:
			if snake, ok := g.snakes[input.PlayerID]; ok && snake.Alive {
				snake.SetInput(input.Direction, input.Boost)
			}
		default:
			return
		}
	}
}

// checkInterSnakeCollisions detects head-to-body collisions between
// different snakes. When two snakes collide head-on, both die.
func (g *Game) checkInterSnakeCollisions() {
	// Collect alive snakes into a slice for O(n²) pairwise check.
	alive := make([]*Snake, 0, len(g.snakes))
	for _, s := range g.snakes {
		if s.Alive {
			alive = append(alive, s)
		}
	}

	for i := 0; i < len(alive); i++ {
		for j := 0; j < len(alive); j++ {
			if i == j {
				continue
			}
			if alive[i].CheckHeadCollision(alive[j]) {
				alive[i].Alive = false
				// Award kill score to the surviving snake.
				alive[j].Score += alive[i].Score / 2
			}
		}
	}
}

// handleDeaths drops food for dead snakes and invokes the death callback.
func (g *Game) handleDeaths() {
	for id, snake := range g.snakes {
		if !snake.Alive {
			// Drop food at the snake's segment positions.
			positions := snake.DropFood()
			g.food.SpawnAtPositions(positions, DeathFoodValue)

			slog.Info("snake died",
				"player_id", id,
				"final_score", snake.Score,
				"dropped_food", len(positions),
			)

			if g.onPlayerDeath != nil {
				g.onPlayerDeath(id)
			}
		}
	}
}

// broadcastState serializes the current game state and sends it to all clients.
func (g *Game) broadcastState() {
	if g.onBroadcast == nil {
		return
	}

	state := &pb.GameState{
		Tick:   g.tick,
		ArenaW: g.arenaW,
		ArenaH: g.arenaH,
	}

	// Add all snakes (alive and recently dead, so clients can show death animation).
	for _, snake := range g.snakes {
		state.Snakes = append(state.Snakes, snake.ToProto())
	}

	// Add food.
	state.Foods = g.food.ToProto()

	// Serialize to binary protobuf.
	data, err := proto.Marshal(state)
	if err != nil {
		slog.Error("failed to marshal game state", "error", err, "tick", g.tick)
		return
	}

	g.onBroadcast(data)
}

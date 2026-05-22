package engine

import (
	"fmt"
	"math/rand"
	"sync"

	pb "github.com/Haruncakir/snakeio_clone/pkg/gen/pb/game"
)

const (
	// MaxFood is the upper limit of food items on the map at any time.
	MaxFood = 200

	// InitialFood is how many food items spawn when the game starts.
	InitialFood = 100

	// FoodValue is the default score value of a naturally spawned food item.
	FoodValue int32 = 1

	// DeathFoodValue is the score value of food dropped by a dead snake.
	DeathFoodValue int32 = 2

	// FoodRadius is used for snake-head-to-food collision detection.
	FoodRadius float32 = 6.0
)

// FoodItem is the server-side representation of a food pickup.
type FoodItem struct {
	ID       string
	Position Vec2
	Value    int32
}

// FoodManager handles spawning, tracking, and consuming food items.
type FoodManager struct {
	mu     sync.Mutex
	items  map[string]*FoodItem
	nextID int64
	arenaW float32
	arenaH float32
	rng    *rand.Rand
}

// NewFoodManager creates a FoodManager and spawns initial food.
func NewFoodManager(arenaW, arenaH float32, seed int64) *FoodManager {
	fm := &FoodManager{
		items:  make(map[string]*FoodItem),
		arenaW: arenaW,
		arenaH: arenaH,
		rng:    rand.New(rand.NewSource(seed)),
	}
	for range InitialFood {
		fm.spawnOne(FoodValue)
	}
	return fm
}

// spawnOne creates a single food item at a random position (must hold lock).
func (fm *FoodManager) spawnOne(value int32) {
	fm.nextID++
	id := fmt.Sprintf("f%d", fm.nextID)
	// Pad 50 units from walls so food isn't right on the edge.
	x := 50 + fm.rng.Float32()*(fm.arenaW-100)
	y := 50 + fm.rng.Float32()*(fm.arenaH-100)
	fm.items[id] = &FoodItem{
		ID:       id,
		Position: Vec2{X: x, Y: y},
		Value:    value,
	}
}

// SpawnAtPositions creates food items at specific positions (used when a
// snake dies and drops food from its segments).
func (fm *FoodManager) SpawnAtPositions(positions []Vec2, value int32) {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	for _, pos := range positions {
		if len(fm.items) >= MaxFood {
			return
		}
		fm.nextID++
		id := fmt.Sprintf("f%d", fm.nextID)
		fm.items[id] = &FoodItem{
			ID:       id,
			Position: pos,
			Value:    value,
		}
	}
}

// Replenish spawns food up to MaxFood. Called once per tick.
func (fm *FoodManager) Replenish() {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	for len(fm.items) < InitialFood {
		fm.spawnOne(FoodValue)
	}
}

// CheckCollisions tests a snake's head against all food items and
// consumes any within range. Returns the total score gained.
func (fm *FoodManager) CheckCollisions(head Vec2) int32 {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	var totalScore int32
	for id, item := range fm.items {
		if dist(head, item.Position) < HeadRadius+FoodRadius {
			totalScore += item.Value
			delete(fm.items, id)
		}
	}
	return totalScore
}

// ToProto returns all food items as Protobuf messages.
func (fm *FoodManager) ToProto() []*pb.Food {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	foods := make([]*pb.Food, 0, len(fm.items))
	for _, item := range fm.items {
		foods = append(foods, &pb.Food{
			Id:       item.ID,
			Position: &pb.Position{X: item.Position.X, Y: item.Position.Y},
			Value:    item.Value,
		})
	}
	return foods
}

// Count returns the current number of food items.
func (fm *FoodManager) Count() int {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	return len(fm.items)
}

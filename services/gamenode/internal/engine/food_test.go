package engine

import (
	"testing"
)

func TestNewFoodManager(t *testing.T) {
	fm := NewFoodManager(2000, 2000, 42)

	count := fm.Count()
	if count != InitialFood {
		t.Fatalf("expected %d initial food items, got %d", InitialFood, count)
	}
}

func TestFoodManager_CheckCollisions(t *testing.T) {
	fm := NewFoodManager(2000, 2000, 42)

	// Place a known food item at a specific position.
	fm.mu.Lock()
	fm.nextID++
	fm.items["test-food"] = &FoodItem{
		ID:       "test-food",
		Position: Vec2{X: 100, Y: 100},
		Value:    5,
	}
	fm.mu.Unlock()

	before := fm.Count()

	// Head directly on top of the food.
	score := fm.CheckCollisions(Vec2{X: 100, Y: 100})
	if score < 5 {
		t.Fatalf("expected at least 5 score from eating test food, got %d", score)
	}

	after := fm.Count()
	if after >= before {
		t.Fatalf("food count should decrease after eating: before=%d, after=%d", before, after)
	}
}

func TestFoodManager_CheckCollisions_Miss(t *testing.T) {
	fm := NewFoodManager(2000, 2000, 42)

	// Head far from any food.
	score := fm.CheckCollisions(Vec2{X: -9999, Y: -9999})
	if score != 0 {
		t.Fatalf("expected 0 score for miss, got %d", score)
	}
}

func TestFoodManager_SpawnAtPositions(t *testing.T) {
	fm := NewFoodManager(2000, 2000, 42)
	before := fm.Count()

	positions := []Vec2{
		{X: 100, Y: 100},
		{X: 200, Y: 200},
		{X: 300, Y: 300},
	}
	fm.SpawnAtPositions(positions, DeathFoodValue)

	after := fm.Count()
	if after != before+3 {
		t.Fatalf("expected %d food after spawning 3, got %d", before+3, after)
	}
}

func TestFoodManager_Replenish(t *testing.T) {
	fm := NewFoodManager(2000, 2000, 42)

	// Remove most food.
	fm.mu.Lock()
	count := 0
	for id := range fm.items {
		if count > 10 {
			break
		}
		delete(fm.items, id)
		count++
	}
	fm.mu.Unlock()

	before := fm.Count()
	fm.Replenish()
	after := fm.Count()

	if after < before {
		t.Fatal("replenish should not reduce food count")
	}
	if after < InitialFood {
		t.Fatalf("replenish should restore to at least %d, got %d", InitialFood, after)
	}
}

func TestFoodManager_MaxFoodCap(t *testing.T) {
	fm := NewFoodManager(2000, 2000, 42)

	// Try to spawn way more than MaxFood.
	positions := make([]Vec2, MaxFood+100)
	for i := range positions {
		positions[i] = Vec2{X: float32(i), Y: float32(i)}
	}
	fm.SpawnAtPositions(positions, 1)

	count := fm.Count()
	if count > MaxFood {
		t.Fatalf("food count %d exceeds MaxFood %d", count, MaxFood)
	}
}

func TestFoodManager_ToProto(t *testing.T) {
	fm := NewFoodManager(2000, 2000, 42)
	foods := fm.ToProto()

	if len(foods) != InitialFood {
		t.Fatalf("expected %d proto foods, got %d", InitialFood, len(foods))
	}

	for _, f := range foods {
		if f.Id == "" {
			t.Fatal("food should have an ID")
		}
		if f.Position == nil {
			t.Fatal("food should have a position")
		}
		if f.Position.X < 50 || f.Position.Y < 50 {
			t.Fatalf("food position should be padded from walls: (%f, %f)", f.Position.X, f.Position.Y)
		}
	}
}

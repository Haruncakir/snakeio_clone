// Package queue provides a Redis-backed matchmaking queue.
//
// Players requesting a match are pushed into a Redis list. The Matchmaker
// pops players from the queue and assigns them to the least-loaded healthy
// Game Node. Redis gives us persistence across Matchmaker restarts and
// atomic list operations that work correctly even under concurrent access.
package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// queueKey is the Redis list key for the matchmaking queue.
	queueKey = "snakeio:matchqueue"

	// playerPrefix stores per-player metadata while they wait.
	playerPrefix = "snakeio:player:"
)

// ErrQueueEmpty is returned when there are no players waiting.
var ErrQueueEmpty = errors.New("queue: empty")

// PlayerEntry holds the data for a queued player.
type PlayerEntry struct {
	PlayerID  string    `json:"player_id"`
	QueuedAt  time.Time `json:"queued_at"`
}

// Queue wraps a Redis client to implement a FIFO matchmaking queue.
type Queue struct {
	rdb *redis.Client
}

// New creates a Queue backed by the provided Redis client.
func New(rdb *redis.Client) *Queue {
	return &Queue{rdb: rdb}
}

// Enqueue adds a player to the back of the matchmaking queue.
// It also stores a metadata key so we can check if a player is already queued.
func (q *Queue) Enqueue(ctx context.Context, playerID string) error {
	entry := PlayerEntry{
		PlayerID: playerID,
		QueuedAt: time.Now(),
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("queue marshal: %w", err)
	}

	// Use a pipeline: set metadata + push to list atomically.
	pipe := q.rdb.Pipeline()
	pipe.Set(ctx, playerPrefix+playerID, data, 5*time.Minute)
	pipe.RPush(ctx, queueKey, data)
	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("queue enqueue: %w", err)
	}

	slog.Debug("player enqueued", "player_id", playerID)
	return nil
}

// Dequeue pops the next player from the front of the queue.
// Returns ErrQueueEmpty if the queue is empty.
func (q *Queue) Dequeue(ctx context.Context) (*PlayerEntry, error) {
	data, err := q.rdb.LPop(ctx, queueKey).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrQueueEmpty
		}
		return nil, fmt.Errorf("queue dequeue: %w", err)
	}

	var entry PlayerEntry
	if err := json.Unmarshal([]byte(data), &entry); err != nil {
		return nil, fmt.Errorf("queue unmarshal: %w", err)
	}

	// Clean up the per-player metadata key.
	q.rdb.Del(ctx, playerPrefix+entry.PlayerID)

	slog.Debug("player dequeued", "player_id", entry.PlayerID,
		"wait_time", time.Since(entry.QueuedAt))
	return &entry, nil
}

// IsQueued checks whether a player is currently waiting in the queue.
func (q *Queue) IsQueued(ctx context.Context, playerID string) (bool, error) {
	exists, err := q.rdb.Exists(ctx, playerPrefix+playerID).Result()
	if err != nil {
		return false, fmt.Errorf("queue exists check: %w", err)
	}
	return exists > 0, nil
}

// Len returns the current length of the matchmaking queue.
func (q *Queue) Len(ctx context.Context) (int64, error) {
	n, err := q.rdb.LLen(ctx, queueKey).Result()
	if err != nil {
		return 0, fmt.Errorf("queue len: %w", err)
	}
	return n, nil
}

// Flush empties the entire queue. Useful for testing or admin resets.
func (q *Queue) Flush(ctx context.Context) error {
	return q.rdb.Del(ctx, queueKey).Err()
}

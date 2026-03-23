// Package config provides environment-based configuration loading
// with sensible defaults for all Snake.io services.
package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds all service configuration sourced from environment variables.
type Config struct {
	// Redis
	RedisAddr     string
	RedisPassword string
	RedisDB       int

	// PostgreSQL
	PostgresDSN string

	// gRPC
	GRPCPort int

	// HTTP / WebSocket
	HTTPPort int

	// Game tuning
	TickRate      time.Duration
	ArenaWidth    float32
	ArenaHeight   float32
	MaxPlayers    int
	NodeID        string

	// Matchmaker
	MatchmakerAddr string

	// Heartbeat
	HeartbeatInterval time.Duration
}

// Load reads configuration from environment variables, falling back to defaults.
func Load() *Config {
	return &Config{
		RedisAddr:         envStr("REDIS_ADDR", "localhost:6379"),
		RedisPassword:     envStr("REDIS_PASSWORD", ""),
		RedisDB:           envInt("REDIS_DB", 0),
		PostgresDSN:       envStr("POSTGRES_DSN", "postgres://snakeio:snakeio_secret@localhost:5432/snakeio?sslmode=disable"),
		GRPCPort:          envInt("GRPC_PORT", 50051),
		HTTPPort:          envInt("HTTP_PORT", 8080),
		TickRate:          envDuration("TICK_RATE", 50*time.Millisecond),
		ArenaWidth:        float32(envInt("ARENA_WIDTH", 2000)),
		ArenaHeight:       float32(envInt("ARENA_HEIGHT", 2000)),
		MaxPlayers:        envInt("MAX_PLAYERS", 50),
		NodeID:            envStr("NODE_ID", "node-1"),
		MatchmakerAddr:    envStr("MATCHMAKER_ADDR", "localhost:50051"),
		HeartbeatInterval: envDuration("HEARTBEAT_INTERVAL", 5*time.Second),
	}
}

func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

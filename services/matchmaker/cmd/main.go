// Matchmaker Service entrypoint.
//
// The Matchmaker manages a Redis-backed player queue and assigns
// incoming players to healthy Game Nodes. Game Nodes register
// themselves via gRPC and send periodic heartbeats.
package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/Haruncakir/snakeio_clone/pkg/config"
	"github.com/Haruncakir/snakeio_clone/pkg/logger"
)

func main() {
	logger.Init()
	cfg := config.Load()

	slog.Info("matchmaker starting",
		"grpc_port", cfg.GRPCPort,
		"redis_addr", cfg.RedisAddr,
	)

	// TODO(step-3): Initialize Redis client, gRPC server, and matchmaking queue.
	fmt.Fprintf(os.Stderr, "matchmaker service not yet implemented\n")
	os.Exit(1)
}

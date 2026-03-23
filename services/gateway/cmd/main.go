// API Gateway entrypoint.
//
// The Gateway is a REST-only HTTP server. It handles:
//   - POST /match       → asks Matchmaker for a game node (returns direct addr)
//   - GET  /leaderboard → queries PostgreSQL leaderboard
//   - GET  /stats/:id   → queries player stats
//
// No WebSocket proxying — clients connect directly to Game Nodes.
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

	slog.Info("gateway starting",
		"http_port", cfg.HTTPPort,
		"matchmaker_addr", cfg.MatchmakerAddr,
	)

	// TODO(step-5): Initialize HTTP server, gRPC client to Matchmaker,
	//               PostgreSQL pool, and REST handlers.
	fmt.Fprintf(os.Stderr, "gateway service not yet implemented\n")
	os.Exit(1)
}

// Game Node Service entrypoint.
//
// Each Game Node runs one or more game rooms. Clients connect
// directly via WebSocket after receiving the node address from
// the Matchmaker. The node registers with the Matchmaker at
// startup and sends periodic heartbeats.
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

	slog.Info("gamenode starting",
		"node_id", cfg.NodeID,
		"http_port", cfg.HTTPPort,
		"grpc_port", cfg.GRPCPort,
		"tick_rate", cfg.TickRate,
		"max_players", cfg.MaxPlayers,
	)

	// TODO(step-4): Initialize WebSocket server, game engine, gRPC server,
	//               and register with the Matchmaker.
	fmt.Fprintf(os.Stderr, "gamenode service not yet implemented\n")
	os.Exit(1)
}

// API Gateway entrypoint.
//
// The Gateway is a REST-only HTTP server. It handles:
//   - POST /api/match       → asks Matchmaker for a game node address
//   - GET  /api/leaderboard → queries PostgreSQL leaderboard
//   - GET  /api/stats/:id   → queries player stats
//
// No WebSocket proxying — clients connect directly to Game Nodes.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Haruncakir/snakeio_clone/pkg/config"
	"github.com/Haruncakir/snakeio_clone/pkg/db"
	"github.com/Haruncakir/snakeio_clone/pkg/logger"
	"github.com/Haruncakir/snakeio_clone/services/gateway/internal/api"
)

func main() {
	logger.Init()
	cfg := config.Load()

	slog.Info("gateway starting",
		"http_port", cfg.HTTPPort,
		"matchmaker_addr", cfg.MatchmakerAddr,
		"postgres_dsn", "***", // don't log credentials
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ── PostgreSQL ──────────────────────────────────────────────────
	pool, err := db.NewPostgresPool(ctx, cfg.PostgresDSN)
	if err != nil {
		slog.Error("failed to connect to PostgreSQL", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	slog.Info("postgres connected")

	// ── API Server ──────────────────────────────────────────────────
	apiServer, err := api.NewServer(cfg.MatchmakerAddr, pool)
	if err != nil {
		slog.Error("failed to create API server", "error", err)
		os.Exit(1)
	}
	defer apiServer.Close()

	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:      api.CORSMiddleware(apiServer),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// ── Signal handling ─────────────────────────────────────────────
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		slog.Info("shutdown signal received", "signal", sig)
		cancel()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			slog.Error("HTTP shutdown error", "error", err)
		}
	}()

	// ── Serve ───────────────────────────────────────────────────────
	slog.Info("gateway HTTP server listening", "port", cfg.HTTPPort)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("HTTP server error", "error", err)
		os.Exit(1)
	}

	slog.Info("gateway stopped")
}

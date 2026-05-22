// Game Node Service entrypoint.
//
// Each Game Node runs one game room. Clients connect directly via
// WebSocket after receiving the node address from the Matchmaker.
//
// Architecture:
//   - HTTP server on HTTP_PORT (default 8080) — WebSocket endpoint at /ws
//   - gRPC server on GRPC_PORT (default 50051) — AssignPlayer, DrainNode
//   - Game engine running a fixed-rate tick loop
//   - gRPC client → Matchmaker for registration + heartbeats
//   - Graceful shutdown on SIGINT/SIGTERM
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/Haruncakir/snakeio_clone/pkg/config"
	"github.com/Haruncakir/snakeio_clone/pkg/logger"
	gnpb "github.com/Haruncakir/snakeio_clone/pkg/gen/pb/gamenode"
	"github.com/Haruncakir/snakeio_clone/services/gamenode/internal/engine"
	"github.com/Haruncakir/snakeio_clone/services/gamenode/internal/network"
	"github.com/Haruncakir/snakeio_clone/services/gamenode/internal/registration"
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
		"arena", [2]float32{cfg.ArenaWidth, cfg.ArenaHeight},
	)

	// ── Context for graceful shutdown ───────────────────────────────
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ── Core components ─────────────────────────────────────────────
	hub := network.NewHub()
	game := engine.NewGame(cfg.ArenaWidth, cfg.ArenaHeight, cfg.TickRate)

	// Wire the broadcast callback: game engine → hub → all WS clients.
	game.SetBroadcastFunc(hub.Broadcast)

	// Wire the death callback: when a snake dies, remove the client.
	game.SetPlayerDeathFunc(func(playerID string) {
		// Don't remove immediately — let the client see the death frame.
		// The client will disconnect on its own or we'll clean up on next tick.
		slog.Info("player death event", "player_id", playerID)
	})

	// ── gRPC server (GameNode service) ──────────────────────────────
	gnServer := registration.NewGameNodeServer(hub, hub.ClientCount)
	grpcServer := grpc.NewServer()
	gnpb.RegisterGameNodeServer(grpcServer, gnServer)
	reflection.Register(grpcServer)

	grpcLis, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.GRPCPort))
	if err != nil {
		slog.Error("gRPC listen failed", "port", cfg.GRPCPort, "error", err)
		os.Exit(1)
	}

	go func() {
		slog.Info("gamenode gRPC server listening", "port", cfg.GRPCPort)
		if err := grpcServer.Serve(grpcLis); err != nil {
			slog.Error("gRPC server error", "error", err)
		}
	}()

	// ── HTTP server (WebSocket endpoint) ────────────────────────────
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", network.Handler(hub, game))
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok","players":%d}`, hub.ClientCount())
	})

	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("gamenode HTTP server listening", "port", cfg.HTTPPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
			os.Exit(1)
		}
	}()

	// ── Register with Matchmaker ────────────────────────────────────
	wsAddr := fmt.Sprintf("%s:%d", cfg.NodeID, cfg.HTTPPort)
	mmClient, err := registration.NewMatchmakerClient(
		ctx,
		cfg.MatchmakerAddr,
		cfg.NodeID,
		wsAddr,
		int32(cfg.MaxPlayers),
		hub.ClientCount,
	)
	if err != nil {
		slog.Error("failed to connect to matchmaker", "error", err)
		os.Exit(1)
	}
	defer mmClient.Close()

	// Register (retry loop for resilience).
	go func() {
		for {
			if err := mmClient.Register(ctx); err != nil {
				slog.Error("registration failed, retrying in 3s", "error", err)
				select {
				case <-ctx.Done():
					return
				case <-time.After(3 * time.Second):
					continue
				}
			}
			break
		}
		// Start heartbeat loop after successful registration.
		mmClient.HeartbeatLoop(ctx, cfg.HeartbeatInterval)
	}()

	// ── Start game loop ─────────────────────────────────────────────
	go game.Run(ctx)

	// ── Signal handling ─────────────────────────────────────────────
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigCh
	slog.Info("shutdown signal received", "signal", sig)

	// Graceful shutdown sequence:
	// 1. Stop accepting new players
	// 2. Cancel context (stops game loop, heartbeats)
	// 3. Shut down HTTP server (existing WS connections drain)
	// 4. Stop gRPC server

	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP shutdown error", "error", err)
	}

	grpcServer.GracefulStop()
	slog.Info("gamenode stopped")
}

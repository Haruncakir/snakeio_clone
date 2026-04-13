// Matchmaker Service entrypoint.
//
// The Matchmaker manages a Redis-backed player queue and assigns
// incoming players to healthy Game Nodes. Game Nodes register
// themselves via gRPC and send periodic heartbeats.
//
// Architecture:
//   - gRPC server on GRPC_PORT (default 50051)
//   - Redis for the matchmaking queue
//   - In-memory registry for node health tracking
//   - Background goroutines: health checker + queue drainer
//   - Graceful shutdown on SIGINT/SIGTERM
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/Haruncakir/snakeio_clone/pkg/config"
	"github.com/Haruncakir/snakeio_clone/pkg/db"
	"github.com/Haruncakir/snakeio_clone/pkg/logger"
	pb "github.com/Haruncakir/snakeio_clone/pkg/gen/pb/matchmaker"
	"github.com/Haruncakir/snakeio_clone/services/matchmaker/internal/queue"
	"github.com/Haruncakir/snakeio_clone/services/matchmaker/internal/registry"
	"github.com/Haruncakir/snakeio_clone/services/matchmaker/internal/server"
)

func main() {
	logger.Init()
	cfg := config.Load()

	slog.Info("matchmaker starting",
		"grpc_port", cfg.GRPCPort,
		"redis_addr", cfg.RedisAddr,
		"heartbeat_interval", cfg.HeartbeatInterval,
	)

	// ── Context with cancellation for graceful shutdown ──────────────
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ── Redis ───────────────────────────────────────────────────────
	rdb, err := db.NewRedisClient(ctx, cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		slog.Error("failed to connect to Redis", "error", err)
		os.Exit(1)
	}
	defer rdb.Close()
	slog.Info("redis connected", "addr", cfg.RedisAddr)

	// ── Core components ─────────────────────────────────────────────
	// Heartbeat timeout = 3× the heartbeat interval. If a node misses
	// three consecutive heartbeats, it's considered dead.
	heartbeatTimeout := cfg.HeartbeatInterval * 3
	reg := registry.New(heartbeatTimeout)
	q := queue.New(rdb)
	srv := server.New(reg, q)

	// ── gRPC server ─────────────────────────────────────────────────
	grpcServer := grpc.NewServer()
	pb.RegisterMatchmakerServer(grpcServer, srv)
	reflection.Register(grpcServer) // enables grpcurl introspection

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.GRPCPort))
	if err != nil {
		slog.Error("failed to listen", "port", cfg.GRPCPort, "error", err)
		os.Exit(1)
	}

	// ── Background goroutines ───────────────────────────────────────
	// 1. Health checker: marks stale nodes as unhealthy.
	go func() {
		ticker := time.NewTicker(cfg.HeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				reg.HealthCheck()
			}
		}
	}()

	// 2. Queue drainer: periodically assigns queued players to nodes
	//    that have since gained capacity.
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				n, err := srv.DrainQueue(ctx)
				if err != nil {
					slog.Error("queue drain error", "error", err)
				} else if n > 0 {
					slog.Info("queue drained", "assigned", n)
				}
			}
		}
	}()

	// ── Signal handling ─────────────────────────────────────────────
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		slog.Info("shutdown signal received", "signal", sig)
		cancel()
		grpcServer.GracefulStop()
	}()

	// ── Serve ───────────────────────────────────────────────────────
	slog.Info("matchmaker gRPC server listening", "port", cfg.GRPCPort)
	if err := grpcServer.Serve(lis); err != nil {
		slog.Error("gRPC server error", "error", err)
		os.Exit(1)
	}

	slog.Info("matchmaker stopped")
}

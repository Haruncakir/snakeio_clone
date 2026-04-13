// Package server implements the Matchmaker gRPC service.
//
// It wires together the node registry and the player queue to implement
// the three RPCs: RegisterNode, Heartbeat, and RequestMatch.
//
// Flow:
//  1. Game Nodes call RegisterNode at startup → stored in registry.
//  2. Game Nodes call Heartbeat every N seconds → registry updates liveness.
//  3. Gateway calls RequestMatch on behalf of a client:
//     a. Player is enqueued (or matched immediately if capacity exists).
//     b. Matchmaker picks the least-loaded healthy node via registry.BestNode().
//     c. Returns the node's direct WebSocket address + a session token.
package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"

	pb "github.com/Haruncakir/snakeio_clone/pkg/gen/pb/matchmaker"
	"github.com/Haruncakir/snakeio_clone/services/matchmaker/internal/queue"
	"github.com/Haruncakir/snakeio_clone/services/matchmaker/internal/registry"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// MatchmakerServer implements the pb.MatchmakerServer interface.
type MatchmakerServer struct {
	pb.UnimplementedMatchmakerServer
	reg   *registry.Registry
	queue *queue.Queue
}

// New creates a MatchmakerServer backed by the given registry and queue.
func New(reg *registry.Registry, q *queue.Queue) *MatchmakerServer {
	return &MatchmakerServer{
		reg:   reg,
		queue: q,
	}
}

// RegisterNode is called by a Game Node at startup to announce itself.
func (s *MatchmakerServer) RegisterNode(
	ctx context.Context,
	req *pb.RegisterNodeReq,
) (*pb.RegisterNodeResp, error) {
	if req.NodeId == "" || req.Address == "" {
		return nil, status.Error(codes.InvalidArgument, "node_id and address are required")
	}
	if req.Capacity <= 0 {
		return nil, status.Error(codes.InvalidArgument, "capacity must be > 0")
	}

	s.reg.Register(req.NodeId, req.Address, req.Capacity)

	slog.Info("RegisterNode RPC completed",
		"node_id", req.NodeId,
		"address", req.Address,
		"capacity", req.Capacity,
		"total_nodes", s.reg.Count(),
	)

	return &pb.RegisterNodeResp{Accepted: true}, nil
}

// Heartbeat is called periodically by Game Nodes to report liveness and load.
func (s *MatchmakerServer) Heartbeat(
	ctx context.Context,
	req *pb.HeartbeatReq,
) (*pb.HeartbeatResp, error) {
	if req.NodeId == "" {
		return nil, status.Error(codes.InvalidArgument, "node_id is required")
	}

	ok := s.reg.Heartbeat(req.NodeId, req.CurrentLoad)
	if !ok {
		// Node not registered — tell it to re-register.
		return nil, status.Errorf(codes.NotFound,
			"node %q not registered; call RegisterNode first", req.NodeId)
	}

	slog.Debug("heartbeat received",
		"node_id", req.NodeId,
		"load", req.CurrentLoad,
	)

	return &pb.HeartbeatResp{
		Acknowledged: true,
		Drain:        false, // TODO: implement admin-triggered drain
	}, nil
}

// RequestMatch is called by the Gateway on behalf of a player seeking a game.
//
// The matchmaker immediately tries to find a healthy node with spare capacity.
// If found, it returns the node's direct address and a session token.
// If no node is available, it returns UNAVAILABLE so the Gateway can retry or
// inform the client.
func (s *MatchmakerServer) RequestMatch(
	ctx context.Context,
	req *pb.MatchReq,
) (*pb.MatchResp, error) {
	if req.PlayerId == "" {
		return nil, status.Error(codes.InvalidArgument, "player_id is required")
	}

	// Check if the player is already queued to prevent duplicates.
	queued, err := s.queue.IsQueued(ctx, req.PlayerId)
	if err != nil {
		slog.Error("queue check failed", "player_id", req.PlayerId, "error", err)
		return nil, status.Error(codes.Internal, "internal queue error")
	}
	if queued {
		return nil, status.Error(codes.AlreadyExists, "player already in queue")
	}

	// Find the least-loaded healthy node.
	node := s.reg.BestNode()
	if node == nil {
		// No capacity right now — enqueue the player for later assignment.
		if err := s.queue.Enqueue(ctx, req.PlayerId); err != nil {
			slog.Error("enqueue failed", "player_id", req.PlayerId, "error", err)
			return nil, status.Error(codes.Internal, "failed to enqueue player")
		}

		slog.Warn("no available nodes, player enqueued",
			"player_id", req.PlayerId)

		return &pb.MatchResp{
			Matched: false,
		}, nil
	}

	// Generate a one-time session token the player will present on WebSocket connect.
	token, err := generateToken()
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to generate token")
	}

	// Optimistically increment load so the next request sees updated capacity.
	s.reg.IncrementLoad(node.NodeID)

	slog.Info("player matched",
		"player_id", req.PlayerId,
		"node_id", node.NodeID,
		"node_addr", node.Address,
		"node_load", node.CurrentLoad+1,
	)

	return &pb.MatchResp{
		Matched:  true,
		NodeId:   node.NodeID,
		NodeAddr: node.Address,
		Token:    token,
	}, nil
}

// DrainQueue attempts to assign all queued players to available nodes.
// Called periodically from a background goroutine.
func (s *MatchmakerServer) DrainQueue(ctx context.Context) (int, error) {
	assigned := 0
	for {
		node := s.reg.BestNode()
		if node == nil {
			break // no capacity
		}

		entry, err := s.queue.Dequeue(ctx)
		if err != nil {
			if errors.Is(err, queue.ErrQueueEmpty) {
				break
			}
			return assigned, fmt.Errorf("drain dequeue: %w", err)
		}

		s.reg.IncrementLoad(node.NodeID)
		assigned++

		slog.Info("queued player assigned",
			"player_id", entry.PlayerID,
			"node_id", node.NodeID,
			"waited", entry.QueuedAt,
		)
	}
	return assigned, nil
}

// generateToken creates a cryptographically random 16-byte hex token.
func generateToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Package registration handles the Game Node's relationship with the Matchmaker.
//
// It provides:
//   - A gRPC client that registers the node at startup and sends periodic heartbeats.
//   - A gRPC server that implements the GameNode service (AssignPlayer, DrainNode).
package registration

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	mmpb "github.com/Haruncakir/snakeio_clone/pkg/gen/pb/matchmaker"
	gnpb "github.com/Haruncakir/snakeio_clone/pkg/gen/pb/gamenode"
	"github.com/Haruncakir/snakeio_clone/services/gamenode/internal/network"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// MatchmakerClient manages the connection to the Matchmaker service.
type MatchmakerClient struct {
	conn     *grpc.ClientConn
	client   mmpb.MatchmakerClient
	nodeID   string
	address  string // this node's public WebSocket address
	capacity int32

	// loadFunc returns the current player count (injected from hub).
	loadFunc func() int
}

// NewMatchmakerClient dials the Matchmaker and returns a client.
func NewMatchmakerClient(
	ctx context.Context,
	matchmakerAddr string,
	nodeID string,
	wsAddress string,
	capacity int32,
	loadFunc func() int,
) (*MatchmakerClient, error) {
	conn, err := grpc.NewClient(
		matchmakerAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("dial matchmaker: %w", err)
	}

	mc := &MatchmakerClient{
		conn:     conn,
		client:   mmpb.NewMatchmakerClient(conn),
		nodeID:   nodeID,
		address:  wsAddress,
		capacity: capacity,
		loadFunc: loadFunc,
	}

	return mc, nil
}

// Register announces this node to the Matchmaker.
func (mc *MatchmakerClient) Register(ctx context.Context) error {
	resp, err := mc.client.RegisterNode(ctx, &mmpb.RegisterNodeReq{
		NodeId:   mc.nodeID,
		Address:  mc.address,
		Capacity: mc.capacity,
	})
	if err != nil {
		return fmt.Errorf("register node: %w", err)
	}
	if !resp.Accepted {
		return fmt.Errorf("registration rejected by matchmaker")
	}
	slog.Info("registered with matchmaker",
		"node_id", mc.nodeID,
		"address", mc.address,
		"capacity", mc.capacity,
	)
	return nil
}

// HeartbeatLoop sends periodic heartbeats to the Matchmaker.
// Blocks until ctx is cancelled.
func (mc *MatchmakerClient) HeartbeatLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("heartbeat loop stopped")
			return
		case <-ticker.C:
			load := int32(mc.loadFunc())
			resp, err := mc.client.Heartbeat(ctx, &mmpb.HeartbeatReq{
				NodeId:      mc.nodeID,
				CurrentLoad: load,
			})
			if err != nil {
				slog.Error("heartbeat failed", "error", err)
				continue
			}
			if resp.Drain {
				slog.Warn("matchmaker requested drain")
				// TODO: trigger graceful drain
			}
		}
	}
}

// Close shuts down the gRPC connection.
func (mc *MatchmakerClient) Close() error {
	return mc.conn.Close()
}

// ── GameNode gRPC Server ────────────────────────────────────────────

// GameNodeServer implements the gnpb.GameNodeServer interface.
type GameNodeServer struct {
	gnpb.UnimplementedGameNodeServer
	hub      *network.Hub
	draining atomic.Bool
	loadFunc func() int
}

// NewGameNodeServer creates the gRPC server for the GameNode service.
func NewGameNodeServer(hub *network.Hub, loadFunc func() int) *GameNodeServer {
	return &GameNodeServer{
		hub:      hub,
		loadFunc: loadFunc,
	}
}

// AssignPlayer is called by the Matchmaker to reserve a slot for a player.
// It stores the session token so the player can connect via WebSocket.
func (s *GameNodeServer) AssignPlayer(
	ctx context.Context,
	req *gnpb.AssignPlayerReq,
) (*gnpb.AssignPlayerResp, error) {
	if s.draining.Load() {
		return &gnpb.AssignPlayerResp{
			Accepted: false,
			Reason:   "node is draining",
		}, nil
	}

	// Store the token in the hub — when the player connects via WS, the
	// token is validated and consumed.
	s.hub.RegisterToken(req.Token, req.PlayerId)

	slog.Info("player assigned via gRPC",
		"player_id", req.PlayerId,
		"pending_tokens", s.hub.PendingTokenCount(),
	)

	return &gnpb.AssignPlayerResp{Accepted: true}, nil
}

// DrainNode sets the node to draining mode — no new player assignments.
func (s *GameNodeServer) DrainNode(
	ctx context.Context,
	req *gnpb.DrainReq,
) (*gnpb.DrainResp, error) {
	s.draining.Store(true)
	remaining := int32(s.loadFunc())

	slog.Info("node draining initiated", "remaining_players", remaining)

	return &gnpb.DrainResp{
		RemainingPlayers: remaining,
	}, nil
}

// IsDraining returns whether the node is in drain mode.
func (s *GameNodeServer) IsDraining() bool {
	return s.draining.Load()
}

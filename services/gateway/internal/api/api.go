// Package api implements the REST HTTP handlers for the API Gateway.
//
// Endpoints:
//   - POST /api/match        — request a game match, returns Game Node address
//   - GET  /api/leaderboard  — top scores from PostgreSQL
//   - GET  /api/stats/:id    — per-player stats from PostgreSQL
//   - GET  /health           — service health check
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	pb "github.com/Haruncakir/snakeio_clone/pkg/gen/pb/matchmaker"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Server holds dependencies for all HTTP handlers.
type Server struct {
	mmClient pb.MatchmakerClient
	mmConn   *grpc.ClientConn
	db       *pgxpool.Pool
	mux      *http.ServeMux
}

// NewServer creates the API gateway server with a gRPC connection to the
// Matchmaker and a PostgreSQL connection pool.
func NewServer(matchmakerAddr string, db *pgxpool.Pool) (*Server, error) {
	conn, err := grpc.NewClient(
		matchmakerAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, err
	}

	s := &Server{
		mmClient: pb.NewMatchmakerClient(conn),
		mmConn:   conn,
		db:       db,
		mux:      http.NewServeMux(),
	}

	s.routes()
	return s, nil
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// Close releases all resources.
func (s *Server) Close() error {
	return s.mmConn.Close()
}

// routes registers all HTTP endpoints.
func (s *Server) routes() {
	s.mux.HandleFunc("POST /api/match", s.handleMatch)
	s.mux.HandleFunc("GET /api/leaderboard", s.handleLeaderboard)
	s.mux.HandleFunc("GET /api/stats/{id}", s.handleStats)
	s.mux.HandleFunc("GET /health", s.handleHealth)
}

// ── Handlers ────────────────────────────────────────────────────────

// MatchRequest is the JSON body for POST /api/match.
type MatchRequest struct {
	PlayerID string `json:"player_id"`
}

// MatchResponse is returned to the client with the game node address.
type MatchResponse struct {
	Matched  bool   `json:"matched"`
	NodeAddr string `json:"node_addr,omitempty"` // direct WS address
	Token    string `json:"token,omitempty"`      // session token for WS auth
	Message  string `json:"message,omitempty"`
}

func (s *Server) handleMatch(w http.ResponseWriter, r *http.Request) {
	var req MatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.PlayerID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "player_id is required"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	resp, err := s.mmClient.RequestMatch(ctx, &pb.MatchReq{
		PlayerId: req.PlayerID,
	})
	if err != nil {
		slog.Error("matchmaker request failed", "error", err, "player_id", req.PlayerID)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "matchmaking service unavailable",
		})
		return
	}

	if resp.Matched {
		writeJSON(w, http.StatusOK, MatchResponse{
			Matched:  true,
			NodeAddr: resp.NodeAddr,
			Token:    resp.Token,
		})
	} else {
		writeJSON(w, http.StatusAccepted, MatchResponse{
			Matched: false,
			Message: "queued — no available servers, try again shortly",
		})
	}
}

// LeaderboardEntry is a single row in the leaderboard response.
type LeaderboardEntry struct {
	PlayerID  string `json:"player_id"`
	Username  string `json:"username"`
	Score     int64  `json:"score"`
	Kills     int    `json:"kills"`
	PlayedAt  string `json:"played_at"`
}

func (s *Server) handleLeaderboard(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit := 25
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	rows, err := s.db.Query(ctx, `
		SELECT l.player_id, p.username, l.score, l.kills, l.played_at
		FROM leaderboard l
		JOIN players p ON p.id = l.player_id
		ORDER BY l.score DESC
		LIMIT $1
	`, limit)
	if err != nil {
		slog.Error("leaderboard query failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
		return
	}
	defer rows.Close()

	entries := make([]LeaderboardEntry, 0, limit)
	for rows.Next() {
		var e LeaderboardEntry
		var playedAt time.Time
		if err := rows.Scan(&e.PlayerID, &e.Username, &e.Score, &e.Kills, &playedAt); err != nil {
			slog.Error("leaderboard row scan failed", "error", err)
			continue
		}
		e.PlayedAt = playedAt.Format(time.RFC3339)
		entries = append(entries, e)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"leaderboard": entries,
		"count":       len(entries),
	})
}

// StatsResponse is the per-player stats response.
type StatsResponse struct {
	PlayerID     string `json:"player_id"`
	Username     string `json:"username"`
	GamesPlayed  int64  `json:"games_played"`
	TotalKills   int64  `json:"total_kills"`
	TotalScore   int64  `json:"total_score"`
	HighestScore int64  `json:"highest_score"`
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	playerID := r.PathValue("id")
	if playerID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "player id required"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	var stats StatsResponse
	err := s.db.QueryRow(ctx, `
		SELECT p.id, p.username, 
		       COALESCE(s.games_played, 0), COALESCE(s.total_kills, 0),
		       COALESCE(s.total_score, 0), COALESCE(s.highest_score, 0)
		FROM players p
		LEFT JOIN player_stats s ON s.player_id = p.id
		WHERE p.id = $1
	`, playerID).Scan(
		&stats.PlayerID, &stats.Username,
		&stats.GamesPlayed, &stats.TotalKills,
		&stats.TotalScore, &stats.HighestScore,
	)
	if err != nil {
		slog.Error("stats query failed", "error", err, "player_id", playerID)
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "player not found"})
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ── Helpers ─────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// CORSMiddleware adds CORS headers for browser clients.
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

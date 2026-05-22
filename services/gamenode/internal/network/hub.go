// Package network handles WebSocket connections and message routing.
//
// hub.go defines the Hub: a concurrent connection manager that tracks
// active players, routes messages between the game engine and clients,
// and provides the broadcast callback for the game loop.
package network

import (
	"log/slog"
	"sync"
)

// Client represents a single WebSocket connection.
type Client struct {
	PlayerID string
	Name     string
	SendCh   chan []byte // buffered channel for outbound messages
	hub      *Hub
}

// Hub manages all active WebSocket clients for a single game node.
type Hub struct {
	mu      sync.RWMutex
	clients map[string]*Client // keyed by playerID

	// tokens stores valid session tokens mapped to player IDs.
	// A token is consumed (deleted) once the player connects.
	tokensMu sync.Mutex
	tokens   map[string]string // token → playerID
}

// NewHub creates an empty Hub.
func NewHub() *Hub {
	return &Hub{
		clients: make(map[string]*Client),
		tokens:  make(map[string]string),
	}
}

// RegisterToken stores a session token for an expected player connection.
// Called when the Matchmaker assigns a player to this node.
func (h *Hub) RegisterToken(token, playerID string) {
	h.tokensMu.Lock()
	defer h.tokensMu.Unlock()
	h.tokens[token] = playerID
	slog.Debug("token registered", "player_id", playerID)
}

// ValidateToken checks and consumes a session token. Returns the associated
// playerID, or empty string if the token is invalid/expired.
func (h *Hub) ValidateToken(token string) string {
	h.tokensMu.Lock()
	defer h.tokensMu.Unlock()
	playerID, ok := h.tokens[token]
	if !ok {
		return ""
	}
	delete(h.tokens, token) // one-time use
	return playerID
}

// AddClient registers a client in the hub.
func (h *Hub) AddClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[client.PlayerID] = client
	slog.Info("client connected", "player_id", client.PlayerID, "total", len(h.clients))
}

// RemoveClient removes a client from the hub and closes its send channel.
func (h *Hub) RemoveClient(playerID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if client, ok := h.clients[playerID]; ok {
		close(client.SendCh)
		delete(h.clients, playerID)
		slog.Info("client disconnected", "player_id", playerID, "total", len(h.clients))
	}
}

// Broadcast sends a binary message to ALL connected clients.
// This is the callback wired into the game engine's BroadcastFunc.
func (h *Hub) Broadcast(data []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, client := range h.clients {
		select {
		case client.SendCh <- data:
		default:
			// Client is too slow — drop the frame rather than blocking the game loop.
			slog.Debug("frame dropped for slow client", "player_id", client.PlayerID)
		}
	}
}

// ClientCount returns the number of connected clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// PendingTokenCount returns how many tokens are waiting for connections.
func (h *Hub) PendingTokenCount() int {
	h.tokensMu.Lock()
	defer h.tokensMu.Unlock()
	return len(h.tokens)
}

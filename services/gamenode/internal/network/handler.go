package network

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	pb "github.com/Haruncakir/snakeio_clone/pkg/gen/pb/game"
	"github.com/Haruncakir/snakeio_clone/services/gamenode/internal/engine"
	"google.golang.org/protobuf/proto"
)

const (
	// writeWait is the max time to wait for a write to complete.
	writeWait = 10 * time.Second

	// pongWait is the max time to wait for a pong from the client.
	pongWait = 30 * time.Second

	// pingPeriod is how often we send pings (must be < pongWait).
	pingPeriod = 20 * time.Second

	// maxMessageSize is the max inbound message size (PlayerInput is tiny).
	maxMessageSize = 512

	// sendBufferSize is the size of the per-client outbound message channel.
	sendBufferSize = 64
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	// In production, restrict origins. For dev, allow all.
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Handler creates an http.HandlerFunc that upgrades requests to WebSocket
// connections. The token query parameter is validated against the Hub,
// and the player is added to the Game.
func Handler(hub *Hub, game *engine.Game) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract session token from query string.
		token := r.URL.Query().Get("token")
		if token == "" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}

		// Validate and consume the one-time token.
		playerID := hub.ValidateToken(token)
		if playerID == "" {
			http.Error(w, "invalid or expired token", http.StatusUnauthorized)
			return
		}

		// Optional player name from query.
		name := r.URL.Query().Get("name")
		if name == "" {
			name = playerID[:8] // use first 8 chars of ID as fallback
		}

		// Upgrade to WebSocket.
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			slog.Error("websocket upgrade failed", "error", err, "player_id", playerID)
			return
		}

		client := &Client{
			PlayerID: playerID,
			Name:     name,
			SendCh:   make(chan []byte, sendBufferSize),
			hub:      hub,
		}

		hub.AddClient(client)
		game.AddPlayer(playerID, name)

		// Start read and write goroutines.
		go clientWriter(r.Context(), conn, client)
		go clientReader(conn, client, game)
	}
}

// clientReader reads PlayerInput messages from the WebSocket and submits
// them to the game engine. Runs until the connection closes.
func clientReader(conn *websocket.Conn, client *Client, game *engine.Game) {
	defer func() {
		client.hub.RemoveClient(client.PlayerID)
		game.RemovePlayer(client.PlayerID)
		conn.Close()
	}()

	conn.SetReadLimit(maxMessageSize)
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err,
				websocket.CloseGoingAway,
				websocket.CloseNormalClosure) {
				slog.Warn("ws read error", "player_id", client.PlayerID, "error", err)
			}
			return
		}

		// We only accept binary (protobuf) messages.
		if messageType != websocket.BinaryMessage {
			continue
		}

		var input pb.PlayerInput
		if err := proto.Unmarshal(data, &input); err != nil {
			slog.Debug("invalid player input", "player_id", client.PlayerID, "error", err)
			continue
		}

		game.SubmitInput(engine.PlayerInput{
			PlayerID:  client.PlayerID,
			Direction: input.Direction,
			Boost:     input.Boost,
		})
	}
}

// clientWriter writes outbound messages (game state ticks) and pings
// to the WebSocket. Runs until the send channel is closed.
func clientWriter(ctx context.Context, conn *websocket.Conn, client *Client) {
	pingTicker := time.NewTicker(pingPeriod)
	defer func() {
		pingTicker.Stop()
		conn.Close()
	}()

	for {
		select {
		case <-ctx.Done():
			return

		case msg, ok := <-client.SendCh:
			if !ok {
				// Hub closed the channel — send close frame.
				conn.SetWriteDeadline(time.Now().Add(writeWait))
				conn.WriteMessage(websocket.CloseMessage, nil)
				return
			}

			conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.BinaryMessage, msg); err != nil {
				slog.Debug("ws write error", "player_id", client.PlayerID, "error", err)
				return
			}

		case <-pingTicker.C:
			conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

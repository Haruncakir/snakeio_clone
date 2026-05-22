// Load Tester for Snake.io Game Nodes.
//
// Simulates N concurrent players, each connecting via WebSocket to a
// Game Node, sending random PlayerInput commands, and receiving
// GameState ticks. Collects and reports latency, throughput, and
// error rate metrics.
//
// Usage:
//   go run ./loadtest/cmd/main.go \
//     -target ws://localhost:8080/ws \
//     -clients 500 \
//     -duration 30s \
//     -rampup 5s
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"math/rand"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	pb "github.com/Haruncakir/snakeio_clone/pkg/gen/pb/game"
	"github.com/Haruncakir/snakeio_clone/pkg/logger"
	"google.golang.org/protobuf/proto"
)

// Metrics holds the aggregate load test statistics.
type Metrics struct {
	ticksReceived  atomic.Int64
	bytesReceived  atomic.Int64
	inputsSent     atomic.Int64
	errors         atomic.Int64
	connectErrors  atomic.Int64
	latenciesMu    sync.Mutex
	latencies      []time.Duration // per-tick receive latencies
}

func main() {
	logger.Init()

	target := flag.String("target", "ws://localhost:8080/ws", "WebSocket URL of the Game Node")
	clients := flag.Int("clients", 100, "Number of concurrent simulated players")
	duration := flag.Duration("duration", 30*time.Second, "Test duration")
	rampup := flag.Duration("rampup", 5*time.Second, "Time to ramp up all clients")
	tokenPrefix := flag.String("token-prefix", "loadtest-", "Prefix for generated session tokens")
	flag.Parse()

	slog.Info("load test starting",
		"target", *target,
		"clients", *clients,
		"duration", *duration,
		"rampup", *rampup,
	)

	ctx, cancel := context.WithTimeout(context.Background(), *duration+*rampup+5*time.Second)
	defer cancel()

	// Also cancel on SIGINT.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	metrics := &Metrics{}
	var wg sync.WaitGroup

	// Ramp up clients gradually.
	delay := *rampup / time.Duration(*clients)
	if delay < time.Millisecond {
		delay = time.Millisecond
	}

	startTime := time.Now()

	for i := 0; i < *clients; i++ {
		select {
		case <-ctx.Done():
			break
		default:
		}

		wg.Add(1)
		token := fmt.Sprintf("%s%d", *tokenPrefix, i)
		go simulateClient(ctx, &wg, *target, token, *duration, metrics)

		if i < *clients-1 {
			time.Sleep(delay)
		}
	}

	// Wait for all clients to finish.
	wg.Wait()
	elapsed := time.Since(startTime)

	// Print results.
	printResults(metrics, elapsed, *clients)
}

// simulateClient runs a single simulated player.
func simulateClient(
	ctx context.Context,
	wg *sync.WaitGroup,
	target string,
	token string,
	duration time.Duration,
	metrics *Metrics,
) {
	defer wg.Done()

	// Add token to URL.
	u, err := url.Parse(target)
	if err != nil {
		metrics.connectErrors.Add(1)
		return
	}
	q := u.Query()
	q.Set("token", token)
	q.Set("name", "loadtest-"+token)
	u.RawQuery = q.Encode()

	// Connect.
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		metrics.connectErrors.Add(1)
		slog.Debug("connect failed", "token", token, "error", err)
		return
	}
	defer conn.Close()

	// Start a goroutine to send random inputs.
	go func() {
		directions := []pb.Direction{
			pb.Direction_DIRECTION_UP,
			pb.Direction_DIRECTION_DOWN,
			pb.Direction_DIRECTION_LEFT,
			pb.Direction_DIRECTION_RIGHT,
		}
		ticker := time.NewTicker(100 * time.Millisecond) // 10 inputs/sec
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				input := &pb.PlayerInput{
					Direction: directions[rand.Intn(len(directions))],
					Boost:     rand.Float32() < 0.1, // 10% chance of boost
				}
				data, err := proto.Marshal(input)
				if err != nil {
					continue
				}
				conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
				if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
					metrics.errors.Add(1)
					return
				}
				metrics.inputsSent.Add(1)
			}
		}
	}()

	// Read game state ticks.
	deadline := time.After(duration)
	for {
		select {
		case <-ctx.Done():
			return
		case <-deadline:
			return
		default:
		}

		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		readStart := time.Now()
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			metrics.errors.Add(1)
			return
		}
		readLatency := time.Since(readStart)

		if msgType != websocket.BinaryMessage {
			continue
		}

		// Verify it's valid protobuf.
		var state pb.GameState
		if err := proto.Unmarshal(data, &state); err != nil {
			metrics.errors.Add(1)
			continue
		}

		metrics.ticksReceived.Add(1)
		metrics.bytesReceived.Add(int64(len(data)))
		metrics.latenciesMu.Lock()
		metrics.latencies = append(metrics.latencies, readLatency)
		metrics.latenciesMu.Unlock()
	}
}

// printResults outputs a summary of the load test.
func printResults(m *Metrics, elapsed time.Duration, clients int) {
	ticks := m.ticksReceived.Load()
	bytes := m.bytesReceived.Load()
	inputs := m.inputsSent.Load()
	errors := m.errors.Load()
	connectErrors := m.connectErrors.Load()

	fmt.Println("\n══════════════════════════════════════════════════")
	fmt.Println("  SNAKE.IO LOAD TEST RESULTS")
	fmt.Println("══════════════════════════════════════════════════")
	fmt.Printf("  Duration:          %s\n", elapsed.Round(time.Millisecond))
	fmt.Printf("  Target Clients:    %d\n", clients)
	fmt.Printf("  Connect Errors:    %d\n", connectErrors)
	fmt.Printf("  Runtime Errors:    %d\n", errors)
	fmt.Println("──────────────────────────────────────────────────")
	fmt.Printf("  Ticks Received:    %d\n", ticks)
	fmt.Printf("  Inputs Sent:       %d\n", inputs)
	fmt.Printf("  Data Received:     %.2f MB\n", float64(bytes)/1024/1024)
	if elapsed.Seconds() > 0 {
		fmt.Printf("  Ticks/sec:         %.0f\n", float64(ticks)/elapsed.Seconds())
		fmt.Printf("  Throughput:        %.2f MB/s\n", float64(bytes)/1024/1024/elapsed.Seconds())
	}
	fmt.Println("──────────────────────────────────────────────────")

	// Latency percentiles.
	m.latenciesMu.Lock()
	lats := m.latencies
	m.latenciesMu.Unlock()

	if len(lats) > 0 {
		sort.Slice(lats, func(i, j int) bool { return lats[i] < lats[j] })
		fmt.Printf("  Latency p50:       %s\n", lats[len(lats)*50/100])
		fmt.Printf("  Latency p90:       %s\n", lats[len(lats)*90/100])
		fmt.Printf("  Latency p99:       %s\n", lats[len(lats)*99/100])
		fmt.Printf("  Latency max:       %s\n", lats[len(lats)-1])
	}
	fmt.Println("══════════════════════════════════════════════════")
}

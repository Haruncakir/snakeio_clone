// Load Tester entrypoint.
//
// Simulates thousands of concurrent WebSocket clients connecting
// to Game Nodes using goroutines. Collects latency, throughput,
// and error rate metrics.
package main

import (
	"fmt"
	"os"

	"github.com/Haruncakir/snakeio_clone/pkg/logger"
)

func main() {
	logger.Init()

	// TODO(step-7): Parse flags, spin up goroutines, each opening a WS conn,
	//               sending PlayerInput, and measuring tick roundtrip.
	fmt.Fprintf(os.Stderr, "load tester not yet implemented\n")
	os.Exit(1)
}

// Package logger provides structured JSON logging via log/slog.
package logger

import (
	"log/slog"
	"os"
	"strings"
)

// Init configures the global slog logger with JSON output and a level
// controlled by the LOG_LEVEL environment variable (debug|info|warn|error).
// Call this once at service startup.
func Init() {
	level := parseLevel(os.Getenv("LOG_LEVEL"))
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     level,
		AddSource: true,
	})
	slog.SetDefault(slog.New(handler))
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

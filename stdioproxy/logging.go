// Package stdioproxy provides a proxy that exposes a stdio MCP server via HTTP streaming.
package stdioproxy

import (
	"log/slog"
	"os"

	"github.com/mattn/go-isatty"
)

// SetupLogger configures slog based on whether we're running in a terminal or as a daemon.
// Terminal mode uses human-readable text format, daemon mode uses JSON for structured logging.
func SetupLogger(level slog.Level) *slog.Logger {
	var handler slog.Handler

	// Detect if stderr is a terminal (we log to stderr to keep stdout clean)
	if isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd()) {
		// Human-readable format for terminal
		handler = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: level,
		})
	} else {
		// JSON format for daemon/log aggregation
		handler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
			Level: level,
		})
	}

	return slog.New(handler)
}

// ParseLogLevel converts a string log level to slog.Level.
// Supported values: debug, info, warn, error. Defaults to info.
func ParseLogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

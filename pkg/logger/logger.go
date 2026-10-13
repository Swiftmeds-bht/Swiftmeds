// Package logger builds the structured (slog) logger every service uses.
//
// Output is JSON on stdout, one line per entry, with a "service" field on every
// line so logs from all 20 containers can be filtered by service.
package logger

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// New returns a JSON logger for service at the given level
// ("debug", "info", "warn" or "error"; anything else means "info").
func New(service, level string) *slog.Logger {
	return NewWithWriter(os.Stdout, service, level)
}

// NewWithWriter is New with a custom output. Used in tests.
func NewWithWriter(w io.Writer, service, level string) *slog.Logger {
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: ParseLevel(level)})
	return slog.New(h).With(slog.String("service", service))
}

// ParseLevel converts a level name to a slog.Level, defaulting to Info.
func ParseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
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

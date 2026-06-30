// Package logging creates structured loggers for the MagicHandy core.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
)

const serviceName = "magichandy"

// New returns a JSON slog logger annotated with the MagicHandy service name.
func New(out io.Writer, level slog.Level) *slog.Logger {
	handler := slog.NewJSONHandler(out, &slog.HandlerOptions{
		Level: level,
	})

	return slog.New(handler).With("service", serviceName)
}

// ParseLevel converts a user-supplied log level string to a slog level.
func ParseLevel(value string) (slog.Level, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(strings.ToLower(value))); err == nil {
		return level, nil
	}

	return slog.LevelInfo, fmt.Errorf("unknown log level %q", value)
}

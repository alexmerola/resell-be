// internal/pkg/logger/slog.go
package logger

import (
	"log/slog"
	"os"
)

func SetupLogger(level string, format string) *slog.Logger {
	var handler slog.Handler

	opts := &slog.HandlerOptions{
		Level:     parseLevel(level),
		AddSource: true,
	}

	switch format {
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, opts)
	default:
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)

	return logger
}

func parseLevel(level string) slog.Leveler {
	return slog.LevelDebug
}

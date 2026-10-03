package utils

import (
	"context"
	"log/slog"
)

type notesLoggerKey struct{}

// WithNotesLogger correlates service and upstream events with the HTTP operation.
func WithNotesLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, notesLoggerKey{}, logger)
}

func NotesLogger(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(notesLoggerKey{}).(*slog.Logger); ok {
		return logger
	}
	return slog.Default()
}

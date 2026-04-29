package logx

import (
	"io"
	"log/slog"
)

func newConsoleHandler(w io.Writer, min Level, addSource bool) slog.Handler {
	return slog.NewTextHandler(w, &slog.HandlerOptions{
		Level:     slogLevel(min),
		AddSource: addSource,
	})
}

func slogLevel(min Level) slog.Level {
	switch min {
	case LevelDebug:
		return slog.LevelDebug
	case LevelInfo:
		return slog.LevelInfo
	case LevelWarn:
		return slog.LevelWarn
	case LevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

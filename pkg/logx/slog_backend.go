package logx

import (
	"io"
	"log/slog"
	"os"
)

// newJSONSlogLogger 构造 JSON 输出的 slog.Logger；仅在本包内使用标准库 slog。
func newJSONSlogLogger(w io.Writer, min Level) *slog.Logger {
	if w == nil {
		w = os.Stdout
	}
	var lv slog.Level
	switch min {
	case LevelDebug:
		lv = slog.LevelDebug
	case LevelInfo:
		lv = slog.LevelInfo
	case LevelWarn:
		lv = slog.LevelWarn
	case LevelError:
		lv = slog.LevelError
	default:
		lv = slog.LevelInfo
	}
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: lv})
	return slog.New(h)
}

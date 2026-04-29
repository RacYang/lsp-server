package logx

import (
	"io"
	"log/slog"
	"os"
)

// newJSONSlogHandler 构造 JSON 输出 Handler；仅在本包内使用标准库 slog。
func newJSONSlogHandler(w io.Writer, min Level, addSource bool) slog.Handler {
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
	return slog.NewJSONHandler(w, &slog.HandlerOptions{Level: lv, AddSource: addSource})
}

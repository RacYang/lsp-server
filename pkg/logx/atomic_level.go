package logx

import (
	"context"
	"log/slog"
	"sync/atomic"
)

// AtomicLevel 支持进程内动态调整日志级别。
type AtomicLevel struct {
	level atomic.Int64
}

// NewAtomicLevel 创建可动态调整的日志级别。
func NewAtomicLevel(level Level) *AtomicLevel {
	lv := &AtomicLevel{}
	lv.SetLevel(level)
	return lv
}

// SetLevel 更新日志级别。
func (lv *AtomicLevel) SetLevel(level Level) {
	lv.level.Store(int64(slogLevel(level)))
}

// Level 返回当前日志级别。
func (lv *AtomicLevel) Level() Level {
	switch slog.Level(lv.level.Load()) {
	case slog.LevelDebug:
		return LevelDebug
	case slog.LevelWarn:
		return LevelWarn
	case slog.LevelError:
		return LevelError
	default:
		return LevelInfo
	}
}

type atomicLevelHandler struct {
	next  slog.Handler
	level *AtomicLevel
}

func newAtomicLevelHandler(next slog.Handler, level *AtomicLevel) slog.Handler {
	return atomicLevelHandler{next: next, level: level}
}

func (h atomicLevelHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= slog.Level(h.level.level.Load()) && h.next.Enabled(ctx, level)
}

func (h atomicLevelHandler) Handle(ctx context.Context, rec slog.Record) error {
	return h.next.Handle(ctx, rec)
}

func (h atomicLevelHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return atomicLevelHandler{next: h.next.WithAttrs(attrs), level: h.level}
}

func (h atomicLevelHandler) WithGroup(name string) slog.Handler {
	return atomicLevelHandler{next: h.next.WithGroup(name), level: h.level}
}
